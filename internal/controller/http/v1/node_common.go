package v1

import (
	"context"
	"errors"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// nodeReqCtx returns the request context carrying the login UserContext. When
// the auth middleware did not attach one (legacy JWT path), a minimal context
// is rebuilt from the gin keys it sets (owner_type / owner_id / username).
func nodeReqCtx(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if _, ok := auth.UserFromContext(ctx); ok {
		return ctx
	}
	ownerID := c.GetString("owner_id")
	if ownerID == "" {
		return ctx
	}
	u := &auth.UserContext{
		Name:      c.GetString("username"),
		OwnerType: c.GetString("owner_type"),
		OwnerID:   ownerID,
	}
	return auth.ContextWithUser(ctx, u)
}

// failNodeErr maps service errors (BizError / ErrorCode) to the response envelope.
func failNodeErr(c *gin.Context, err error) {
	if be, ok := service.AsBizError(err); ok && be.Code != nil {
		if be.Msg == "" {
			response.Fail(c, be.Code)
			return
		}
		response.FailWithMsg(c, be.Code, be.Msg)
		return
	}
	var ec *errcode.ErrorCode
	if errors.As(err, &ec) {
		response.Fail(c, ec)
		return
	}
	response.FailWithMsg(c, errcode.SystemError, err.Error())
}

// nodeIDBody accepts both the Java camelCase and legacy snake_case node id.
type nodeIDBody struct {
	NodeID    string `json:"nodeId"`
	NodeIDAlt string `json:"node_id"`
}

func (b nodeIDBody) id() string {
	if b.NodeID != "" {
		return b.NodeID
	}
	return b.NodeIDAlt
}

// routerIDBody accepts routerId / router_id / routeId.
type routerIDBody struct {
	RouterID    string `json:"routerId"`
	RouterIDAlt string `json:"router_id"`
	RouteID     string `json:"routeId"`
}

func (b routerIDBody) id() string {
	for _, v := range []string{b.RouterID, b.RouterIDAlt, b.RouteID} {
		if v != "" {
			return v
		}
	}
	return ""
}
