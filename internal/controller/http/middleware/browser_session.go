// Copyright 2026 The Privahub Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package middleware

// Browser session hardening: HttpOnly session cookie + CSRF double submit +
// Origin allowlist (ported from PrivShield pkg/middleware/browser_session.go).
//
// Why: a token in localStorage is fully readable by JS, so one XSS or a
// poisoned dependency can exfiltrate a credential that works anywhere. An
// HttpOnly cookie cannot be read by JS. Because browsers attach cookies
// automatically, cookie-authenticated state-changing requests additionally
// need (1) SameSite=Strict, (2) a double-submit CSRF token (readable cookie
// echoed in the X-CSRF-Token header) and (3) an Origin/Referer allowlist.
//
// Boundary (stated honestly): the CSRF token is readable by page scripts, so
// XSS can still act *inside* the page; what this removes is credential theft
// and reuse elsewhere.
//
// API/automation clients keep using "Authorization: Bearer" / "User-Token"
// headers; header authentication is not ambient, so it needs no CSRF check.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// Cookie / header names. The __Host- prefix (Secure, Path=/, no Domain) is
// used whenever the cookie is Secure; plain HTTP development falls back to
// the unprefixed names (browsers refuse __Host- cookies without Secure).
const (
	SessionCookieSecure = "__Host-privahub_session"
	SessionCookiePlain  = "privahub_session"
	RefreshCookieSecure = "__Host-privahub_refresh"
	RefreshCookiePlain  = "privahub_refresh"
	CSRFCookieSecure    = "__Host-privahub_csrf"
	CSRFCookiePlain     = "privahub_csrf"
	CSRFHeader          = "X-CSRF-Token"
	// SessionModeHeader: a browser client sends "X-Privahub-Session: cookie"
	// on login/refresh so the tokens are only set as HttpOnly cookies and
	// omitted from the JSON body.
	SessionModeHeader = "X-Privahub-Session"
)

// CtxAuthViaCookie is set (true) on requests authenticated by the session cookie.
const CtxAuthViaCookie = "auth_via_cookie"

// BrowserSessionConfig configures the cookie session.
type BrowserSessionConfig struct {
	// Secure: nil = automatic (Secure when the request arrived over TLS or
	// with X-Forwarded-Proto: https); true/false force it.
	Secure *bool
	// AllowedOrigins are the origins accepted for cookie-authenticated
	// state-changing requests (server.cors_allowed_origins; "*" is ignored).
	// Same-origin requests (Origin host == Host) are always accepted.
	AllowedOrigins []string
	// AllowAnyOrigin disables the Origin check (dev profile with no
	// configured origins, e.g. the Vite proxy on another port). The CSRF
	// token is still required.
	AllowAnyOrigin bool
	// AccessMaxAge / RefreshMaxAge are the cookie lifetimes.
	AccessMaxAge  time.Duration
	RefreshMaxAge time.Duration
}

var browserCfg atomic.Pointer[BrowserSessionConfig]

func init() {
	browserCfg.Store(&BrowserSessionConfig{AccessMaxAge: 2 * time.Hour, RefreshMaxAge: 7 * 24 * time.Hour})
}

// SetBrowserSessionConfig installs the process-wide cookie session config.
func SetBrowserSessionConfig(cfg BrowserSessionConfig) {
	if cfg.AccessMaxAge <= 0 {
		cfg.AccessMaxAge = 2 * time.Hour
	}
	if cfg.RefreshMaxAge <= 0 {
		cfg.RefreshMaxAge = 7 * 24 * time.Hour
	}
	origins := make([]string, 0, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o = normalizeOrigin(o); o != "" && o != "*" {
			origins = append(origins, o)
		}
	}
	cfg.AllowedOrigins = origins
	browserCfg.Store(&cfg)
}

// GetBrowserSessionConfig returns the active config.
func GetBrowserSessionConfig() BrowserSessionConfig { return *browserCfg.Load() }

// cookieSecure reports whether cookies for this request must be Secure.
func cookieSecure(c *gin.Context) bool {
	cfg := browserCfg.Load()
	if cfg.Secure != nil {
		return *cfg.Secure
	}
	return c.Request.TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

func cookieNames(secure bool) (session, refresh, csrf string) {
	if secure {
		return SessionCookieSecure, RefreshCookieSecure, CSRFCookieSecure
	}
	return SessionCookiePlain, RefreshCookiePlain, CSRFCookiePlain
}

// NewCSRFToken returns 32 random bytes, hex encoded.
func NewCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func setCookie(c *gin.Context, name, value string, maxAge time.Duration, secure, httpOnly bool) {
	ma := int(maxAge / time.Second)
	if value == "" {
		ma = -1
	}
	// httpOnly is false only for the CSRF double-submit cookie, which the SPA
	// must read to echo into X-CSRF-Token; session/refresh cookies are HttpOnly.
	http.SetCookie(c.Writer, &http.Cookie{ //nolint:gosec // G124: CSRF cookie is intentionally JS-readable (double-submit); Secure follows TLS, SameSite=Strict
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   ma,
		Secure:   secure,
		HttpOnly: httpOnly,
		SameSite: http.SameSiteStrictMode,
	})
}

// IssueBrowserSession sets the HttpOnly session + refresh cookies and a NEW
// CSRF token (rotated on every login to prevent fixation). Pass rotateCSRF
// false on refresh so concurrent requests holding the current CSRF token keep
// working.
func IssueBrowserSession(c *gin.Context, accessToken, refreshToken string, rotateCSRF bool) error {
	cfg := browserCfg.Load()
	secure := cookieSecure(c)
	sName, rName, cName := cookieNames(secure)
	csrf := ""
	if !rotateCSRF {
		csrf, _ = c.Cookie(cName)
	}
	if csrf == "" {
		t, err := NewCSRFToken()
		if err != nil {
			return err
		}
		csrf = t
	}
	setCookie(c, sName, accessToken, cfg.RefreshMaxAge, secure, true)
	if refreshToken != "" {
		setCookie(c, rName, refreshToken, cfg.RefreshMaxAge, secure, true)
	}
	// Not HttpOnly: the double-submit pattern needs JS to read it. It is not
	// a credential.
	setCookie(c, cName, csrf, cfg.RefreshMaxAge, secure, false)
	return nil
}

// ClearBrowserSession expires every session cookie (both name variants).
func ClearBrowserSession(c *gin.Context) {
	for _, secure := range []bool{true, false} {
		s, r, x := cookieNames(secure)
		setCookie(c, s, "", 0, secure, true)
		setCookie(c, r, "", 0, secure, true)
		setCookie(c, x, "", 0, secure, false)
	}
}

// sessionCookieToken returns the session token from the cookie matching the
// request's security mode. In Secure mode only the __Host- cookie is read, so
// a cookie planted by a sibling (sub)domain or over plain HTTP is ignored.
func sessionCookieToken(c *gin.Context) string {
	name, _, _ := cookieNames(cookieSecure(c))
	v, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// RefreshCookieToken returns the refresh token cookie of the request.
func RefreshCookieToken(c *gin.Context) string {
	_, name, _ := cookieNames(cookieSecure(c))
	v, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// WantsCookieSession reports whether a browser client asked for a
// cookie-only session (tokens omitted from JSON bodies).
func WantsCookieSession(c *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(c.GetHeader(SessionModeHeader)), "cookie")
}

func isStateChangingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	}
	return true
}

func normalizeOrigin(o string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(o)), "/")
}

// requestOrigin returns the Origin header, falling back to the scheme://host
// of the Referer.
func requestOrigin(r *http.Request) string {
	if o := strings.TrimSpace(r.Header.Get("Origin")); o != "" && o != "null" {
		return normalizeOrigin(o)
	}
	ref := strings.TrimSpace(r.Header.Get("Referer"))
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return normalizeOrigin(u.Scheme + "://" + u.Host)
}

// OriginAllowed reports whether a request origin is acceptable: configured
// allowlist, same origin as the request Host, or AllowAnyOrigin (dev).
func OriginAllowed(c *gin.Context, origin string) bool {
	cfg := browserCfg.Load()
	if cfg.AllowAnyOrigin {
		return true
	}
	if origin == "" {
		return false
	}
	for _, a := range cfg.AllowedOrigins {
		if subtle.ConstantTimeCompare([]byte(origin), []byte(a)) == 1 {
			return true
		}
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, c.Request.Host)
}

// VerifyBrowserRequest enforces the Origin allowlist and CSRF double submit
// on a cookie-authenticated state-changing request. It returns "" when the
// request passes (or is not state changing) and a short reason otherwise.
func VerifyBrowserRequest(c *gin.Context) string {
	if !isStateChangingMethod(c.Request.Method) {
		return ""
	}
	cfg := browserCfg.Load()
	origin := requestOrigin(c.Request)
	if !cfg.AllowAnyOrigin {
		if origin == "" {
			return "missing Origin"
		}
		if !OriginAllowed(c, origin) {
			return "origin not allowed"
		}
	}
	_, _, cName := cookieNames(cookieSecure(c))
	cookie, _ := c.Cookie(cName)
	header := strings.TrimSpace(c.GetHeader(CSRFHeader))
	if cookie == "" || header == "" || subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) != 1 {
		return "invalid CSRF token"
	}
	return ""
}

// VerifyLoginOrigin rejects a login whose Origin (when present) is not
// allowed (login CSRF). Non-browser clients send no Origin and pass.
func VerifyLoginOrigin(c *gin.Context) string {
	if o := requestOrigin(c.Request); o != "" && !OriginAllowed(c, o) {
		return "origin not allowed"
	}
	return ""
}
