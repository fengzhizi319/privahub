package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/gin-gonic/gin"
)

func withBrowserCfg(t *testing.T, cfg BrowserSessionConfig) {
	t.Helper()
	prev := GetBrowserSessionConfig()
	SetBrowserSessionConfig(cfg)
	t.Cleanup(func() { SetBrowserSessionConfig(prev) })
}

func browserRouter(jwtMgr *auth.JWTManager, s SessionValidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SessionAuth(jwtMgr, s, true))
	h := func(c *gin.Context) {
		c.JSON(200, gin.H{"status": gin.H{"code": 0}, "data": CurrentUser(c).Name, "cookie": c.GetBool(CtxAuthViaCookie)})
	}
	r.POST("/api/v1alpha1/project/list", h)
	r.GET("/api/v1alpha1/project/list", h)
	r.POST("/api/v1alpha1/user/get", h)
	r.POST("/api/v1alpha1/user/updatePwd", h)
	r.POST("/api/v1alpha1/user/logout", h)
	return r
}

type reqOpt func(*http.Request)

func cookie(name, v string) reqOpt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: v}) }
}
func header(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

func serve(r *gin.Engine, method, path string, opts ...reqOpt) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://privahub.example"+path, strings.NewReader("{}"))
	for _, o := range opts {
		o(req)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCookieSession_CSRFAndOrigin(t *testing.T) {
	withBrowserCfg(t, BrowserSessionConfig{AllowedOrigins: []string{"https://console.example", "*"}})
	jwtMgr := auth.NewJWTManager("secret-0123456789abcd", time.Hour, time.Hour)
	pair, _ := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	r := browserRouter(jwtMgr, &fakeSessions{valid: map[string]*auth.UserContext{pair.AccessToken: {Name: "admin"}}})
	sess := cookie(SessionCookiePlain, pair.AccessToken)
	csrf := cookie(CSRFCookiePlain, "tok123")

	cases := []struct {
		name   string
		method string
		opts   []reqOpt
		want   int
	}{
		{"GET needs no CSRF", http.MethodGet, []reqOpt{sess}, 200},
		{"POST without CSRF header", http.MethodPost, []reqOpt{sess, csrf, header("Origin", "http://privahub.example")}, 403},
		{"POST with mismatching CSRF", http.MethodPost, []reqOpt{sess, csrf, header("Origin", "http://privahub.example"), header(CSRFHeader, "other")}, 403},
		{"POST without CSRF cookie", http.MethodPost, []reqOpt{sess, header("Origin", "http://privahub.example"), header(CSRFHeader, "tok123")}, 403},
		{"POST same origin + CSRF", http.MethodPost, []reqOpt{sess, csrf, header("Origin", "http://privahub.example"), header(CSRFHeader, "tok123")}, 200},
		{"POST allowlisted origin + CSRF", http.MethodPost, []reqOpt{sess, csrf, header("Origin", "https://console.example/"), header(CSRFHeader, "tok123")}, 200},
		{"POST Referer fallback", http.MethodPost, []reqOpt{sess, csrf, header("Referer", "https://console.example/x/y"), header(CSRFHeader, "tok123")}, 200},
		{"POST foreign origin", http.MethodPost, []reqOpt{sess, csrf, header("Origin", "https://evil.example"), header(CSRFHeader, "tok123")}, 403},
		{"POST without Origin/Referer", http.MethodPost, []reqOpt{sess, csrf, header(CSRFHeader, "tok123")}, 403},
		{"Bearer needs no CSRF", http.MethodPost, []reqOpt{header("Authorization", "Bearer "+pair.AccessToken), header("Origin", "https://evil.example")}, 200},
		{"__Host- cookie ignored over plain HTTP", http.MethodGet, []reqOpt{cookie(SessionCookieSecure, pair.AccessToken)}, 401},
	}
	for _, c := range cases {
		w := serve(r, c.method, "/api/v1alpha1/project/list", c.opts...)
		if w.Code != c.want {
			t.Errorf("%s: code %d want %d (%s)", c.name, w.Code, c.want, w.Body.String())
		}
	}
}

func TestCookieSession_SecureModeUsesHostPrefix(t *testing.T) {
	yes := true
	withBrowserCfg(t, BrowserSessionConfig{Secure: &yes})
	jwtMgr := auth.NewJWTManager("secret-0123456789abcd", time.Hour, time.Hour)
	pair, _ := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	r := browserRouter(jwtMgr, &fakeSessions{valid: map[string]*auth.UserContext{pair.AccessToken: {Name: "admin"}}})
	if w := serve(r, http.MethodGet, "/api/v1alpha1/project/list", cookie(SessionCookieSecure, pair.AccessToken)); w.Code != 200 {
		t.Fatalf("__Host- cookie: %d", w.Code)
	}
	// Plain-name cookie (e.g. tossed from a sibling domain) is ignored in Secure mode.
	if w := serve(r, http.MethodGet, "/api/v1alpha1/project/list", cookie(SessionCookiePlain, pair.AccessToken)); w.Code != 401 {
		t.Fatalf("plain cookie in secure mode: %d", w.Code)
	}
}

func TestIssueBrowserSession_CookieAttributes(t *testing.T) {
	yes := true
	withBrowserCfg(t, BrowserSessionConfig{Secure: &yes, AccessMaxAge: time.Hour, RefreshMaxAge: 2 * time.Hour})
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/login", nil)
	if err := IssueBrowserSession(c, "acc", "ref", true); err != nil {
		t.Fatal(err)
	}
	got := map[string]*http.Cookie{}
	for _, ck := range w.Result().Cookies() {
		got[ck.Name] = ck
	}
	s, rf, x := got[SessionCookieSecure], got[RefreshCookieSecure], got[CSRFCookieSecure]
	if s == nil || rf == nil || x == nil {
		t.Fatalf("cookies: %v", w.Header()["Set-Cookie"])
	}
	for _, ck := range []*http.Cookie{s, rf, x} {
		if !ck.Secure || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" || ck.Domain != "" {
			t.Errorf("%s attributes: %+v", ck.Name, ck)
		}
	}
	if !s.HttpOnly || !rf.HttpOnly || x.HttpOnly || len(x.Value) != 64 {
		t.Errorf("HttpOnly/CSRF value wrong: %+v %+v %+v", s, rf, x)
	}

	// Logout clears both name variants.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	ClearBrowserSession(c2)
	if n := len(w2.Result().Cookies()); n != 6 {
		t.Errorf("expected 6 expiring cookies, got %d", n)
	}
}

func TestMustChangePassword_Sandbox(t *testing.T) {
	jwtMgr := auth.NewJWTManager("secret-0123456789abcd", time.Hour, time.Hour)
	pair, _ := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	r := browserRouter(jwtMgr, &fakeSessions{valid: map[string]*auth.UserContext{pair.AccessToken: {Name: "admin", MustChangePassword: true}}})
	bearer := header("Authorization", "Bearer "+pair.AccessToken)
	for path, want := range map[string]int{
		"/api/v1alpha1/project/list":   403,
		"/api/v1alpha1/user/get":       200,
		"/api/v1alpha1/user/updatePwd": 200,
		"/api/v1alpha1/user/logout":    200,
	} {
		w := serve(r, http.MethodPost, path, bearer)
		if w.Code != want {
			t.Errorf("%s: %d want %d", path, w.Code, want)
		}
		if want == 403 && !strings.Contains(w.Body.String(), "202011605") {
			t.Errorf("%s: expected PASSWORD_CHANGE_REQUIRED code: %s", path, w.Body.String())
		}
	}
}

func TestSessionAuth_RejectsStalePasswordVersion(t *testing.T) {
	jwtMgr := auth.NewJWTManager("secret-0123456789abcd", time.Hour, time.Hour)
	pair, _ := jwtMgr.GenerateSessionTokens(auth.SessionTokenParams{Username: "admin", PwdVer: 1})
	r := browserRouter(jwtMgr, &fakeSessions{valid: map[string]*auth.UserContext{pair.AccessToken: {Name: "admin", PwdVer: 2}}})
	if w := serve(r, http.MethodGet, "/api/v1alpha1/project/list", header("Authorization", "Bearer "+pair.AccessToken)); w.Code != 401 {
		t.Fatalf("token from an older password version must be rejected: %d", w.Code)
	}
}

func TestVerifyLoginOrigin(t *testing.T) {
	withBrowserCfg(t, BrowserSessionConfig{AllowedOrigins: []string{"https://console.example"}})
	gin.SetMode(gin.TestMode)
	for origin, ok := range map[string]bool{"": true, "https://console.example": true, "http://privahub.example": true, "https://evil.example": false} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "http://privahub.example/api/login", nil)
		if origin != "" {
			c.Request.Header.Set("Origin", origin)
		}
		if got := VerifyLoginOrigin(c) == ""; got != ok {
			t.Errorf("origin %q: allowed=%v want %v", origin, got, ok)
		}
	}
}
