package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestToSnakeCase(t *testing.T) {
	cases := map[string]string{
		"projectId":      "project_id",
		"datatableId":    "datatable_id",
		"scheduleTaskId": "schedule_task_id",
		"name":           "name",
		"already_snake":  "already_snake",
		"":               "",
		"userID":         "user_i_d", // consecutive uppercase splits per-rune (acceptable; frontend uses userId)
	}
	for in, want := range cases {
		if got := toSnakeCase(in); got != want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	cases := map[string]string{
		"project_id":         "projectId",
		"datatable_id":       "datatableId",
		"schedule_task_id":   "scheduleTaskId",
		"name":               "name",
		"alreadyCamel":       "alreadyCamel",
		"":                   "",
		"gmt_create":         "gmtCreate",
		"push_to_tee_status": "pushToTeeStatus",
	}
	for in, want := range cases {
		if got := toCamelCase(in); got != want {
			t.Errorf("toCamelCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandKeys_AdditiveTwinKeys(t *testing.T) {
	in := map[string]interface{}{
		"projectId": "p1",
		"nested": map[string]interface{}{
			"datatableId": "d1",
		},
		"list": []interface{}{
			map[string]interface{}{"nodeId": "n1"},
		},
	}
	out := expandKeys(in).(map[string]interface{})

	// original camelCase keys preserved
	if out["projectId"] != "p1" {
		t.Errorf("original projectId lost: %v", out)
	}
	// snake_case twin injected
	if out["project_id"] != "p1" {
		t.Errorf("project_id twin missing: %v", out)
	}
	// nested object expanded
	nested := out["nested"].(map[string]interface{})
	if nested["datatable_id"] != "d1" {
		t.Errorf("nested datatable_id twin missing: %v", nested)
	}
	// array element expanded
	list := out["list"].([]interface{})
	elem := list[0].(map[string]interface{})
	if elem["node_id"] != "n1" {
		t.Errorf("array elem node_id twin missing: %v", elem)
	}
}

func TestCamelCaseResponseKeys_AdditiveTwinKeys(t *testing.T) {
	in := map[string]interface{}{
		"project_id": "p1",
		"nodes": []interface{}{
			map[string]interface{}{"graph_node_id": "g1"},
		},
	}
	out := camelCaseResponseKeys(in).(map[string]interface{})

	if out["project_id"] != "p1" {
		t.Errorf("original project_id lost: %v", out)
	}
	if out["projectId"] != "p1" {
		t.Errorf("projectId twin missing: %v", out)
	}
	nodes := out["nodes"].([]interface{})
	elem := nodes[0].(map[string]interface{})
	if elem["graphNodeId"] != "g1" {
		t.Errorf("nested graphNodeId twin missing: %v", elem)
	}
}

// newCaseRouter builds a gin engine wired with both case middlewares and an
// echo handler that binds a snake_case required field and returns a snake_case
// body — mirroring the legacy project/get handler shape.
func newCaseRouter() *gin.Engine {
	r := gin.New()
	r.Use(CaseKeysResponse())
	r.Use(CaseKeysRequest())
	r.POST("/api/v1alpha1/project/get", func(c *gin.Context) {
		var req struct {
			ProjectID string `json:"project_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": gin.H{"code": 1, "msg": "param error"}})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": gin.H{"code": 0, "msg": "success"},
			"data":   gin.H{"project_id": req.ProjectID, "name": "demo", "compute_mode": "mpc"},
		})
	})
	return r
}

func TestCaseMiddleware_EndToEnd_CamelRequestSnakeHandler(t *testing.T) {
	r := newCaseRouter()

	// Frontend sends camelCase {projectId}; handler binds snake_case project_id.
	body, _ := json.Marshal(map[string]string{"projectId": "p123"})
	req, _ := http.NewRequest("POST", "/api/v1alpha1/project/get", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (request twin key should satisfy required binding), got %d; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v; body=%s", err, w.Body.String())
	}
	if resp.Status.Code != 0 {
		t.Fatalf("expected status.code 0, got %d; body=%s", resp.Status.Code, w.Body.String())
	}
	// Response must expose camelCase projectId (frontend Zod requires it) while
	// keeping the original snake_case project_id.
	if resp.Data["projectId"] != "p123" {
		t.Errorf("response missing camelCase projectId: %v", resp.Data)
	}
	if resp.Data["project_id"] != "p123" {
		t.Errorf("response lost original project_id: %v", resp.Data)
	}
	if resp.Data["computeMode"] != "mpc" {
		t.Errorf("response missing camelCase computeMode: %v", resp.Data)
	}
}

func TestCaseMiddleware_NonAPIPathSkipped(t *testing.T) {
	r := gin.New()
	r.Use(CaseKeysResponse())
	r.GET("/index.html", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<html>some_id</html>"))
	})

	req, _ := http.NewRequest("GET", "/index.html", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Non-API path must be passed through verbatim (no buffering/rewriting).
	if !strings.Contains(w.Body.String(), "some_id") {
		t.Errorf("non-API body altered: %s", w.Body.String())
	}
}

func TestCaseMiddleware_NonJSONRequestUntouched(t *testing.T) {
	r := gin.New()
	r.Use(CaseKeysRequest())
	var gotContentType string
	var gotBody string
	r.POST("/api/v1alpha1/upload", func(c *gin.Context) {
		gotContentType = c.GetHeader("Content-Type")
		b, _ := c.GetRawData()
		gotBody = string(b)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// multipart content type must not be parsed/rewritten.
	req, _ := http.NewRequest("POST", "/api/v1alpha1/upload", strings.NewReader("raw-bytes"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if gotContentType == "" || !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("multipart content type lost: %q", gotContentType)
	}
	if gotBody != "raw-bytes" {
		t.Errorf("multipart body altered: %q", gotBody)
	}
}
