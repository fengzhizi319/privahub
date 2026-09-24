package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service"
)

type cookieJar map[string]string

func (j cookieJar) absorb(w *httptest.ResponseRecorder) {
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(j, c.Name)
		} else {
			j[c.Name] = c.Value
		}
	}
}

func (e *authFlowEnv) browser(t *testing.T, path string, jar cookieJar, csrf bool, body interface{}, hdr ...string) (*httptest.ResponseRecorder, flowResp) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "http://privahub.local"+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://privahub.local")
	for k, v := range jar {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	if csrf {
		req.Header.Set(middleware.CSRFHeader, jar[middleware.CSRFCookiePlain])
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	jar.absorb(w)
	var fr flowResp
	_ = json.Unmarshal(w.Body.Bytes(), &fr)
	return w, fr
}

func TestAuthFlow_BrowserCookieSession(t *testing.T) {
	e := newAuthFlowEnv(t)
	jar := cookieJar{}
	login := map[string]string{"name": "admin", "passwordHash": service.HashPassword("12345678")}

	// Cookie-mode login: tokens only in HttpOnly cookies, not in the body.
	w, resp := e.browser(t, "/api/login", jar, false, login, middleware.SessionModeHeader, "cookie")
	if resp.Status.Code != 0 {
		t.Fatalf("login: %+v", resp.Status)
	}
	var data map[string]interface{}
	_ = json.Unmarshal(resp.Data, &data)
	if data["token"] != "" || data["access_token"] != "" || data["refresh_token"] != "" {
		t.Fatalf("cookie-mode login must not expose tokens: %v", data)
	}
	if jar[middleware.SessionCookiePlain] == "" || jar[middleware.RefreshCookiePlain] == "" || jar[middleware.CSRFCookiePlain] == "" {
		t.Fatalf("session cookies not set: %v", w.Header()["Set-Cookie"])
	}

	// State-changing request without the CSRF header is refused.
	if w, _ := e.browser(t, "/api/v1alpha1/user/get", jar, false, nil); w.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF must be 403, got %d", w.Code)
	}
	// Foreign origin is refused even with a valid CSRF token.
	if w, _ := e.browser(t, "/api/v1alpha1/user/get", jar, true, nil, "Origin", "https://evil.example"); w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin must be 403, got %d", w.Code)
	}
	w, resp = e.browser(t, "/api/v1alpha1/user/get", jar, true, nil)
	if w.Code != 200 || resp.Status.Code != 0 {
		t.Fatalf("cookie + CSRF user/get: %d %+v", w.Code, resp.Status)
	}
	_ = json.Unmarshal(resp.Data, &data)
	if data["token"] != "" || data["name"] != "admin" {
		t.Fatalf("user/get must not echo the cookie token: %v", data)
	}

	// Refresh via cookie rotates the pair; replaying the old refresh cookie
	// revokes the session.
	oldRefresh := jar[middleware.RefreshCookiePlain]
	oldCSRF := jar[middleware.CSRFCookiePlain]
	if w, resp := e.browser(t, "/api/user/refresh", jar, true, nil); w.Code != 200 || resp.Status.Code != 0 {
		t.Fatalf("refresh: %d %+v", w.Code, resp.Status)
	}
	if jar[middleware.RefreshCookiePlain] == oldRefresh || jar[middleware.CSRFCookiePlain] != oldCSRF {
		t.Fatal("refresh must rotate the refresh cookie and keep the CSRF token")
	}
	if w, _ := e.browser(t, "/api/v1alpha1/user/get", jar, true, nil); w.Code != 200 {
		t.Fatalf("session after refresh: %d", w.Code)
	}
	stolen := cookieJar{middleware.RefreshCookiePlain: oldRefresh, middleware.CSRFCookiePlain: oldCSRF}
	if w, _ := e.browser(t, "/api/user/refresh", stolen, true, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token replay must fail: %d", w.Code)
	}
	if w, _ := e.browser(t, "/api/v1alpha1/user/get", jar, true, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("replay must revoke the session: %d", w.Code)
	}

	// Log in again and log out: cookies cleared, session gone.
	e.browser(t, "/api/login", jar, false, login, middleware.SessionModeHeader, "cookie")
	sessionTok := jar[middleware.SessionCookiePlain]
	if w, _ := e.browser(t, "/api/logout", jar, true, nil); w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	if jar[middleware.SessionCookiePlain] != "" {
		t.Fatal("logout must clear the session cookie")
	}
	if code, _ := e.post(t, "/api/v1alpha1/user/get", sessionTok, nil); code != http.StatusUnauthorized {
		t.Fatalf("logged-out token still valid: %d", code)
	}
}

func TestAuthFlow_BearerStillWorksWithoutCSRF(t *testing.T) {
	e := newAuthFlowEnv(t)
	_, resp := e.post(t, "/api/login", "", map[string]string{"name": "admin", "passwordHash": service.HashPassword("12345678")})
	var data map[string]interface{}
	_ = json.Unmarshal(resp.Data, &data)
	token, _ := data["token"].(string)
	if token == "" || data["refresh_token"] == "" {
		t.Fatalf("API clients keep receiving tokens: %v", data)
	}
	code, r := e.post(t, "/api/v1alpha1/user/get", "", nil, "Authorization", "Bearer "+token, "Origin", "https://evil.example")
	if code != 200 || r.Status.Code != 0 {
		t.Fatalf("bearer user/get: %d %+v", code, r.Status)
	}
	// Body refresh (API clients) rotates and returns the new pair.
	code, r = e.post(t, "/api/user/refresh", "", map[string]string{"refresh_token": data["refresh_token"].(string)})
	if code != 200 || r.Status.Code != 0 {
		t.Fatalf("body refresh: %d %+v", code, r.Status)
	}
}

func TestAuthFlow_LoginRejectsForeignOrigin(t *testing.T) {
	e := newAuthFlowEnv(t)
	code, _ := e.post(t, "/api/login", "", map[string]string{"name": "admin", "passwordHash": service.HashPassword("12345678")},
		"Origin", "https://evil.example")
	if code != http.StatusForbidden {
		t.Fatalf("login CSRF from a foreign origin must be refused: %d", code)
	}
}

func TestAuthFlow_MustChangePasswordSandbox(t *testing.T) {
	e := newAuthFlowEnv(t)
	e.db.Exec("UPDATE user_accounts SET must_change_password = 1 WHERE name = 'admin'")
	_, resp := e.post(t, "/api/login", "", map[string]string{"name": "admin", "passwordHash": service.HashPassword("12345678")})
	var data map[string]interface{}
	_ = json.Unmarshal(resp.Data, &data)
	if data["mustChangePassword"] != true {
		t.Fatalf("login must report mustChangePassword: %v", data)
	}
	token := data["token"].(string)
	if code, _ := e.post(t, "/api/v1alpha1/user/create", token, map[string]string{"name": "x", "passwordHash": service.HashPassword("Str0ng-Pass#1")}); code != http.StatusForbidden {
		t.Fatalf("sandboxed session must not reach other APIs: %d", code)
	}
	if code, r := e.post(t, "/api/v1alpha1/user/get", token, nil); code != 200 || r.Status.Code != 0 {
		t.Fatalf("user/get must stay reachable: %d", code)
	}
	code, r := e.post(t, "/api/v1alpha1/user/updatePwd", token, map[string]string{
		"oldPasswordHash": service.HashPassword("12345678"), "newPasswordHash": service.HashPassword("Mine-Pass#7"),
		"confirmPasswordHash": service.HashPassword("Mine-Pass#7"),
	})
	if code != 200 || r.Status.Code != 0 {
		t.Fatalf("updatePwd: %d %+v", code, r.Status)
	}
	_, resp = e.post(t, "/api/login", "", map[string]string{"name": "admin", "passwordHash": service.HashPassword("Mine-Pass#7")})
	data = map[string]interface{}{}
	_ = json.Unmarshal(resp.Data, &data)
	if _, set := data["mustChangePassword"]; set {
		t.Fatalf("flag must be cleared: %v", data)
	}
}
