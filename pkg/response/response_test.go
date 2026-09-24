package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(handler gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.GET("/test", handler)
	return r
}

func TestOK(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		OK(c, gin.H{"key": "value"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var body Body
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status.Code != 0 {
		t.Errorf("expected code 0, got %d", body.Status.Code)
	}
	if body.Status.Msg != "success" {
		t.Errorf("expected msg 'success', got %q", body.Status.Msg)
	}
	if body.Data == nil {
		t.Error("expected non-nil data")
	}
}

func TestOKEmpty(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		OKEmpty(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var body Body
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status.Code != 0 {
		t.Errorf("expected code 0, got %d", body.Status.Code)
	}
	if body.Data != nil {
		t.Errorf("expected nil data, got %v", body.Data)
	}
}

func TestFail(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		Fail(c, errcode.ParamError)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	// Fail uses HTTP 200 (Java SecretPad convention)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var body Body
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status.Code != errcode.ParamError.Code {
		t.Errorf("expected code %d, got %d", errcode.ParamError.Code, body.Status.Code)
	}
	// No Accept-Language -> Java default locale (zh_CN), placeholder dropped.
	if body.Status.Msg != "入参校验失败" {
		t.Errorf("expected localized zh msg, got %q", body.Status.Msg)
	}
}

func TestFail_AcceptLanguageEN(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		FailArgs(c, errcode.ParamError, "name")
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.ServeHTTP(w, req)
	var body Body
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status.Msg != "Input parameter verification failed: name" {
		t.Errorf("unexpected en msg %q", body.Status.Msg)
	}
}

func TestFailWithMsg(t *testing.T) {
	customMsg := "custom error message"
	r := setupRouter(func(c *gin.Context) {
		FailWithMsg(c, errcode.SystemError, customMsg)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	var body Body
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status.Code != errcode.SystemError.Code {
		t.Errorf("expected code %d, got %d", errcode.SystemError.Code, body.Status.Code)
	}
	// SystemError's Java template is "系统未知错误: {0}" -> custom msg fills {0}.
	if body.Status.Msg != "系统未知错误: "+customMsg {
		t.Errorf("expected msg with %q, got %q", customMsg, body.Status.Msg)
	}
	// Codes without placeholder keep the custom message verbatim.
	r2 := setupRouter(func(c *gin.Context) { FailWithMsg(c, errcode.AuthUserIsLocked, customMsg) })
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, req)
	json.Unmarshal(w2.Body.Bytes(), &body)
	if body.Status.Msg == "" {
		t.Error("expected non-empty msg")
	}
}

func TestFailHTTP(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		FailHTTP(c, http.StatusForbidden, errcode.Forbidden)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}

	var body Body
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status.Code != errcode.Forbidden.Code {
		t.Errorf("expected code %d, got %d", errcode.Forbidden.Code, body.Status.Code)
	}
}

func TestBody_JSONStructure(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		OK(c, map[string]interface{}{
			"name":  "test",
			"value": 123,
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	// Verify JSON structure matches Java SecretPad format
	var raw map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &raw)

	// Must have "status" object with "code" and "msg"
	status, ok := raw["status"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'status' object in response")
	}
	if _, ok := status["code"]; !ok {
		t.Error("expected 'code' in status")
	}
	if _, ok := status["msg"]; !ok {
		t.Error("expected 'msg' in status")
	}

	// Must have "data" object
	if _, ok := raw["data"]; !ok {
		t.Error("expected 'data' in response")
	}
}

func TestOK_NilData(t *testing.T) {
	r := setupRouter(func(c *gin.Context) {
		OK(c, nil)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	var body Body
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status.Code != 0 {
		t.Errorf("expected code 0, got %d", body.Status.Code)
	}
	// Data should be omitted when nil (omitempty tag)
	var raw map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &raw)
	if _, exists := raw["data"]; exists {
		t.Error("expected 'data' to be omitted when nil")
	}
}
