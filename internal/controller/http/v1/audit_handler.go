package v1

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditHandler serves the read-only security audit trail
// (/api/v1alpha1/audit/*). Only AUDITOR / ADMIN users may call it; there are
// deliberately no update or delete endpoints.
type AuditHandler struct {
	svc        *audit.Service
	db         *gorm.DB
	anchorFile string
	archiveDir string
}

// NewAuditHandler creates the handler. svc may be nil (audit disabled).
func NewAuditHandler(svc *audit.Service, db *gorm.DB, anchorFile, archiveDir string) *AuditHandler {
	return &AuditHandler{svc: svc, db: db, anchorFile: anchorFile, archiveDir: archiveDir}
}

// AuditListRequest filters audit records. Times are unix milliseconds;
// endTime is exclusive.
type AuditListRequest struct {
	StartTime    int64  `json:"startTime"`
	EndTime      int64  `json:"endTime"`
	Actor        string `json:"actor"`
	Action       string `json:"action"`
	Result       string `json:"result"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	IP           string `json:"ip"`
	TraceID      string `json:"traceId"`
	Page         int    `json:"page"`
	Size         int    `json:"size"`
}

// AuditRecordVO is one audit record.
type AuditRecordVO struct {
	Seq          int64             `json:"seq"`
	EventID      string            `json:"eventId"`
	Time         string            `json:"time"`
	Actor        string            `json:"actor"`
	ActorType    string            `json:"actorType"`
	OwnerID      string            `json:"ownerId"`
	IP           string            `json:"ip"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resourceType"`
	ResourceID   string            `json:"resourceId"`
	Result       string            `json:"result"`
	Reason       string            `json:"reason"`
	TraceID      string            `json:"traceId"`
	Extra        map[string]string `json:"extra,omitempty"`
	PrevHash     string            `json:"prevHash"`
	Hash         string            `json:"hash"`
}

// AuditListResponse is a page of audit records (newest first).
type AuditListResponse struct {
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
	List  []AuditRecordVO `json:"list"`
}

// AuditVerifyResponse is the chain verification result.
type AuditVerifyResponse struct {
	*audit.VerifyResult
	AnchoringEnabled bool   `json:"anchoringEnabled"`
	AnchorWarning    string `json:"anchorWarning,omitempty"`
}

func (h *AuditHandler) allowed(c *gin.Context) bool {
	if middleware.IsAuditor(c.Request.Context(), h.db, middleware.CurrentUser(c)) {
		return true
	}
	response.FailWithMsg(c, errcode.AuthFailed, "audit log access requires the AUDITOR or ADMIN role")
	c.Abort()
	return false
}

// List is POST /api/v1alpha1/audit/list.
func (h *AuditHandler) List(c *gin.Context) {
	if !h.allowed(c) {
		return
	}
	if h.svc == nil {
		response.FailWithMsg(c, errcode.SystemError, "audit trail is not available")
		return
	}
	var req AuditListRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.FailWithMsg(c, errcode.SystemValidationError, "invalid request body")
			return
		}
	}
	q := audit.Query{
		Actor: req.Actor, Action: req.Action, Result: req.Result,
		ResourceType: req.ResourceType, ResourceID: req.ResourceID,
		IP: req.IP, TraceID: req.TraceID, Page: req.Page, Size: req.Size,
	}
	if req.StartTime > 0 {
		q.StartMicro = req.StartTime * 1000
	}
	if req.EndTime > 0 {
		q.EndMicro = req.EndTime * 1000
	}
	rows, total, err := h.svc.Store().List(c.Request.Context(), q)
	if err != nil {
		response.FailWithMsg(c, errcode.SystemError, "query audit log failed")
		return
	}
	page, size := max(q.Page, 1), q.Size
	if size <= 0 {
		size = 20
	}
	size = min(size, 200)
	out := AuditListResponse{Total: total, Page: page, Size: size, List: make([]AuditRecordVO, 0, len(rows))}
	for _, r := range rows {
		vo := AuditRecordVO{
			Seq: r.Seq, EventID: r.EventID,
			Time:  time.UnixMicro(r.TsMicro).UTC().Format("2006-01-02T15:04:05.000000Z07:00"),
			Actor: r.Actor, ActorType: r.ActorType, OwnerID: r.OwnerID, IP: r.IP,
			Action: r.Action, ResourceType: r.ResourceType, ResourceID: r.ResourceID,
			Result: r.Result, Reason: r.Reason, TraceID: r.TraceID,
			PrevHash: r.PrevHash, Hash: r.Hash,
		}
		if r.Extra != "" {
			_ = json.Unmarshal([]byte(r.Extra), &vo.Extra)
		}
		out.List = append(out.List, vo)
	}
	response.OK(c, out)
}

// Verify is GET /api/v1alpha1/audit/verify: re-computes the hash chain and
// reports the first broken link. Anchors from audit.anchor.file are checked;
// ?archives=true also verifies archived segment files.
func (h *AuditHandler) Verify(c *gin.Context) {
	if !h.allowed(c) {
		return
	}
	if h.svc == nil {
		response.FailWithMsg(c, errcode.SystemError, "audit trail is not available")
		return
	}
	opt := audit.VerifyOptions{}
	resp := AuditVerifyResponse{AnchoringEnabled: h.anchorFile != "" || h.svc.Options().AnchorURL != ""}
	if h.anchorFile != "" {
		anchors, err := audit.ReadAnchorFile(h.anchorFile)
		switch {
		case err == nil:
			opt.Anchors = anchors
		case errors.Is(err, os.ErrNotExist):
			resp.AnchorWarning = "anchor file not written yet"
		default:
			resp.AnchorWarning = "anchor file unreadable"
		}
	} else if !resp.AnchoringEnabled {
		resp.AnchorWarning = "external anchoring is not configured: the chain only proves self-consistency"
	}
	if c.Query("archives") == "true" {
		opt.ArchiveDir = h.archiveDir
	}
	res, err := h.svc.Store().Verify(c.Request.Context(), opt)
	if err != nil {
		response.FailWithMsg(c, errcode.SystemError, "verify audit chain failed")
		return
	}
	resp.VerifyResult = res
	response.OK(c, resp)
}
