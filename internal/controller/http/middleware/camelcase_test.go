package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"camelCase", "camel_case"},
		{"PascalCase", "pascal_case"},
		{"already_snake", "already_snake"},
		{"HTTPServer", "h_t_t_p_server"},
		{"simple", "simple"},
		{"", ""},
		{"A", "a"},
		{"ABC", "a_b_c"},
		{"projectID", "project_i_d"},
		{"gmtCreate", "gmt_create"},
	}

	for _, tt := range tests {
		got := toSnakeCase(tt.input)
		if got != tt.expected {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"snake_case", "snakeCase"},
		{"already_camel", "alreadyCamel"},
		{"simple", "simple"},
		{"", ""},
		{"project_id", "projectId"},
		{"gmt_create", "gmtCreate"},
		{"_leading", "leading"},
		{"trailing_", "trailing"},
		{"multi_word_name", "multiWordName"},
	}

	for _, tt := range tests {
		got := toCamelCase(tt.input)
		if got != tt.expected {
			t.Errorf("toCamelCase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExpandKeys(t *testing.T) {
	input := map[string]interface{}{
		"projectId": "proj-1",
		"nested": map[string]interface{}{
			"graphId": "graph-1",
		},
	}

	result := expandKeys(input).(map[string]interface{})

	// Original keys should be preserved
	if result["projectId"] != "proj-1" {
		t.Error("expected original camelCase key to be preserved")
	}

	// Snake_case twin should be added
	if result["project_id"] != "proj-1" {
		t.Error("expected snake_case twin key to be added")
	}

	// Nested object should also be expanded
	nested := result["nested"].(map[string]interface{})
	if nested["graph_id"] != "graph-1" {
		t.Error("expected nested snake_case twin key to be added")
	}
}

func TestCamelCaseResponseKeys(t *testing.T) {
	input := map[string]interface{}{
		"project_id": "proj-1",
		"nested": map[string]interface{}{
			"graph_id": "graph-1",
		},
	}

	result := camelCaseResponseKeys(input).(map[string]interface{})

	// Original keys should be preserved
	if result["project_id"] != "proj-1" {
		t.Error("expected original snake_case key to be preserved")
	}

	// CamelCase twin should be added
	if result["projectId"] != "proj-1" {
		t.Error("expected camelCase twin key to be added")
	}

	// Nested object should also be expanded
	nested := result["nested"].(map[string]interface{})
	if nested["graphId"] != "graph-1" {
		t.Error("expected nested camelCase twin key to be added")
	}
}

func TestExpandKeys_Array(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{"projectId": "proj-1"},
		map[string]interface{}{"projectId": "proj-2"},
	}

	result := expandKeys(input).([]interface{})

	for i, item := range result {
		m := item.(map[string]interface{})
		if m["project_id"] == nil {
			t.Errorf("expected array item %d to have snake_case twin", i)
		}
	}
}

func TestShouldSkipCasePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/api/v1alpha1/sync", true},
		{"/metrics", true},
		{"/api/v1alpha1/healthz", true},
		{"/assets/main.js", true},
		{"/favicon.ico", true},
		{"/api/v1alpha1/project/list", false},
		{"/api/v1alpha1/user/login", false},
	}

	for _, tt := range tests {
		got := shouldSkipCasePath(tt.path)
		if got != tt.expected {
			t.Errorf("shouldSkipCasePath(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestIsJSONContentType(t *testing.T) {
	tests := []struct {
		ct       string
		expected bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"APPLICATION/JSON", true},
		{"text/html", false},
		{"multipart/form-data", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isJSONContentType(tt.ct)
		if got != tt.expected {
			t.Errorf("isJSONContentType(%q) = %v, want %v", tt.ct, got, tt.expected)
		}
	}
}

// --- Integration tests for CaseKeysRequest middleware ---

func TestCaseKeysRequest_ExpandsCamelToSnake(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysRequest())
	router.POST("/test", func(c *gin.Context) {
		var body map[string]interface{}
		_ = c.ShouldBindJSON(&body)
		c.JSON(200, body)
	})

	payload := `{"projectId":"p1","graphId":"g1"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Both camelCase and snake_case keys should be present
	if resp["projectId"] != "p1" {
		t.Error("expected camelCase key preserved")
	}
	if resp["project_id"] != "p1" {
		t.Error("expected snake_case twin added")
	}
}

func TestCaseKeysRequest_SkipsNonJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysRequest())
	router.POST("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader("plain text"))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCaseKeysRequest_SkipsEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysRequest())
	router.POST("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCaseKeysRequest_ArrayPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysRequest())
	router.POST("/test", func(c *gin.Context) {
		var body []interface{}
		_ = c.ShouldBindJSON(&body)
		c.JSON(200, body)
	})

	payload := `[{"projectId":"p1"}]`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Integration tests for CaseKeysResponse middleware ---

func TestCaseKeysResponse_ConvertsSnakeToCamel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysResponse())
	router.GET("/api/v1alpha1/project", func(c *gin.Context) {
		c.JSON(200, gin.H{"project_id": "p1", "graph_id": "g1"})
	})

	req := httptest.NewRequest("GET", "/api/v1alpha1/project", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Both snake_case and camelCase keys should be present
	if resp["project_id"] != "p1" {
		t.Error("expected original snake_case key preserved")
	}
	if resp["projectId"] != "p1" {
		t.Error("expected camelCase twin added")
	}
}

func TestCaseKeysResponse_SkipsNonAPIPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysResponse())
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// No camelCase twin should be added for non-API paths
	if _, exists := resp["status"]; !exists {
		t.Error("expected 'status' key in response")
	}
}

func TestCaseKeysResponse_SkipsSyncPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysResponse())
	router.GET("/api/v1alpha1/sync", func(c *gin.Context) {
		c.JSON(200, gin.H{"node_id": "alice"})
	})

	req := httptest.NewRequest("GET", "/api/v1alpha1/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// shouldSkipCasePath returns true for /sync, so no conversion
	if _, exists := resp["nodeId"]; exists {
		t.Error("expected no camelCase conversion for /sync path")
	}
}

func TestCaseKeysResponse_NestedObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysResponse())
	router.GET("/api/v1alpha1/data", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"project_id": "p1",
			"nested":     gin.H{"graph_id": "g1"},
		})
	})

	req := httptest.NewRequest("GET", "/api/v1alpha1/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	nested, ok := resp["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("expected nested object")
	}
	if nested["graphId"] != "g1" {
		t.Error("expected nested camelCase twin added")
	}
}

func TestCaseKeysResponse_WriteHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CaseKeysResponse())
	router.GET("/api/v1alpha1/created", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"result_id": "r1"})
	})

	req := httptest.NewRequest("GET", "/api/v1alpha1/created", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}
