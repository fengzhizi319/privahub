package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/internal/wire"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// javaDataResourceRoutes are the Java controller methods annotated with
// @DataResource (secretpad-web controllers), by field.
var javaDataResourceRoutes = []string{
	// ProjectController (15)
	"/api/v1alpha1/project/get", "/api/v1alpha1/project/update", "/api/v1alpha1/project/delete",
	"/api/v1alpha1/project/inst/add", "/api/v1alpha1/project/node/add", "/api/v1alpha1/project/datatable/add",
	"/api/v1alpha1/project/datatable/delete", "/api/v1alpha1/project/datatable/get",
	"/api/v1alpha1/project/job/list", "/api/v1alpha1/project/job/get", "/api/v1alpha1/project/job/stop",
	"/api/v1alpha1/project/job/task/logs", "/api/v1alpha1/project/job/task/output",
	"/api/v1alpha1/project/update/tableConfig", "/api/v1alpha1/project/datasource/list",
	// GraphController (13)
	"/api/v1alpha1/graph/create", "/api/v1alpha1/graph/delete", "/api/v1alpha1/graph/list",
	"/api/v1alpha1/graph/meta/update", "/api/v1alpha1/graph/update", "/api/v1alpha1/graph/node/update",
	"/api/v1alpha1/graph/start", "/api/v1alpha1/graph/node/status", "/api/v1alpha1/graph/stop",
	"/api/v1alpha1/graph/detail", "/api/v1alpha1/graph/node/output", "/api/v1alpha1/graph/node/logs",
	"/api/v1alpha1/graph/node/max_index",
	// Scheduled (4), ModelExport (3), ModelManagement (4)
	"/api/v1alpha1/scheduled/id", "/api/v1alpha1/scheduled/graph/once/success",
	"/api/v1alpha1/scheduled/graph/create", "/api/v1alpha1/scheduled/job/list",
	"/api/v1alpha1/model/pack", "/api/v1alpha1/model/status", "/api/v1alpha1/model/modelPartyPath",
	"/api/v1alpha1/model/page", "/api/v1alpha1/model/detail", "/api/v1alpha1/model/info",
	"/api/v1alpha1/model/serving/create",
	// CloudLog, FeatureDatasource (2), P2PProject, P2PNode
	"/api/v1alpha1/cloud_log/sls", "/api/v1alpha1/feature_datasource/create",
	"/api/v1alpha1/feature_datasource/auth/list", "/api/v1alpha1/p2p/project/update",
	"/api/v1alpha1/p2p/node/delete",
	// NodeController (8), NodeRoute (1)
	"/api/v1alpha1/node/update", "/api/v1alpha1/node/get", "/api/v1alpha1/node/delete",
	"/api/v1alpha1/node/token", "/api/v1alpha1/node/newToken", "/api/v1alpha1/node/refresh",
	"/api/v1alpha1/node/result/list", "/api/v1alpha1/node/result/detail", "/api/v1alpha1/nodeRoute/page",
	// DataSource (5), Datatable (4), Data (2)
	"/api/v1alpha1/datasource/create", "/api/v1alpha1/datasource/delete", "/api/v1alpha1/datasource/list",
	"/api/v1alpha1/datasource/detail", "/api/v1alpha1/datasource/nodes",
	"/api/v1alpha1/datatable/create", "/api/v1alpha1/datatable/list", "/api/v1alpha1/datatable/get",
	"/api/v1alpha1/datatable/delete", "/api/v1alpha1/data/create", "/api/v1alpha1/data/download",
	// Inst (2), Message (4), Approval (1), NodeUser, RemoteUser
	"/api/v1alpha1/inst/get", "/api/v1alpha1/inst/node/delete",
	"/api/v1alpha1/message/reply", "/api/v1alpha1/message/list", "/api/v1alpha1/message/detail",
	"/api/v1alpha1/message/pending", "/api/v1alpha1/approval/create",
	"/api/v1alpha1/user/node/resetPassword", "/api/v1alpha1/user/remote/resetPassword",
}

func TestDataResourceRoutes_CoverJava(t *testing.T) {
	for _, p := range javaDataResourceRoutes {
		if _, ok := middleware.DataResourceRoutes[p]; !ok {
			t.Errorf("Java @DataResource route %s is not guarded", p)
		}
	}
	app := newTestApp(t)
	registered := map[string]bool{}
	for _, ri := range apiRoutes(NewRouter(zap.NewNop(), app), NewInnerRouter(zap.NewNop(), app)) {
		registered[ri.Path] = true
	}
	for p := range middleware.DataResourceRoutes {
		if !registered[p] {
			t.Errorf("DataResourceRoutes entry %s has no registered route", p)
		}
	}
}

// loginAs creates a user and returns a Bearer token for it.
func loginAs(t *testing.T, app *wire.App, name, ownerType, ownerID string) string {
	t.Helper()
	u := &model.UserAccountsDO{Name: name, PasswordHash: service.HashPassword("pw-" + name),
		OwnerType: ownerType, OwnerID: ownerID}
	if err := app.DB.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	resp, err := app.AuthService.Login(t.Context(), &service.LoginRequest{Username: name, Password: "pw-" + name})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return resp.AccessToken
}

// foreignBody fills every field a rule may read with foreign ids.
const foreignBody = `{"projectId":"foreign-project","nodeId":"foreign-node","ownerId":"foreign-node",` +
	`"instId":"foreign-node","voteParticipantId":"foreign-node","initiatorId":"foreign-node",` +
	`"graphId":"g","jobId":"j","taskId":"t","datatableId":"d","datasourceId":"ds"}`

func callAPI(t *testing.T, r *gin.Engine, token, path, body string) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	var out struct {
		Status struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Status.Code, out.Status.Msg
}

func sortedRules() []string {
	paths := make([]string, 0, len(middleware.DataResourceRoutes))
	for p := range middleware.DataResourceRoutes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// TestDataResourceGuard_RejectsForeignIDs drives the real router: a non-CENTER
// user asking for a foreign projectId / nodeId is rejected on every guarded
// route with the Java DataResource AUTH_FAILED error, before the handler runs.
func TestDataResourceGuard_RejectsForeignIDs(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		ownerType string
		skipWhite bool
	}{
		{name: "center-platform/edge-user", mode: "master", ownerType: auth.OwnerTypeEdge},
		{name: "autonomy-platform/p2p-user", mode: "autonomy", ownerType: auth.OwnerTypeP2P, skipWhite: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestAppWith(t, func(c *config.Config) {
				c.Server.Mode = tc.mode
				c.Server.NodeID = "self-node"
			})
			// The platform node owner passes interface RBAC, isolating the data check.
			token := loginAs(t, app, "u-"+tc.mode, tc.ownerType, "self-node")
			r := NewRouter(zap.NewNop(), app)
			white := map[string]bool{}
			for _, p := range middleware.DefaultAutonomyWhitelist {
				white[p] = true
			}
			for _, path := range sortedRules() {
				if strings.HasPrefix(path, "/api/v1alpha1/user/node/") {
					continue // inner-port only; covered below
				}
				code, msg := callAPI(t, r, token, path, foreignBody)
				denied := code == errcode.AuthFailed.Code && strings.Contains(msg, "No permission to access the data")
				if tc.skipWhite && white[path] {
					// Java white-list.paths: the aspect is skipped for P2P users
					// (the handler / service may still scope the request).
					continue
				}
				if !denied {
					t.Errorf("%s: foreign id not rejected: code=%d msg=%q", path, code, msg)
				}
			}
		})
	}
}

// TestDataResourceGuard_OwnResourcesPass checks the guard lets the owner through.
func TestDataResourceGuard_OwnResourcesPass(t *testing.T) {
	app := newTestAppWith(t, func(c *config.Config) { c.Server.NodeID = "self-node" })
	app.DB.Create(&model.ProjectNodeDO{ProjectID: "own-project", NodeID: "self-node"})
	token := loginAs(t, app, "owner", auth.OwnerTypeEdge, "self-node")
	r := NewRouter(zap.NewNop(), app)
	for _, path := range []string{"/api/v1alpha1/project/get", "/api/v1alpha1/graph/list", "/api/v1alpha1/node/get"} {
		_, msg := callAPI(t, r, token, path, `{"projectId":"own-project","nodeId":"self-node"}`)
		if strings.Contains(msg, "No permission to access the data") {
			t.Errorf("%s: owner rejected: %s", path, msg)
		}
	}
	// Inner port: a node may only reset its own node user.
	inner := NewInnerRouter(zap.NewNop(), app)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/user/node/resetPassword",
		bytes.NewBufferString(`{"nodeId":"other-node","name":"x","newPasswordHash":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.KusciaOriginSourceHeader, "alice")
	inner.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "No permission to access the") {
		t.Errorf("inner resetPassword for a foreign node must be rejected: %s", w.Body.String())
	}
}

// TestVerifyRouteCoverage_RealRouters is the default-deny startup check on
// the real main + inner routers (the server refuses to start otherwise).
func TestVerifyRouteCoverage_RealRouters(t *testing.T) {
	for _, mode := range []string{"master", "lite", "autonomy"} {
		app := newTestAppWith(t, func(c *config.Config) { c.Server.Mode = mode })
		main := NewRouter(zap.NewNop(), app)
		inner := NewInnerRouter(zap.NewNop(), app)
		MountEdgeForwardTargets(inner, main, app)
		if err := VerifyRouteCoverage(main, inner); err != nil {
			t.Errorf("%s: %v", mode, err)
		}
	}
	// No stale exemptions.
	registered := map[string]bool{}
	app := newTestApp(t)
	for _, ri := range apiRoutes(NewRouter(zap.NewNop(), app), NewInnerRouter(zap.NewNop(), app)) {
		registered[ri.Path] = true
	}
	for p := range middleware.DataResourceExempt {
		if !registered[p] && p != "/api/v1alpha1/docs" {
			t.Errorf("DataResourceExempt entry %s has no registered route", p)
		}
	}
}

// TestParamConfusion_RealRouter replays the confirmed IDOR PoC against the
// real router: aliased / duplicated id keys are rejected before any guard or
// handler runs, for EDGE users and CENTER admins alike.
func TestParamConfusion_RealRouter(t *testing.T) {
	app := newTestAppWith(t, func(c *config.Config) { c.Server.NodeID = "self-node" })
	app.DB.Create(&model.ProjectNodeDO{ProjectID: "mine", NodeID: "self-node"})
	token := loginAs(t, app, "pc-user", auth.OwnerTypeEdge, "self-node")
	r := NewRouter(zap.NewNop(), app)
	for _, body := range []string{
		`{"projectId":"mine","projectid":"victim"}`,
		`{"projectId":"mine","project_id":"victim"}`,
		`{"projectId":"mine","projectId":"victim"}`,
		`{"project_id":"victim","projectId":"mine"}`,
	} {
		for _, path := range []string{"/api/v1alpha1/project/get", "/api/v1alpha1/graphDatasource/list", "/api/v1alpha1/graph/list"} {
			code, _ := callAPI(t, r, token, path, body)
			if code != errcode.ParamError.Code {
				t.Errorf("%s %s: expected ParamError, got %d", path, body, code)
			}
		}
	}
	// The plain request for the own project passes the data check.
	if _, msg := callAPI(t, r, token, "/api/v1alpha1/graphDatasource/list", `{"projectId":"mine"}`); strings.Contains(msg, "No permission") {
		t.Errorf("own project rejected: %s", msg)
	}
	if code, _ := callAPI(t, r, token, "/api/v1alpha1/graphDatasource/list", `{"project_id":"victim"}`); code != errcode.AuthFailed.Code {
		t.Errorf("foreign snake_case id must be denied, got %d", code)
	}
}

// TestFeatureTableAliasesRemoved: /featureTable/create|list bypassed the
// ownerId / nodeId checks of /feature_datasource/* and are gone.
func TestFeatureTableAliasesRemoved(t *testing.T) {
	app := newTestApp(t)
	for _, ri := range apiRoutes(NewRouter(zap.NewNop(), app), NewInnerRouter(zap.NewNop(), app)) {
		if ri.Path == "/api/v1alpha1/featureTable/create" || ri.Path == "/api/v1alpha1/featureTable/list" {
			t.Errorf("%s must not be registered", ri.Path)
		}
	}
}
