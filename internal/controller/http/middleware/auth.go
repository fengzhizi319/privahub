package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// Gin context keys set by the auth middlewares.
const (
	CtxUserContext = "user_context"
	CtxToken       = "user_token"
)

// KusciaOriginSourceHeader identifies the calling node on the inner port
// (set by the Kuscia gateway).
const KusciaOriginSourceHeader = "kuscia-origin-source"

// SessionValidator is implemented by service.AuthService.
type SessionValidator interface {
	// ValidateSession looks the token up in user_tokens (sliding expiry) and
	// returns the stored UserContext.
	ValidateSession(ctx context.Context, token string) (*auth.UserContext, error)
	// DisabledAuthUser is injected when authentication is disabled.
	DisabledAuthUser() *auth.UserContext
	// BuildNodeVirtualUser builds the inner-port identity of a calling node.
	BuildNodeVirtualUser(ctx context.Context, nodeID string) *auth.UserContext
}

// ExtractToken returns the session token from User-Token (Java), Authorization
// Bearer or token headers, falling back to the HttpOnly session cookie.
func ExtractToken(c *gin.Context) string {
	t, _ := ExtractTokenWithSource(c)
	return t
}

// ExtractTokenWithSource is ExtractToken that also reports whether the token
// came from the session cookie (ambient credential: CSRF checks apply).
func ExtractTokenWithSource(c *gin.Context) (token string, fromCookie bool) {
	if t := extractHeaderToken(c); t != "" {
		return t, false
	}
	if t := sessionCookieToken(c); t != "" {
		return t, true
	}
	return "", false
}

func extractHeaderToken(c *gin.Context) string {
	if t := strings.TrimSpace(c.GetHeader("User-Token")); t != "" {
		return t
	}
	if h := c.GetHeader("Authorization"); h != "" {
		parts := strings.Fields(h)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	return strings.TrimSpace(c.GetHeader("token"))
}

// credentialHeadersValid rejects ambiguous credentials before selecting an
// identity. In particular a malformed Authorization header must not silently
// fall back to an ambient browser cookie. Identical legacy/Bearer tokens are
// accepted for client compatibility; conflicting or repeated headers fail closed.
func credentialHeadersValid(c *gin.Context) bool {
	selected := ""
	for _, name := range []string{"User-Token", "Authorization", "token"} {
		values := c.Request.Header.Values(name)
		if len(values) > 1 {
			return false
		}
		if len(values) == 0 {
			continue
		}
		value := strings.TrimSpace(values[0])
		if strings.EqualFold(name, "Authorization") {
			parts := strings.Fields(value)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return false
			}
			value = parts[1]
		}
		if value == "" || strings.ContainsAny(value, " \t\r\n,") {
			return false
		}
		if selected != "" && selected != value {
			return false
		}
		selected = value
	}
	return true
}

// MustChangePasswordAllowed lists the routes a must_change_password session
// may call: read its own context, change the password, log out.
var MustChangePasswordAllowed = map[string]bool{
	"/api/v1alpha1/user/get":       true,
	"/api/v1alpha1/user/current":   true,
	"/api/v1alpha1/user/updatePwd": true,
	"/api/v1alpha1/user/logout":    true,
	"/api/logout":                  true,
}

// abortAuthFailed rejects the request with HTTP 401 and the Java AUTH_FAILED
// code (202011602). HTTP 401 is kept (Java answers 200) because the frontend
// uses it to drop the stored token and redirect to /login.
func abortAuthFailed(c *gin.Context, reason string) {
	response.AbortHTTP(c, http.StatusUnauthorized, errcode.AuthFailed, reason)
}

// SetUser stores the UserContext in both the request context and the gin
// context (plus the legacy username/owner keys used by existing handlers).
func SetUser(c *gin.Context, u *auth.UserContext) {
	c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), u))
	c.Set(CtxUserContext, u)
	c.Set("username", u.Name)
	c.Set("owner_type", u.OwnerType)
	c.Set("owner_id", u.OwnerID)
	if u.UserID != "" {
		c.Set("user_id", u.UserID)
	} else {
		c.Set("user_id", u.Name)
	}
}

// CurrentUser returns the authenticated UserContext (Java UserContext.getUser()),
// or nil for unauthenticated requests.
func CurrentUser(c *gin.Context) *auth.UserContext {
	if v, ok := c.Get(CtxUserContext); ok {
		if u, ok := v.(*auth.UserContext); ok {
			return u
		}
	}
	if u, ok := auth.UserFromContext(c.Request.Context()); ok {
		return u
	}
	return nil
}

// SessionAuth authenticates main-port requests: the token must be a valid
// access JWT (when it is a JWT) AND an existing, non-idle user_tokens session.
// Logout deletes the session so the token stops working immediately.
// When enabled is false a platform admin is injected (Java auth.enabled=false).
func SessionAuth(jwtManager *auth.JWTManager, sessions SessionValidator, enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if !enabled {
			SetUser(c, sessions.DisabledAuthUser())
			c.Next()
			return
		}
		u, token, fromCookie, ok := authenticateSession(c, jwtManager, sessions)
		if !ok {
			return
		}
		SetUser(c, u)
		c.Set(CtxToken, token)
		c.Set(CtxAuthViaCookie, fromCookie)
		c.Next()
	}
}

// authenticateSession runs the login checks shared by SessionAuth and the
// EDGE request forwarder (EdgeForward), so a forwarded request is held to
// exactly the same rules as a locally served one:
//   - token from User-Token / Authorization: Bearer / token header, else the
//     HttpOnly session cookie;
//   - cookie credential => Origin allowlist + CSRF double submit;
//   - JWT signature / expiry / token type (access only) when a JWT manager is
//     configured;
//   - a live server-side session (idle timeout, revocation);
//   - token password version == account password version;
//   - must_change_password sessions only reach MustChangePasswordAllowed.
//
// On failure the request has been aborted and ok is false.
func authenticateSession(c *gin.Context, jwtManager *auth.JWTManager, sessions SessionValidator) (u *auth.UserContext, token string, fromCookie bool, ok bool) {
	if !credentialHeadersValid(c) {
		abortAuthFailed(c, "ambiguous or malformed authentication headers")
		return nil, "", false, false
	}
	token, fromCookie = ExtractTokenWithSource(c)
	if token == "" {
		abortAuthFailed(c, "login is required")
		return nil, "", false, false
	}
	if fromCookie {
		// Ambient cookie credential: Origin allowlist + CSRF double submit.
		if reason := VerifyBrowserRequest(c); reason != "" {
			response.AbortHTTP(c, http.StatusForbidden, errcode.AuthCSRFFailed, reason)
			return nil, "", false, false
		}
	}
	var claims *auth.Claims
	if strings.Count(token, ".") == 2 && jwtManager != nil {
		cl, err := jwtManager.ValidateToken(token)
		if err != nil {
			abortAuthFailed(c, "login is expire, please login again.")
			return nil, "", false, false
		}
		if cl.TokenType != auth.AccessToken {
			abortAuthFailed(c, "invalid token type")
			return nil, "", false, false
		}
		claims = cl
	}
	if sessions == nil {
		abortAuthFailed(c, "login is required")
		return nil, "", false, false
	}
	u, err := sessions.ValidateSession(c.Request.Context(), token)
	if err != nil || u == nil {
		abortAuthFailed(c, "login is required")
		return nil, "", false, false
	}
	if claims != nil {
		if u.UserID == "" {
			u.UserID = claims.UserID
		}
		c.Request = c.Request.WithContext(auth.ContextWithClaims(c.Request.Context(), claims))
		if claims.PwdVer != u.PwdVer {
			// Token issued before the last password change.
			abortAuthFailed(c, "login is expire, please login again.")
			return nil, "", false, false
		}
	}
	if u.MustChangePassword && !MustChangePasswordAllowed[strings.TrimRight(c.Request.URL.Path, "/")] {
		response.AbortHTTP(c, http.StatusForbidden, errcode.AuthPasswordChangeRequired)
		return nil, "", false, false
	}
	return u, token, fromCookie, true
}

// InnerAuth authenticates inner-port (node RPC) requests: the calling node is
// identified by the kuscia-origin-source header and a node-scoped virtual user
// is built (Java LoginInterceptor.processByNodeRpcRequest).
func InnerAuth(sessions SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := strings.TrimSpace(c.GetHeader(KusciaOriginSourceHeader))
		if nodeID == "" {
			abortAuthFailed(c, "Cannot find node id in header for rpc.")
			return
		}
		SetUser(c, sessions.BuildNodeVirtualUser(c.Request.Context(), nodeID))
		c.Next()
	}
}

// BlockInnerPaths answers 404 on the main port for paths that must only be
// reachable through the inner port (Java InnerPortPathConfig / inner-port.path).
func BlockInnerPaths(paths []string) gin.HandlerFunc {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		if p = strings.TrimSpace(p); p != "" {
			set[strings.TrimRight(p, "/")] = true
		}
	}
	return func(c *gin.Context) {
		p := strings.TrimRight(c.Request.URL.Path, "/")
		if set[p] {
			c.AbortWithStatus(http.StatusNotFound)
			_, _ = c.Writer.WriteString("404")
			return
		}
		c.Next()
	}
}
