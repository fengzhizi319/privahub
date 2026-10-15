package kuscia

import (
	"encoding/json"
	"testing"
)

func TestFlexInt64_Unmarshal(t *testing.T) {
	cases := map[string]FlexInt64{`86400`: 86400, `"86400"`: 86400, `""`: 0, `null`: 0, `-5`: -5}
	for in, want := range cases {
		var v FlexInt64
		if err := json.Unmarshal([]byte(in), &v); err != nil || v != want {
			t.Errorf("unmarshal %s = %d, %v; want %d", in, v, err, want)
		}
	}
	for _, bad := range []string{`"abc"`, `1.5`, `true`} {
		var v FlexInt64
		if err := json.Unmarshal([]byte(bad), &v); err == nil {
			t.Errorf("unmarshal %s: expected error", bad)
		}
	}
	b, _ := json.Marshal(struct {
		V FlexInt64 `json:"v"`
	}{7})
	if string(b) != `{"v":7}` {
		t.Errorf("marshal = %s", b)
	}
}

// Regression: KusciaAPI (protojson) returns token_config.rolling_update_period
// as a string; QueryDomainRoute failed to decode it and every route was
// reported as Failed (graph start: "训练图节点路由不可用").
func TestQueryDomainRouteResponse_StringInt64(t *testing.T) {
	body := `{"status":{"code":0,"message":"success"},"data":{"name":"alice-bob","source":"alice",` +
		`"destination":"bob","authentication_type":"Token","token_config":{"rolling_update_period":"86400",` +
		`"token_gen_method":"RSA-GEN"},"status":{"status":"Succeeded","reason":""}}}`
	var resp QueryDomainRouteResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Status.Status != RouteStatusSucceeded {
		t.Errorf("status = %q", resp.Data.Status.Status)
	}
	if resp.Data.TokenConfig == nil || resp.Data.TokenConfig.RollingUpdatePeriod != 86400 {
		t.Errorf("token_config = %+v", resp.Data.TokenConfig)
	}
}
