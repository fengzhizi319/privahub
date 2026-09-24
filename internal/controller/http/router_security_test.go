package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"go.uber.org/zap"
)

func getStatus(t *testing.T, h http.Handler, method, path, remote string, hdr map[string]string, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.RemoteAddr = remote
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestMetricsAccessModes(t *testing.T) {
	for _, tc := range []struct {
		access         string
		remoteCode     int
		loopbackServed bool
	}{
		{config.MetricsPublic, 200, true},
		{config.MetricsRestricted, 403, true},
		{config.MetricsAuth, 401, false},
		{config.MetricsDisabled, 404, false},
	} {
		t.Run(tc.access, func(t *testing.T) {
			app := newTestAppWith(t, func(c *config.Config) { c.Observability.MetricsAccess = tc.access })
			r := NewRouter(zap.NewNop(), app)
			if code, _ := getStatus(t, r, http.MethodGet, "/metrics", "203.0.113.9:4000", nil, ""); code != tc.remoteCode {
				t.Fatalf("remote /metrics = %d, want %d", code, tc.remoteCode)
			}
			code, body := getStatus(t, r, http.MethodGet, "/metrics", "127.0.0.1:4000", nil, "")
			if served := code == 200 && strings.Contains(body, "go_goroutines"); served != tc.loopbackServed {
				t.Fatalf("loopback /metrics = %d served=%v", code, served)
			}
		})
	}
}

func TestInnerPortGuardToken(t *testing.T) {
	const tok = "0123456789abcdef-inner"
	app := newTestAppWith(t, func(c *config.Config) { c.Server.InnerAuth.Token = tok })
	defer innerauth.SetOutgoingToken("")
	if innerauth.OutgoingToken() != tok {
		t.Fatal("outgoing inner calls must carry the configured token")
	}
	inner := NewInnerRouter(zap.NewNop(), app)
	hdr := map[string]string{"kuscia-origin-source": "alice"}
	code, _ := getStatus(t, inner, http.MethodPost, "/api/v1alpha1/data/sync", "10.0.0.8:1", hdr, `{}`)
	if code != http.StatusUnauthorized {
		t.Fatalf("inner call without token = %d, want 401", code)
	}
	hdr[innerauth.TokenHeader] = tok
	if code, _ = getStatus(t, inner, http.MethodPost, "/api/v1alpha1/data/sync", "10.0.0.8:1", hdr, `{}`); code == http.StatusUnauthorized {
		t.Fatal("inner call with the token must pass the guard")
	}
	if code, _ = getStatus(t, inner, http.MethodGet, "/healthz", "10.0.0.8:1", nil, ""); code != 200 {
		t.Fatalf("healthz stays open, got %d", code)
	}
}

func TestInstCreateListOwnerAdmin(t *testing.T) {
	app := newTestAppWith(t, func(c *config.Config) { c.Server.NodeID = "self-node" })
	app.DB.Create(&model.InstDO{InstID: "alice", Name: "alice"})
	app.DB.Create(&model.InstDO{InstID: "bob", Name: "bob"})
	r := NewRouter(zap.NewNop(), app)
	// A non-CENTER user holding the ADMIN role: may list only its own
	// institution and cannot create one.
	token := loginAs(t, app, "inst-admin", auth.OwnerTypeEdge, "alice")
	app.DB.Create(&model.SysUserPermissionRelDO{UserType: auth.PermissionUserTypeEdgeUser, UserKey: "inst-admin",
		TargetType: "ROLE", TargetCode: auth.RoleAdmin})
	hdr := map[string]string{"Authorization": "Bearer " + token}
	_, body := getStatus(t, r, http.MethodPost, "/api/v1alpha1/inst/create", "127.0.0.1:1", hdr, `{"instId":"x1","name":"x1"}`)
	if !strings.Contains(body, "only the CENTER platform owner") {
		t.Fatalf("inst/create by a non-CENTER admin must be refused: %s", body)
	}
	_, body = getStatus(t, r, http.MethodPost, "/api/v1alpha1/inst/list", "127.0.0.1:1", hdr, `{}`)
	if strings.Contains(body, `"bob"`) || !strings.Contains(body, `"alice"`) {
		t.Fatalf("inst/list must be limited to the own institution: %s", body)
	}
	// Without the ADMIN role: refused.
	plain := loginAs(t, app, "plain", auth.OwnerTypeEdge, "alice")
	_, body = getStatus(t, r, http.MethodPost, "/api/v1alpha1/inst/list", "127.0.0.1:1",
		map[string]string{"Authorization": "Bearer " + plain}, `{}`)
	if !strings.Contains(body, "202011602") {
		t.Fatalf("inst/list without ADMIN must be refused: %s", body)
	}
}
