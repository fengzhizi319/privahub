package v1

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/service"
	datasync "github.com/fengzhizi319/privahub/internal/service/sync"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// SyncHandler serves the inner-port node-to-node sync endpoints:
// POST /api/v1alpha1/vote_sync/create (Java VoteSyncController) and
// POST /api/v1alpha1/data/sync (Java p2p DataSyncController).
type SyncHandler struct {
	voteSync *datasync.VoteSyncService
	consumer *datasync.Consumer
	log      *zap.Logger
}

// NewSyncHandler creates the handler.
func NewSyncHandler(voteSync *datasync.VoteSyncService, consumer *datasync.Consumer, log *zap.Logger) *SyncHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &SyncHandler{voteSync: voteSync, consumer: consumer, log: log}
}

// VoteSyncCreate handles POST /api/v1alpha1/vote_sync/create:
// {dbSyncRequests:[{syncDataType, projectNodesInfo}]} upserts VOTE_REQUEST /
// VOTE_INVITE / NODE_ROUTE / TEE_NODE_DATATABLE_MANAGEMENT / PROJECT /
// PROJECT_NODE / PROJECT_APPROVAL_CONFIG entities.
func (h *SyncHandler) VoteSyncCreate(c *gin.Context) {
	var req datasync.VoteSyncRequest
	body, err := io.ReadAll(c.Request.Body)
	if err == nil {
		err = json.Unmarshal(body, &req)
	}
	if err != nil {
		response.FailArgs(c, errcode.SystemValidationError, "invalid request body")
		return
	}
	if len(req.DbSyncRequests) == 0 {
		response.FailArgs(c, errcode.SystemValidationError, datasync.ErrEmptyVoteSync.Error())
		return
	}
	if err := h.voteSync.Sync(c.Request.Context(), req.DbSyncRequests); err != nil {
		h.log.Warn("vote sync failed", zap.String("origin", c.GetHeader("kuscia-origin-source")), zap.Error(err))
		if errors.Is(err, datasync.ErrEmptyVoteSync) || strings.Contains(err.Error(), "dbSyncRequests[") {
			response.FailArgs(c, errcode.SystemValidationError, err.Error())
			return
		}
		response.FailWithMsg(c, errcode.SystemError, "vote sync failed")
		return
	}
	response.OKEmpty(c)
}

// DataSync handles POST /api/v1alpha1/data/sync (P2P consumer, Java
// DataSyncConsumerTemplate): {tableName, action, data} is upserted/deleted by
// its business key. Only the configured sync tables are accepted.
func (h *SyncHandler) DataSync(c *gin.Context) {
	origin := strings.TrimSpace(c.GetHeader("kuscia-origin-source"))
	var d service.SyncData
	body, err := io.ReadAll(c.Request.Body)
	if err == nil {
		err = json.Unmarshal(body, &d)
	}
	if err != nil || d.TableName == "" {
		response.FailArgs(c, errcode.SystemValidationError, "invalid sync data")
		return
	}
	h.log.Info("dataSyncConsumer consumer", zap.String("origin", origin), zap.String("table", d.TableName), zap.String("action", d.Action))
	if _, err := h.consumer.ApplyFromPeer(c.Request.Context(), origin, &d); err != nil {
		h.log.Warn("data sync failed", zap.String("origin", origin), zap.String("table", d.TableName),
			zap.String("action", d.Action), zap.Error(err))
		switch {
		case errors.Is(err, datasync.ErrTableNotAllowed):
			response.FailArgs(c, errcode.SystemValidationError, datasync.ErrTableNotAllowed.Error())
		case errors.Is(err, datasync.ErrNotOwner):
			audit.Record(c.Request.Context(), audit.Event{
				Actor: origin, ActorType: audit.ActorNode, IP: c.ClientIP(), Action: audit.ActionAuthzDenied,
				ResourceType: "sync:" + d.TableName, Result: audit.ResultDenied,
				Reason: "P2P data sync row not owned by the sending node",
				Extra:  map[string]string{"action": d.Action, "path": c.Request.URL.Path},
			})
			response.FailWithMsg(c, errcode.AuthFailed, "sync data is not owned by the sending node")
		default:
			response.FailWithMsg(c, errcode.SystemError, "data sync failed")
		}
		return
	}
	response.OK(c, &d)
}
