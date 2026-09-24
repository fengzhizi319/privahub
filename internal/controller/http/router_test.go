package http

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/controller/http/apidocs"
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao"
	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/wire"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// javaAPIResourceCodes are the codes of Java ApiResourceCodeConstants.
var javaAPIResourceCodes = map[string]bool{}

func init() {
	for _, c := range strings.Fields(`ALL_INTERFACE_RESOURCE NODE_UPDATE NODE_CREATE NODE_PAGE NODE_GET
	NODE_DELETE NODE_TOKEN NODE_NEW_TOKEN NODE_REFRESH NODE_LIST NODE_RESULT_LIST NODE_RESULT_DETAIL
	NODE_REGISTER INST_GET AUTH_LOGIN AUTH_LOGOUT DATA_CREATE DATA_CREATE_DATA DATA_UPLOAD DATA_DOWNLOAD
	DATA_LIST_DATASOURCE DATATABLE_CREATE DATATABLE_LIST DATATABLE_GET DATATABLE_DELETE GRAPH_COMM_I18N
	GRAPH_COMM_LIST GRAPH_COMM_GET GRAPH_COMM_BATH GRAPH_CREATE GRAPH_DELETE GRAPH_META_UPDATE GRAPH_UPDATE
	GRAPH_LIST MODEL_EXPORT GRAPH_NODE_UPDATE GRAPH_START GRAPH_NODE_STATUS GRAPH_STOP GRAPH_DETAIL
	GRAPH_NODE_OUTPUT GRAPH_NODE_LOGS INDEX NODE_ROUTE_CREATE NODE_ROUTE_PAGE NODE_ROUTE_GET
	NODE_ROUTE_UPDATE NODE_ROUTE_LIST_NODE NODE_ROUTE_REFRESH NODE_ROUTE_DELETE NODE_USER_RESET_PWD
	NODE_USER_CREATE NODE_USER_LIST_BY_NODE NODE_USER_NODE_LIST NODE_USER_REMOTE_NODE_LIST
	NODE_USER_OTHER_NODE_BASE_INFO_LIST PRJ_CREATE PRJ_LIST PRJ_GET PRJ_UPDATE PRJ_DELETE PRJ_PARTICIPANTS
	PRJ_ARCHIVE PRJ_ADD_INST PRJ_ADD_NODE PRJ_ADD_TABLE PRJ_DATATABLE_DELETE PRJ_DATATABLE_GET PRJ_JOB_LIST
	PRJ_JOB_GET PRJ_JOB_STOP PRJ_TASK_LOGS PRJ_TASK_OUTPUT REMOTE_USER_RESET_PWD REMOTE_USER_CREATE
	REMOTE_USER_LIST_BY_NODE USER_CREATE USER_GET ENV_GET USER_UPDATE_PWD MODEL_PAGE MODEL_GET MODEL_DISCARD
	MODEL_DELETE SERV_CREATE SERV_DELETE COMPONENT_VERSION_LIST SCHEDULED_ID SCHEDULED_CREATE SCHEDULED_PAGE
	SCHEDULED_OFFLINE`) {
		javaAPIResourceCodes[c] = true
	}
}

// newTestApp builds the full application on an in-memory SQLite database.
func newTestApp(t *testing.T) *wire.App {
	t.Helper()
	return newTestAppWith(t, nil)
}

// newTestAppWith is newTestApp with a config mutator applied before wiring.
func newTestAppWith(t *testing.T, mutate func(*config.Config)) *wire.App {
	t.Helper()
	t.Setenv("PRIVAHUB_PROFILE", "test")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	dir := t.TempDir()
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = "file:" + filepath.Join(dir, "router.db") + "?_pragma=busy_timeout(5000)"
	cfg.Kuscia.PoolFile = filepath.Join(dir, "nodes.json")
	cfg.Auth.InitialPasswordFile = filepath.Join(dir, "initial-password")
	cfg.Server.APIDocs.Enabled = true
	if mutate != nil {
		mutate(cfg)
	}
	log := zap.NewNop()
	db, err := dao.NewDB(&cfg.Database, log)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := migration.AutoMigrate(db, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := wire.NewApp(db, cfg, log)
	t.Cleanup(func() {
		app.Shutdown()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return app
}

func apiRoutes(engines ...*gin.Engine) []gin.RouteInfo {
	var out []gin.RouteInfo
	for _, e := range engines {
		for _, ri := range e.Routes() {
			if strings.HasPrefix(ri.Path, "/api/") {
				out = append(out, ri)
			}
		}
	}
	return out
}

// TestRouteInterfaceResourceCoverage walks every registered /api route and
// asserts it is classified for RBAC: mapped to a Java ApiResourceCode,
// admin-only, explicitly unchecked (Java has no @ApiResource), or public.
func TestRouteInterfaceResourceCoverage(t *testing.T) {
	app := newTestApp(t)
	main := NewRouter(zap.NewNop(), app)
	inner := NewInnerRouter(zap.NewNop(), app)
	registered := map[string]bool{}
	var missing []string
	for _, ri := range apiRoutes(main, inner) {
		p := ri.Path
		registered[p] = true
		n := 0
		if code, ok := middleware.APIResourceCodes[p]; ok {
			n++
			if !javaAPIResourceCodes[code] {
				t.Errorf("%s mapped to unknown Java ApiResourceCode %q", p, code)
			}
		}
		if middleware.AdminOnlyPaths[p] {
			n++
		}
		if _, ok := middleware.NoInterfaceCheckPaths[p]; ok {
			n++
		}
		if middleware.PublicPaths[p] {
			n++
		}
		switch {
		case n == 0:
			missing = append(missing, ri.Method+" "+p)
		case n > 1 && !(middleware.PublicPaths[p] && p == "/api/v1alpha1/inst/node/register"):
			t.Errorf("%s is classified %d times", p, n)
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("route not classified for RBAC: %s", m)
	}

	// No stale entries: every classified path must be a registered route.
	check := func(kind string, paths []string) {
		for _, p := range paths {
			if !registered[p] {
				t.Errorf("%s entry %s has no registered route", kind, p)
			}
		}
	}
	keys := func(m map[string]string) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	boolKeys := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	check("APIResourceCodes", keys(middleware.APIResourceCodes))
	check("NoInterfaceCheckPaths", keys(middleware.NoInterfaceCheckPaths))
	check("AdminOnlyPaths", boolKeys(middleware.AdminOnlyPaths))
	check("PublicPaths", boolKeys(middleware.PublicPaths))
}

// TestOpenAPIDocumentCoversRoutes keeps apidocs/openapi.json in sync with the
// route table. Regenerate with:
//
//	CGO_ENABLED=1 go run ./cmd/openapi-gen
func TestOpenAPIDocumentCoversRoutes(t *testing.T) {
	app := newTestApp(t)
	documented, err := apidocs.Paths()
	if err != nil {
		t.Fatalf("parse openapi.json: %v", err)
	}
	registered := map[string]bool{}
	for _, ri := range apiRoutes(NewRouter(zap.NewNop(), app), NewInnerRouter(zap.NewNop(), app)) {
		key := ri.Method + " " + ri.Path
		registered[key] = true
		if !documented[key] {
			t.Errorf("route %s missing from apidocs/openapi.json (run: CGO_ENABLED=1 go run ./cmd/openapi-gen)", key)
		}
	}
	for key := range documented {
		if !registered[key] {
			t.Errorf("apidocs/openapi.json documents unregistered route %s (regenerate it)", key)
		}
	}
}

func TestAPIDocsEndpoint(t *testing.T) {
	app := newTestApp(t)
	r := NewRouter(zap.NewNop(), app)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1alpha1/docs", nil))
	if strings.Contains(w.Body.String(), `"openapi"`) {
		t.Fatal("docs must require a session unless server.api_docs.public")
	}

	app.Config.Server.APIDocs.Public = true
	r = NewRouter(zap.NewNop(), app)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1alpha1/docs", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"openapi": "3.0.3"`) {
		t.Fatalf("public docs: %d %.200s", w.Code, w.Body.String())
	}

	app.Config.Server.APIDocs.Enabled = false
	r = NewRouter(zap.NewNop(), app)
	for _, ri := range r.Routes() {
		if ri.Path == "/api/v1alpha1/docs" {
			t.Fatal("docs route registered while disabled")
		}
	}
}
