package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/pkg/errcode"
)

func TestNodeHandler_ResultList_BadBodyIsParamError(t *testing.T) {
	h := NewNodeHandler(nil) // the service must not be reached
	r := setupTestRouter()
	r.POST("/node/result/list", h.ResultList)
	for _, body := range []string{"", "{bad json", `{"pageSize":"ten"}`} {
		req := httptest.NewRequest(http.MethodPost, "/node/result/list", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp struct {
			Status struct {
				Code int `json:"code"`
			} `json:"status"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("body %q: %v %s", body, err, w.Body.String())
		}
		if resp.Status.Code != errcode.ParamError.Code {
			t.Fatalf("body %q: code %d, want %d", body, resp.Status.Code, errcode.ParamError.Code)
		}
	}
}
