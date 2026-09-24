package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func TestCanonicalJSONKey(t *testing.T) {
	same := []string{"projectId", "project_id", "projectid", "PROJECTID", "Project_Id", "_project__id_"}
	for _, k := range same {
		if CanonicalJSONKey(k) != CanonicalJSONKey("projectId") {
			t.Errorf("%q should alias projectId", k)
		}
	}
	// Kelvin sign K (U+212A) folds to k like encoding/json does.
	if CanonicalJSONKey("Key") != CanonicalJSONKey("key") {
		t.Error("unicode fold must match encoding/json")
	}
	if CanonicalJSONKey("projectId") == CanonicalJSONKey("projectIds") {
		t.Error("different names must not alias")
	}
}

func TestCheckJSONKeys(t *testing.T) {
	bad := []string{
		`{"projectId":"a","projectid":"b"}`,
		`{"projectId":"a","project_id":"b"}`,
		`{"projectId":"a","projectId":"b"}`,
		`{"x":{"nodeId":"a","NODE_ID":"b"}}`,
		`[{"a":1},{"b":1,"B":2}]`,
		`{"projectId":"a","projectId":"b"}`,
		`{"a":1} {"a":2}`,
		`{"a":1`,
		`{"a":1}garbage`,
	}
	for _, b := range bad {
		if CheckJSONKeys([]byte(b)) == nil {
			t.Errorf("expected rejection: %s", b)
		}
	}
	good := []string{
		`{"projectId":"a","nodeId":"b","x":{"projectId":"c"},"y":[{"projectId":"d"},{"projectId":"e"}]}`,
		// Same-valued aliases (objects echoed back from twin-key responses).
		`{"nodeId":"a","node_id":"a","n":1,"N":1,"o":{"a":[1, 2]},"O":{"a":[1,2]}}`,
		`{"projectId" : "a" , "project_id":"a"}`,
		`[]`, `{}`, `[1,"a",{"k":null}]`, ` {"a":[{"a":1}],"b":{"a":{"a":2}}} `,
	}
	for _, g := range good {
		if err := CheckJSONKeys([]byte(g)); err != nil {
			t.Errorf("unexpected rejection of %s: %v", g, err)
		}
	}
}

// guardRouter mirrors the production chain: CaseKeysRequest -> auth ->
// DataResourceGuard -> handler. The handler reports what several typical
// binding shapes receive for the guarded field.
type bound struct {
	reached  bool
	verified string
	values   []string // successfully bound, non-empty values
}

func guardRouter(u *auth.UserContext, withCaseKeys bool, out *bound) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if withCaseKeys {
		r.Use(CaseKeysRequest())
	}
	r.Use(func(c *gin.Context) { SetUser(c, u); c.Next() })
	r.Use(DataResourceGuard(nil))
	h := func(field, snake string) gin.HandlerFunc {
		return func(c *gin.Context) {
			out.reached = true
			out.verified, _ = VerifiedResourceID(c, field)
			body, _ := io.ReadAll(c.Request.Body)
			var camel struct {
				V string `json:"nodeId"`
				O string `json:"ownerId"`
			}
			var sn struct {
				V string `json:"node_id"`
				O string `json:"owner_id"`
			}
			var lower struct {
				V string `json:"nodeid"`
				O string `json:"ownerid"`
			}
			pick := func(v, o string) string {
				if field == "ownerId" {
					return o
				}
				return v
			}
			if binding.JSON.BindBody(body, &camel) == nil && pick(camel.V, camel.O) != "" {
				out.values = append(out.values, pick(camel.V, camel.O))
			}
			if binding.JSON.BindBody(body, &sn) == nil && pick(sn.V, sn.O) != "" {
				out.values = append(out.values, pick(sn.V, sn.O))
			}
			if binding.JSON.BindBody(body, &lower) == nil && pick(lower.V, lower.O) != "" {
				out.values = append(out.values, pick(lower.V, lower.O))
			}
			_ = snake
			c.JSON(200, gin.H{"status": gin.H{"code": 0}})
		}
	}
	r.POST("/api/v1alpha1/node/get", h("nodeId", "node_id"))
	r.POST("/api/v1alpha1/datasource/list", h("ownerId", "owner_id"))
	return r
}

func doPost(r *gin.Engine, path, ct string, body []byte) (int, int) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	r.ServeHTTP(w, req)
	var out struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Status.Code
}

var edgeMine = &auth.UserContext{Name: "eu", OwnerType: auth.OwnerTypeEdge, OwnerID: "mine", PlatformType: auth.PlatformTypeCenter}

// TestParamConfusionPoC is the regression test for the confirmed IDOR: the
// guard checked "mine" while the handler bound "victim".
func TestParamConfusionPoC(t *testing.T) {
	SetDataResourceAuth(nil) // node check: ownerId == nodeId
	for _, withCK := range []bool{true, false} {
		for _, body := range []string{
			`{"nodeId":"mine","nodeid":"victim"}`,
			`{"nodeId":"mine","node_id":"victim"}`,
			`{"node_id":"victim","nodeId":"mine"}`,
			`{"nodeId":"mine","NodeId":"victim"}`,
			`{"nodeId":"mine","nodeId":"victim"}`,
			`{"nodeId":"mine"}{"nodeId":"victim"}`,
		} {
			for _, ct := range []string{"application/json", "text/plain", ""} {
				var b bound
				r := guardRouter(edgeMine, withCK, &b)
				_, code := doPost(r, "/api/v1alpha1/node/get", ct, []byte(body))
				if b.reached {
					t.Errorf("caseKeys=%v ct=%q body %s reached the handler (values %v)", withCK, ct, body, b.values)
				}
				if code == 0 {
					t.Errorf("caseKeys=%v ct=%q body %s not rejected", withCK, ct, body)
				}
			}
		}
	}
	// The legitimate request still works and the verified id is exposed.
	var b bound
	r := guardRouter(edgeMine, true, &b)
	if _, code := doPost(r, "/api/v1alpha1/node/get", "application/json", []byte(`{"nodeId":"mine"}`)); code != 0 || !b.reached || b.verified != "mine" {
		t.Fatalf("own id rejected: code=%d reached=%v verified=%q", code, b.reached, b.verified)
	}
	for _, v := range b.values {
		if v != "mine" {
			t.Errorf("handler bound %q", v)
		}
	}
	// Foreign id denied with AUTH_FAILED.
	b = bound{}
	r = guardRouter(edgeMine, true, &b)
	if _, code := doPost(r, "/api/v1alpha1/node/get", "application/json", []byte(`{"node_id":"victim"}`)); code != errcode.AuthFailed.Code || b.reached {
		t.Errorf("foreign id not denied: code=%d", code)
	}
}

func TestCaseKeysRequest_RejectsDuplicatesGlobally(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CaseKeysRequest())
	reached := false
	r.POST("/x", func(c *gin.Context) { reached = true; c.JSON(200, gin.H{"status": gin.H{"code": 0}}) })
	for _, body := range []string{`{"a":{"projectId":"1","project_id":"2"}}`, `{"a":1}x`} {
		reached = false
		if _, code := doPost(r, "/x", "application/json", []byte(body)); code != errcode.ParamError.Code || reached {
			t.Errorf("%s: code=%d reached=%v", body, code, reached)
		}
	}
	reached = false
	if _, code := doPost(r, "/x", "application/json", []byte(`{"projectId":"1","x":{"projectId":"2"}}`)); code != 0 || !reached {
		t.Errorf("nested same key in different objects must pass: code=%d", code)
	}
	// Multipart bodies are not JSON-scanned.
	reached = false
	if _, code := doPost(r, "/x", "multipart/form-data; boundary=x", []byte(`{"a":1,"a":2}`)); code != 0 || !reached {
		t.Errorf("multipart must pass through: code=%d", code)
	}
}

func jsonKV(k, v string) string {
	kb, _ := json.Marshal(k)
	vb, _ := json.Marshal(v)
	return string(kb) + ":" + string(vb)
}

// FuzzGuardBindingConsistency asserts, for arbitrary bodies, that whenever the
// handler runs every id a handler binding receives equals the id the guard
// verified — and that the guard only lets the owner's own id through.
func FuzzGuardBindingConsistency(f *testing.F) {
	seeds := []struct {
		body           string
		k1, v1, k2, v2 string
		ct             uint8
	}{
		{`{"nodeId":"mine","nodeid":"victim"}`, "nodeId", "mine", "nodeid", "victim", 0},
		{`{"nodeId":"mine","node_id":"victim"}`, "nodeId", "mine", "node_id", "victim", 1},
		{`{"ownerId":"mine","owner_id":"victim"}`, "ownerId", "mine", "OWNER_ID", "victim", 2},
		{`{"nodeId":"mine"}`, "nodeId", "mine", "x", "y", 0},
		{`{"nodeId":"mine","NODEID":"victim"}`, "nodeKd", "victim", "nodeId", "mine", 0},
		{`{"nodeId":"mine"} {"nodeId":"victim"}`, "node_Id", "victim", "nodeİd", "mine", 1},
	}
	for _, s := range seeds {
		f.Add([]byte(s.body), s.k1, s.v1, s.k2, s.v2, s.ct)
	}
	SetDataResourceAuth(nil)
	cts := []string{"application/json", "text/plain", ""}
	f.Fuzz(func(t *testing.T, body []byte, k1, v1, k2, v2 string, ct uint8) {
		built := []byte("{" + jsonKV(k1, v1) + "," + jsonKV(k2, v2) + "}")
		for _, b := range [][]byte{body, built} {
			for _, path := range []string{"/api/v1alpha1/node/get", "/api/v1alpha1/datasource/list"} {
				for _, withCK := range []bool{true, false} {
					var got bound
					r := guardRouter(edgeMine, withCK, &got)
					doPost(r, path, cts[int(ct)%len(cts)], b)
					if !got.reached {
						continue
					}
					if got.verified != "mine" {
						t.Fatalf("handler reached with verified id %q (body %q)", got.verified, b)
					}
					for _, v := range got.values {
						if v != got.verified {
							t.Fatalf("guard verified %q but handler bound %q (path %s, caseKeys=%v, body %q)",
								got.verified, v, path, withCK, b)
						}
					}
				}
			}
		}
	})
}

func TestVerifyRouteCoverage(t *testing.T) {
	ok := gin.RoutesInfo{{Method: "POST", Path: "/api/v1alpha1/project/get"}, {Method: "POST", Path: "/api/v1alpha1/project/list"}}
	if err := VerifyRouteCoverage(ok); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	bad := gin.RoutesInfo{{Method: "POST", Path: "/api/v1alpha1/brand/new"}}
	err := VerifyRouteCoverage(bad)
	if err == nil || !strings.Contains(err.Error(), "not classified for RBAC") || !strings.Contains(err.Error(), "DataResourceExempt") {
		t.Fatalf("unclassified route must fail: %v", err)
	}
	for p := range DataResourceRoutes {
		if _, dup := DataResourceExempt[p]; dup {
			t.Errorf("%s both guarded and exempt", p)
		}
	}
}

func TestRBAC_DefaultDenyUnclassified(t *testing.T) {
	db := setupRBACTestDB(t)
	u := &auth.UserContext{Name: "admin", OwnerType: auth.OwnerTypeCenter, OwnerID: "kuscia-system"}
	r := rbacRouter(db, fakePlatform{}, u, "/api/v1alpha1/brand/new", "/api/v1alpha1/project/list")
	if code := postCode(t, r, "/api/v1alpha1/brand/new"); code != errcode.AuthFailed.Code {
		t.Errorf("unclassified route must be denied even for CENTER, got %d", code)
	}
	if code := postCode(t, r, "/api/v1alpha1/project/list"); code != 0 {
		t.Errorf("classified route denied: %d", code)
	}
}

// TestDataResourceGuard_CenterSemantics pins the Java DataResourceAspect
// semantics: ownerType CENTER bypasses, EDGE / P2P accounts on a CENTER
// platform are checked, a missing identity is denied.
func TestDataResourceGuard_CenterSemantics(t *testing.T) {
	SetDataResourceAuth(nil)
	cases := []struct {
		u      *auth.UserContext
		reach  bool
		reason string
	}{
		{&auth.UserContext{Name: "admin", OwnerType: auth.OwnerTypeCenter, OwnerID: "kuscia-system", PlatformType: auth.PlatformTypeCenter}, true, "center owner bypasses"},
		{&auth.UserContext{Name: "eu", OwnerType: auth.OwnerTypeEdge, OwnerID: "alice", PlatformType: auth.PlatformTypeCenter}, false, "edge account on CENTER is checked"},
		{&auth.UserContext{Name: "pu", OwnerType: auth.OwnerTypeP2P, OwnerID: "alice", PlatformType: auth.PlatformTypeCenter}, false, "p2p account on CENTER is checked"},
		{nil, false, "no identity is denied"},
	}
	for _, tc := range cases {
		var b bound
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(CaseKeysRequest())
		r.Use(func(c *gin.Context) {
			if tc.u != nil {
				SetUser(c, tc.u)
			}
			c.Next()
		})
		r.Use(DataResourceGuard(nil))
		r.POST("/api/v1alpha1/node/get", func(c *gin.Context) { b.reached = true; c.JSON(200, gin.H{}) })
		doPost(r, "/api/v1alpha1/node/get", "application/json", []byte(`{"nodeId":"bob"}`))
		if b.reached != tc.reach {
			t.Errorf("%s: reached=%v", tc.reason, b.reached)
		}
	}
}
