package kuscia

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestClient_RestartJob(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/job/restart" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req RestartJobRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		code := int32(0)
		if req.JobID != "job-1" {
			code = 11100
		}
		_ = json.NewEncoder(w).Encode(RestartJobResponse{Status: Status{Code: code, Message: "m"}})
	})
	defer server.Close()

	if err := client.RestartJob(context.Background(), "job-1"); err != nil {
		t.Fatalf("RestartJob failed: %v", err)
	}
	if err := client.RestartJob(context.Background(), "other"); err == nil {
		t.Fatal("expected error for non-zero status")
	}
}
