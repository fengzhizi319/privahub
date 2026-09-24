package v1

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InstHandler handles inst/* (Java InstController).
type InstHandler struct {
	db  *gorm.DB
	svc *service.InstService
}

// NewInstHandler creates a new InstHandler.
func NewInstHandler(db *gorm.DB, svc *service.InstService) *InstHandler {
	return &InstHandler{db: db, svc: svc}
}

// CreateInst handles Go-only inst/create.
func (h *InstHandler) CreateInst(c *gin.Context) {
	var req struct {
		InstID    string `json:"inst_id"`
		InstIDAlt string `json:"instId"`
		Name      string `json:"name"`
		InstName  string `json:"instName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	id, name := firstNonEmptyStr(req.InstID, req.InstIDAlt), firstNonEmptyStr(req.Name, req.InstName)
	if id == "" || name == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	// Go-only route: besides the ADMIN role (RBAC AdminOnlyPaths), only the
	// CENTER platform owner may register institutions.
	if !requireInstAdmin(c, h.db, true) {
		return
	}
	inst := &model.InstDO{InstID: id, Name: name}
	if err := h.db.WithContext(c.Request.Context()).Create(inst).Error; err != nil {
		response.Fail(c, errcode.InstDuplicated)
		return
	}
	response.OK(c, service.InstVO{InstID: inst.InstID, InstName: inst.Name})
}

// ListInsts handles Go-only inst/list.
func (h *InstHandler) ListInsts(c *gin.Context) {
	if !requireInstAdmin(c, h.db, false) {
		return
	}
	// A non-CENTER admin only sees its own institution.
	q := h.db.WithContext(c.Request.Context())
	if u := middleware.CurrentUser(c); !u.IsCenterOwner() {
		q = q.Where("inst_id = ?", u.OwnerID)
	}
	var insts []model.InstDO
	if err := q.Find(&insts).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}
	result := make([]service.InstVO, 0, len(insts))
	for _, inst := range insts {
		result = append(result, service.InstVO{InstID: inst.InstID, InstName: inst.Name})
	}
	response.OK(c, result)
}

// GetInst handles inst/get (instId must equal the login ownerId).
func (h *InstHandler) GetInst(c *gin.Context) {
	var req struct {
		InstID    string `json:"instId"`
		InstIDAlt string `json:"inst_id"`
	}
	_ = c.ShouldBindJSON(&req)
	vo, err := h.svc.GetInst(nodeReqCtx(c), firstNonEmptyStr(req.InstID, req.InstIDAlt))
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// ListInstNodes handles inst/node/list.
func (h *InstHandler) ListInstNodes(c *gin.Context) {
	nodes, err := h.svc.ListNodes(nodeReqCtx(c))
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, nodes)
}

// AddInstNode handles inst/node/add → InstTokenVO.
func (h *InstHandler) AddInstNode(c *gin.Context) {
	var req service.InstCreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.svc.CreateNode(nodeReqCtx(c), &req)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// InstNodeToken handles inst/node/token.
func (h *InstHandler) InstNodeToken(c *gin.Context) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.svc.GetToken(nodeReqCtx(c), req.id())
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// InstNodeNewToken handles inst/node/newToken.
func (h *InstHandler) InstNodeNewToken(c *gin.Context) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.svc.NewToken(nodeReqCtx(c), req.id())
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// DeleteInstNode handles inst/node/delete.
func (h *InstHandler) DeleteInstNode(c *gin.Context) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.svc.DeleteNode(nodeReqCtx(c), req.id()); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

const registerFileMax = 1 << 20

func readFormFile(c *gin.Context, name string) ([]byte, bool, error) {
	fh, err := c.FormFile(name)
	if err != nil {
		return nil, false, nil
	}
	if fh.Size > registerFileMax {
		return nil, true, errcode.InstRegisterCheckFailed
	}
	return readMultipart(fh)
}

func readMultipart(fh *multipart.FileHeader) ([]byte, bool, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, true, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, registerFileMax+1))
	if err != nil {
		return nil, true, err
	}
	if len(b) > registerFileMax {
		return nil, true, errcode.InstRegisterCheckFailed
	}
	return b, true, nil
}

// RegisterInstNode handles inst/node/register (multipart: json_data, certFile,
// keyFile, token; optional caFile). json_data is read from the multipart form
// (Java @RequestParam), falling back to the query string for older clients.
func (h *InstHandler) RegisterInstNode(c *gin.Context) {
	jsonData := c.PostForm("json_data")
	if jsonData == "" {
		jsonData = c.Query("json_data")
	}
	if jsonData == "" {
		response.FailWithMsg(c, errcode.InstRegisterCheckFailed, "json_data is required")
		return
	}
	var req service.InstRegisterRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		response.FailWithMsg(c, errcode.InstRegisterCheckFailed, "json_data is invalid")
		return
	}
	var err error
	for _, f := range []struct {
		name string
		dst  *[]byte
	}{{"certFile", &req.CertFile}, {"keyFile", &req.KeyFile}, {"token", &req.Token}, {"caFile", &req.CAFile}} {
		b, present, rerr := readFormFile(c, f.name)
		if rerr != nil {
			err = rerr
			break
		}
		if present {
			if b == nil {
				b = []byte{}
			}
			*f.dst = b
		}
	}
	if err != nil {
		response.FailWithMsg(c, errcode.InstRegisterCheckFailed, "file is invalid")
		return
	}
	remote := c.ClientIP()
	if remote == "" {
		remote, _, _ = net.SplitHostPort(c.Request.RemoteAddr)
	}
	if err := h.svc.RegisterNode(nodeReqCtx(c), &req, remote); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

// requireInstAdmin checks the Go-only inst/create and inst/list callers: an
// ADMIN (or CENTER_ADMIN) user, and for writes the CENTER platform owner.
// Denials answer AUTH_FAILED (HTTP 200, Java convention).
func requireInstAdmin(c *gin.Context, db *gorm.DB, write bool) bool {
	u := middleware.CurrentUser(c)
	if u == nil || !middleware.IsAdmin(c.Request.Context(), db, u) {
		response.FailWithMsg(c, errcode.AuthFailed, "institution management requires the ADMIN role")
		c.Abort()
		return false
	}
	if write && !u.IsCenterOwner() {
		response.FailWithMsg(c, errcode.AuthFailed, "only the CENTER platform owner can create institutions")
		c.Abort()
		return false
	}
	return true
}
