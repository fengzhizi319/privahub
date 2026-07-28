// Package v1 provides HTTP handlers for API v1alpha1 endpoints.
package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login handles user login.
// @Summary User login
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body service.LoginRequest true "Login request"
// @Success 200 {object} response.Body{data=service.LoginResponse}
// @Router /api/v1alpha1/user/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	resp, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case service.ErrUserNotFound:
			response.FailWithMsg(c, errcode.NotFound, "user not found")
		case service.ErrInvalidPassword:
			response.FailWithMsg(c, errcode.Unauthorized, "invalid password")
		case service.ErrUserLocked:
			response.FailWithMsg(c, errcode.Forbidden, "account locked, please try again later")
		default:
			response.Fail(c, errcode.SystemError)
		}
		return
	}

	response.OK(c, resp)
}

// Logout handles user logout.
// @Summary User logout
// @Tags Auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Body
// @Router /api/v1alpha1/user/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		response.Fail(c, errcode.Unauthorized)
		return
	}

	if err := h.authService.Logout(c.Request.Context(), username.(string)); err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// RefreshToken handles token refresh.
// @Summary Refresh access token
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body object true "Refresh token request"
// @Success 200 {object} response.Body{data=auth.TokenPair}
// @Router /api/v1alpha1/user/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	tokenPair, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.FailWithMsg(c, errcode.Unauthorized, "invalid refresh token")
		return
	}

	response.OK(c, tokenPair)
}

// GetCurrentUser returns the current authenticated user's information.
// @Summary Get current user info
// @Tags Auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Body{data=service.UserInfo}
// @Router /api/v1alpha1/user/current [get]
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c.Request.Context())
	if !ok {
		response.Fail(c, errcode.Unauthorized)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": gin.H{"code": 0, "msg": "success"},
		"data": gin.H{
			"name":       claims.Username,
			"owner_type": claims.OwnerType,
			"owner_id":   claims.OwnerID,
		},
	})
}
