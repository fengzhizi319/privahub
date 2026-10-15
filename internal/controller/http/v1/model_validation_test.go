package v1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
)

// Java ModelPartyPathRequest / CreateModelServingRequest bean validation runs
// before the service (and before the data-resource check).
func TestModelHandler_JavaValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelHandler(service.NewModelService(nil, nil), nil)
	r := gin.New()
	r.POST("/model/modelPartyPath", h.ModelPartyPath)
	r.POST("/model/serving/create", h.CreateServing)

	party := map[string]interface{}{"nodeId": "alice", "featureTableId": "mock", "isMock": true,
		"features":  []map[string]string{{"offlineName": "a", "onlineName": "a"}},
		"resources": []map[string]string{{"minCPU": "1", "maxCPU": "2", "minMemory": "1Gi", "maxMemory": "2Gi"}}}
	without := func(k string) map[string]interface{} {
		out := map[string]interface{}{}
		for kk, v := range party {
			if kk != k {
				out[kk] = v
			}
		}
		return out
	}
	cases := []struct {
		name, path string
		body       interface{}
		msg        string
	}{
		{"path missing graphNodeOutPutId", "/model/modelPartyPath", map[string]string{"projectId": "p", "graphNodeId": "n"}, "graphNodeOutPutId"},
		{"path blank graphNodeId", "/model/modelPartyPath", map[string]string{"projectId": "p", "graphNodeId": " ", "graphNodeOutPutId": "o"}, "graphNodeId"},
		{"serving missing featureTableId", "/model/serving/create", map[string]interface{}{"modelId": "m", "projectId": "p",
			"partyConfigs": []interface{}{without("featureTableId")}}, "partyConfigs[0].featureTableId"},
		{"serving missing isMock", "/model/serving/create", map[string]interface{}{"modelId": "m", "projectId": "p",
			"partyConfigs": []interface{}{party, without("isMock")}}, "partyConfigs[1].isMock"},
		{"serving missing nodeId", "/model/serving/create", map[string]interface{}{"modelId": "m", "projectId": "p",
			"partyConfigs": []interface{}{without("nodeId")}}, "nodeId"},
		{"serving blank resource", "/model/serving/create", map[string]interface{}{"modelId": "m", "projectId": "p",
			"partyConfigs": []interface{}{func() map[string]interface{} {
				p := without("resources")
				p["resources"] = []map[string]string{{"minCPU": "1", "maxCPU": "", "minMemory": "1Gi", "maxMemory": "2Gi"}}
				return p
			}()}}, "resources[0].maxCPU"},
	}
	for _, tc := range cases {
		w := postJSON(r, tc.path, tc.body)
		var resp voteRespBody
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Status.Code != errcode.SystemValidationError.Code || !strings.Contains(resp.Status.Msg, tc.msg) {
			t.Errorf("%s: got %+v, want VALIDATION_ERROR mentioning %q", tc.name, resp.Status, tc.msg)
		}
	}
}

func TestValidateServingPartyConfigs_Valid(t *testing.T) {
	mock := false
	if msg := service.ValidateServingPartyConfigs([]service.ServingPartyConfig{{NodeID: "a", FeatureTableID: "ft", IsMock: &mock}}); msg != "" {
		t.Errorf("valid config rejected: %s", msg)
	}
	if msg := service.ValidateServingPartyConfigs(nil); msg == "" {
		t.Error("empty deployment must be rejected instead of reporting a false success")
	}
}
