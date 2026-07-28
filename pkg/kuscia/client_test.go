package kuscia

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(handler http.HandlerFunc) (*httptest.Server, *Client) {
	server := httptest.NewServer(handler)
	client := NewClient(&ClientConfig{
		Host:     server.Listener.Addr().(*net.TCPAddr).IP.String(),
		Port:     server.Listener.Addr().(*net.TCPAddr).Port,
		Protocol: "notls",
		Timeout:  5 * time.Second,
	})
	return server, client
}

func TestClient_Ping_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/health/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 0, "msg": "success"},
		})
	})
	defer server.Close()

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestClient_CreateJob_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/job/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req CreateJobRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.JobID != "test-job-1" {
			t.Errorf("expected job_id 'test-job-1', got %q", req.JobID)
		}
		json.NewEncoder(w).Encode(CreateJobResponse{
			Status: Status{Code: 0, Message: "success"},
		})
	})
	defer server.Close()

	resp, err := client.CreateJob(context.Background(), &CreateJobRequest{
		JobID:     "test-job-1",
		Initiator: "alice",
		Tasks: []TaskConfig{
			{AppImage: "secretflow", Alias: "task-1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}
	if resp.Status.Code != 0 {
		t.Errorf("expected status code 0, got %d", resp.Status.Code)
	}
}

func TestClient_QueryJob_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 0, "msg": "success"},
			"data": map[string]interface{}{
				"job_id":    "job-123",
				"initiator": "alice",
				"state":     "Running",
				"tasks": []map[string]interface{}{
					{"task_id": "t1", "alias": "node-1", "state": "Running"},
				},
			},
		})
	})
	defer server.Close()

	resp, err := client.QueryJob(context.Background(), "job-123")
	if err != nil {
		t.Fatalf("QueryJob failed: %v", err)
	}
	if resp.Data.State != "Running" {
		t.Errorf("expected state 'Running', got %q", resp.Data.State)
	}
	if len(resp.Data.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(resp.Data.Tasks))
	}
}

func TestClient_StopJob_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/job/stop" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(StopJobResponse{
			Status: Status{Code: 0, Message: "success"},
		})
	})
	defer server.Close()

	if err := client.StopJob(context.Background(), "job-123"); err != nil {
		t.Fatalf("StopJob failed: %v", err)
	}
}

func TestClient_CreateDomain_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/domain/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(CreateDomainResponse{
			Status: Status{Code: 0, Message: "success"},
		})
	})
	defer server.Close()

	err := client.CreateDomain(context.Background(), &CreateDomainRequest{
		DomainID: "alice",
		Role:     "lite",
	})
	if err != nil {
		t.Fatalf("CreateDomain failed: %v", err)
	}
}

func TestClient_CreateDomainRoute_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/domainroute/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(CreateDomainRouteResponse{
			Status: Status{Code: 0, Message: "success"},
		})
	})
	defer server.Close()

	err := client.CreateDomainRoute(context.Background(), &CreateDomainRouteRequest{
		Source:      "alice",
		Destination: "bob",
	})
	if err != nil {
		t.Fatalf("CreateDomainRoute failed: %v", err)
	}
}

func TestClient_ErrorResponse(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(CreateJobResponse{
			Status: Status{Code: 1, Message: "job already exists"},
		})
	})
	defer server.Close()

	_, err := client.CreateJob(context.Background(), &CreateJobRequest{
		JobID:     "dup-job",
		Initiator: "alice",
	})
	if err == nil {
		t.Fatal("expected error for non-zero status code")
	}
}

func TestClient_GrantDomainData_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/domaindata/grant" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(GrantDomainDataResponse{
			Status: Status{Code: 0, Message: "success"},
		})
	})
	defer server.Close()

	err := client.GrantDomainData(context.Background(), &GrantDomainDataRequest{
		DomainID:     "alice",
		DomainDataID: "table-1",
		GrantDomain:  "bob",
	})
	if err != nil {
		t.Fatalf("GrantDomainData failed: %v", err)
	}
}
