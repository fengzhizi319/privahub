// Package v1 provides HTTP handlers for API v1alpha1 endpoints.
package v1

import (
	"errors"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// failAuthErr renders a service error with its Java code/args.
func failAuthErr(c *gin.Context, err error) {
	var ae *service.AuthError
	if errors.As(err, &ae) && ae.Code != nil {
		response.FailArgs(c, ae.Code, ae.Args...)
		return
	}
	response.FailWithMsg(c, errcode.SystemUnknownError, err.Error())
}

// Login handles user login (POST /api/login and /api/v1alpha1/user/login).
//
// Request:  {name, passwordHash, passwordHashSm3?}   (see service.AuthService doc)
// Response: Java UserContextDTO {token, name, platformType, platformNodeId,
// ownerType, ownerId, ownerName, projectIds, apiResources, deployMode} plus
// access_token/refresh_token/expires_at/token_type and a legacy `user` object.
// Failures use HTTP 200 with AUTH USER_PASSWORD_ERROR (202011601, uniform for
// unknown users) or USER_IS_LOCKED (202011603).
//
// @Summary User login
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body service.LoginRequest true "Login request"
// @Success 200 {object} response.Body{data=service.LoginResponse}
// @Router /api/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailArgs(c, errcode.SystemValidationError, "name is required")
		return
	}

	resp, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		failAuthErr(c, err)
		return
	}
	response.OK(c, resp)
}

// Logout deletes the current session only (Java deleteByNameAndToken) and
// returns the user name. Requires authentication.
//
// @Summary User logout
// @Tags Auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Body
// @Router /api/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		response.FailHTTP(c, 401, errcode.AuthFailed, "login is required")
		return
	}
	token := c.GetString(middleware.CtxToken)
	if token == "" {
		token = middleware.ExtractToken(c)
	}
	if err := h.authService.Logout(c.Request.Context(), u.Name, token); err != nil {
		response.FailWithMsg(c, errcode.SystemUnknownError, err.Error())
		return
	}
	response.OK(c, u.Name)
}

// RefreshToken handles token refresh for a still-existing session.
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
		response.FailArgs(c, errcode.SystemValidationError, "refresh_token is required")
		return
	}

	tokenPair, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.FailArgs(c, errcode.AuthFailed, "invalid refresh token")
		return
	}
	response.OK(c, tokenPair)
}

// GetCurrentUser returns the current session UserContext.
// @Summary Get current user info
// @Tags Auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Body{data=auth.UserContext}
// @Router /api/v1alpha1/user/current [get]
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		response.FailHTTP(c, 401, errcode.AuthFailed, "login is required")
		return
	}
	response.OK(c, u.Normalize())
}
