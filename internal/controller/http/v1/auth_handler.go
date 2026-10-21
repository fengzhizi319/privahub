// Package v1 provides HTTP handlers for API v1alpha1 endpoints.
package v1

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
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
// ownerType, ownerId, ownerName, projectIds, apiResources, deployMode,
// mustChangePassword?} plus access_token/refresh_token/expires_at/token_type
// and a legacy `user` object. The session is also set as HttpOnly cookies
// (+ CSRF cookie); with "X-Privahub-Session: cookie" the tokens are omitted
// from the body. Failures use HTTP 200 with 202011601 (uniform "username or
// password error", no remaining-attempt count) or USER_IS_LOCKED (202011603,
// only when the password was correct).
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
	if reason := middleware.VerifyLoginOrigin(c); reason != "" {
		response.FailHTTP(c, http.StatusForbidden, errcode.AuthCSRFFailed, reason)
		return
	}
	req.ClientIP = c.ClientIP()

	resp, err := h.authService.Login(service.WithClientIP(c.Request.Context(), req.ClientIP), &req)
	if err != nil {
		failAuthErr(c, err)
		return
	}
	// Browser session: HttpOnly cookies + fresh CSRF token. API clients keep
	// using the tokens from the body (omitted when the browser asks for a
	// cookie-only session).
	if err := middleware.IssueBrowserSession(c, resp.TokenPair.AccessToken, resp.TokenPair.RefreshToken, true); err != nil {
		// Login already persisted the server-side session; revoke it so the
		// failure does not leave an orphaned session occupying the quota
		// until the idle timeout.
		_ = h.authService.Logout(service.WithClientIP(c.Request.Context(), c.ClientIP()), resp.Name, resp.TokenPair.AccessToken)
		response.FailWithMsg(c, errcode.SystemUnknownError, "cannot issue session")
		return
	}
	if middleware.WantsCookieSession(c) {
		resp.Token = ""
		resp.TokenPair = &auth.TokenPair{ExpiresAt: resp.TokenPair.ExpiresAt, TokenType: "Cookie"}
	}
	response.OK(c, resp)
}

// Logout deletes the current session only (Java deleteByNameAndToken),
// clears the browser session cookies and returns the user name. Requires
// authentication.
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
	ctx := service.WithClientIP(c.Request.Context(), c.ClientIP())
	if err := h.authService.Logout(ctx, u.Name, token); err != nil {
		response.FailWithMsg(c, errcode.SystemUnknownError, err.Error())
		return
	}
	middleware.ClearBrowserSession(c)
	response.OK(c, u.Name)
}

// RefreshToken rotates the token pair of a still-existing session. The
// refresh token comes from the JSON body (API clients) or from the HttpOnly
// refresh cookie (browsers; Origin + CSRF checks apply). Refresh tokens are
// one-time-use: replaying a rotated one revokes the session.
// @Summary Refresh access token
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body object true "Refresh token request"
// @Success 200 {object} response.Body{data=auth.TokenPair}
// @Router /api/user/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&req)
	token, fromCookie := strings.TrimSpace(req.RefreshToken), false
	if token == "" {
		token, fromCookie = middleware.RefreshCookieToken(c), true
	}
	if token == "" {
		response.FailArgs(c, errcode.SystemValidationError, "refresh_token is required")
		return
	}
	if fromCookie {
		if reason := middleware.VerifyBrowserRequest(c); reason != "" {
			response.FailHTTP(c, http.StatusForbidden, errcode.AuthCSRFFailed, reason)
			return
		}
	}

	ctx := service.WithClientIP(c.Request.Context(), c.ClientIP())
	tokenPair, err := h.authService.RefreshToken(ctx, token)
	if err != nil {
		if fromCookie {
			middleware.ClearBrowserSession(c)
		}
		response.FailHTTP(c, http.StatusUnauthorized, errcode.AuthFailed, "invalid refresh token")
		return
	}
	if fromCookie || middleware.WantsCookieSession(c) {
		if err := middleware.IssueBrowserSession(c, tokenPair.AccessToken, tokenPair.RefreshToken, false); err != nil {
			response.FailWithMsg(c, errcode.SystemUnknownError, "cannot issue session")
			return
		}
		response.OK(c, &auth.TokenPair{ExpiresAt: tokenPair.ExpiresAt, TokenType: "Cookie"})
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
	response.OK(c, PublicUserContext(c, u))
}

// PublicUserContext returns the UserContext as sent to clients: the session
// token is never echoed to a cookie-authenticated (browser) caller, so page
// scripts cannot read the HttpOnly credential through the API.
func PublicUserContext(c *gin.Context, u *auth.UserContext) *auth.UserContext {
	out := *u
	out.Normalize()
	if c.GetBool(middleware.CtxAuthViaCookie) {
		out.Token = ""
	}
	return &out
}
