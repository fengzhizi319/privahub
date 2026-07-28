package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// UserHandler handles user management HTTP requests.
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// Create handles user creation.
func (h *UserHandler) Create(c *gin.Context) {
	var req service.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.userService.CreateUser(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrUserAlreadyExists {
			response.Fail(c, errcode.AlreadyExists)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// List handles user list retrieval.
func (h *UserHandler) List(c *gin.Context) {
	result, err := h.userService.ListUsers(c.Request.Context())
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, result)
}

// Update handles user update.
func (h *UserHandler) Update(c *gin.Context) {
	var req service.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.userService.UpdateUser(c.Request.Context(), &req); err != nil {
		if err == service.ErrUserNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Delete handles user deletion.
func (h *UserHandler) Delete(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.userService.DeleteUser(c.Request.Context(), req.Name); err != nil {
		if err == service.ErrUserNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// ResetPassword handles password reset.
func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req service.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.userService.ResetPassword(c.Request.Context(), &req); err != nil {
		if err == service.ErrUserNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Get handles user detail retrieval.
func (h *UserHandler) Get(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.userService.GetUser(c.Request.Context(), req.Name)
	if err != nil {
		if err == service.ErrUserNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// UpdatePwd handles self-service password update.
func (h *UserHandler) UpdatePwd(c *gin.Context) {
	var req service.UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.userService.UpdatePassword(c.Request.Context(), &req); err != nil {
		if err == service.ErrUserNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}

	response.OKEmpty(c)
}

// RemoteResetPassword handles remote user password reset (P2P mode).
func (h *UserHandler) RemoteResetPassword(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
		NodeID      string `json:"nodeId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	err := h.userService.ResetPassword(c.Request.Context(), &service.ResetPasswordRequest{
		Name:        req.Name,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		if err == service.ErrUserNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// NodeResetPassword handles node user password reset.
func (h *UserHandler) NodeResetPassword(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
		NodeID      string `json:"nodeId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	err := h.userService.ResetPassword(c.Request.Context(), &service.ResetPasswordRequest{
		Name:        req.Name,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		if err == service.ErrUserNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}
