package v1

import (
	"errors"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// UserHandler handles user management HTTP requests.
type UserHandler struct {
	userService     *service.UserService
	nodeUserService *service.NodeUserService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// SetNodeUserService injects the node user service (node/remote reset).
func (h *UserHandler) SetNodeUserService(s *service.NodeUserService) *UserHandler {
	h.nodeUserService = s
	return h
}

// Create handles user creation (Go-only, ADMIN role required by RBAC).
func (h *UserHandler) Create(c *gin.Context) {
	var req service.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailArgs(c, errcode.SystemValidationError, err.Error())
		return
	}

	vo, err := h.userService.CreateUser(c.Request.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserAlreadyExists):
			response.FailArgs(c, errcode.SystemValidationError, "The name already exists in db.")
		case errors.Is(err, service.ErrPasswordRequired):
			response.FailArgs(c, errcode.SystemValidationError, "passwordHash required")
		default:
			response.FailWithMsg(c, errcode.SystemError, err.Error())
		}
		return
	}
	middleware.InvalidateUserCache(req.Name)
	response.OK(c, vo)
}

// List handles user list retrieval (Go-only, ADMIN).
func (h *UserHandler) List(c *gin.Context) {
	result, err := h.userService.ListUsers(c.Request.Context())
	if err != nil {
		response.FailWithMsg(c, errcode.SystemError, err.Error())
		return
	}
	response.OK(c, result)
}

// Update handles user update (Go-only, ADMIN).
func (h *UserHandler) Update(c *gin.Context) {
	var req service.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailArgs(c, errcode.SystemValidationError, err.Error())
		return
	}
	if err := h.userService.UpdateUser(c.Request.Context(), &req); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.FailWithMsg(c, errcode.SystemError, err.Error())
		return
	}
	middleware.InvalidateUserCache(req.Name)
	response.OKEmpty(c)
}

// Delete handles user deletion (Go-only, ADMIN). Deleting yourself is refused.
func (h *UserHandler) Delete(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailArgs(c, errcode.SystemValidationError, "name is required")
		return
	}
	if u := middleware.CurrentUser(c); u != nil && u.Name == req.Name {
		response.FailArgs(c, errcode.SystemValidationError, "cannot delete the current user")
		return
	}
	if err := h.userService.DeleteUser(c.Request.Context(), req.Name); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.FailWithMsg(c, errcode.SystemError, err.Error())
		return
	}
	middleware.InvalidateUserCache(req.Name)
	response.OKEmpty(c)
}

// ResetPassword handles admin password reset (Go-only, ADMIN). All sessions
// of the target user are cleared.
func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req service.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailArgs(c, errcode.SystemValidationError, err.Error())
		return
	}
	if err := h.userService.ResetPassword(c.Request.Context(), &req); err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			response.Fail(c, errcode.NotFound)
		case errors.Is(err, service.ErrPasswordRequired):
			response.FailArgs(c, errcode.SystemValidationError, "newPasswordHash required")
		default:
			response.FailWithMsg(c, errcode.SystemError, err.Error())
		}
		return
	}
	response.OKEmpty(c)
}

// Get returns the current session user as Java UserContextDTO
// (POST /api/v1alpha1/user/get). The request body is ignored.
func (h *UserHandler) Get(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		response.FailHTTP(c, 401, errcode.AuthFailed, "login is required")
		return
	}
	response.OK(c, u.Normalize())
}

// UpdatePwd handles self-service password update (Java UserController.updatePwd).
// Body: {oldPasswordHash, newPasswordHash, confirmPasswordHash, oldPasswordHashSm3?};
// the user name always comes from the session. Returns data=true.
func (h *UserHandler) UpdatePwd(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		response.FailHTTP(c, 401, errcode.AuthFailed, "login is required")
		return
	}
	var req service.UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailArgs(c, errcode.SystemValidationError, err.Error())
		return
	}
	req.Name = u.Name
	if err := h.userService.UpdatePassword(c.Request.Context(), &req); err != nil {
		failAuthErr(c, err)
		return
	}
	response.OK(c, true)
}

// NodeResetPassword resets a node user's password (Java NodeUserController,
// inner port only). Body: {nodeId, name, passwordHash, newPasswordHash}. The
// caller must have access to nodeId (DataResource NODE_ID) and the account
// must belong to nodeId with a matching current hash.
func (h *UserHandler) NodeResetPassword(c *gin.Context) {
	var req service.ResetNodeUserPwdRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Validate() != nil {
		response.FailArgs(c, errcode.SystemValidationError, "nodeId/name/passwordHash/newPasswordHash required")
		return
	}
	if !middleware.RequireNodeAccess(c, req.NodeID) {
		return
	}
	if h.nodeUserService == nil {
		response.FailArgs(c, errcode.SystemUnknownError, "node user service unavailable")
		return
	}
	if err := h.nodeUserService.ResetPassword(c.Request.Context(), &req); err != nil {
		code, msg := service.NodeUserErrorCode(err, &req)
		response.FailArgs(c, code, msg)
		return
	}
	response.OK(c, "ok")
}

// RemoteResetPassword proxies the node reset to the center platform through
// the Kuscia gateway (Java RemoteUserController). Same body as
// NodeResetPassword; the center's response is returned verbatim.
func (h *UserHandler) RemoteResetPassword(c *gin.Context) {
	var req service.ResetNodeUserPwdRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Validate() != nil {
		response.FailArgs(c, errcode.SystemValidationError, "nodeId/name/passwordHash/newPasswordHash required")
		return
	}
	if !middleware.RequireNodeAccess(c, req.NodeID) {
		return
	}
	if h.nodeUserService == nil {
		response.FailArgs(c, errcode.SystemRemoteCallError, "node user service unavailable")
		return
	}
	raw, err := h.nodeUserService.RemoteResetPassword(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrRemoteNotConfigured) || errors.Is(err, service.ErrNodeUserInvalidParam) ||
			errors.Is(err, service.ErrNodeUserNotFound) || errors.Is(err, service.ErrNodeUserWrongPwd) {
			code, msg := service.NodeUserErrorCode(err, &req)
			response.FailArgs(c, code, msg)
			return
		}
		response.FailArgs(c, errcode.SystemRemoteCallError, err.Error())
		return
	}
	c.Data(200, "application/json; charset=utf-8", raw)
}
