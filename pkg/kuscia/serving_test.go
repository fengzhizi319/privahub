package kuscia

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestClient_CreateServing_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/serving/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req CreateServingRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ServingID != "serving-1" {
			t.Errorf("expected serving_id 'serving-1', got %q", req.ServingID)
		}
		if len(req.Parties) != 2 {
			t.Errorf("expected 2 parties, got %d", len(req.Parties))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 0, "message": "success"},
		})
	})
	defer server.Close()

	err := client.CreateServing(context.Background(), &CreateServingRequest{
		ServingID: "serving-1",
		Initiator: "alice",
		Parties: []ServingParty{
			{DomainID: "alice", Role: "server", AppImage: "sf-serving"},
			{DomainID: "bob", Role: "client", AppImage: "sf-serving"},
		},
	})
	if err != nil {
		t.Fatalf("CreateServing failed: %v", err)
	}
}

func TestClient_QueryServing_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/serving/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(QueryServingResponse{
			Status: Status{Code: 0, Message: "success"},
			Data: &QueryServingResponseData{
				Initiator: "alice",
				Parties: []ServingParty{
					{DomainID: "alice", Role: "server"},
				},
				Status: ServingStatusDetail{
					State:            "Running",
					TotalParties:     2,
					AvailableParties: 2,
				},
			},
		})
	})
	defer server.Close()

	resp, err := client.QueryServing(context.Background(), "serving-1")
	if err != nil {
		t.Fatalf("QueryServing failed: %v", err)
	}
	if resp.Data == nil {
		t.Fatal("expected non-nil data")
	}
	if resp.Data.Status.State != "Running" {
		t.Errorf("expected state 'Running', got %q", resp.Data.Status.State)
	}
	if resp.Data.Status.AvailableParties != 2 {
		t.Errorf("expected 2 available parties, got %d", resp.Data.Status.AvailableParties)
	}
}

func TestClient_UpdateServing_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/serving/update" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 0, "message": "success"},
		})
	})
	defer server.Close()

	err := client.UpdateServing(context.Background(), &UpdateServingRequest{
		ServingID: "serving-1",
		Parties: []ServingParty{
			{DomainID: "alice", Role: "server", Replicas: 2},
		},
	})
	if err != nil {
		t.Fatalf("UpdateServing failed: %v", err)
	}
}

func TestClient_DeleteServing_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/serving/delete" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req DeleteServingRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ServingID != "serving-1" {
			t.Errorf("expected serving_id 'serving-1', got %q", req.ServingID)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 0, "message": "success"},
		})
	})
	defer server.Close()

	err := client.DeleteServing(context.Background(), "serving-1")
	if err != nil {
		t.Fatalf("DeleteServing failed: %v", err)
	}
}

func TestClient_BatchQueryServingStatus_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/serving/status/batchQuery" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(BatchQueryServingStatusResponse{
			Status: Status{Code: 0, Message: "success"},
			Data: &BatchQueryServingStatusResponseData{
				Servings: []ServingStatusEntry{
					{ServingID: "s1", Status: ServingStatusDetail{State: "Running"}},
					{ServingID: "s2", Status: ServingStatusDetail{State: "Stopped"}},
				},
			},
		})
	})
	defer server.Close()

	entries, err := client.BatchQueryServingStatus(context.Background(), []string{"s1", "s2"})
	if err != nil {
		t.Fatalf("BatchQueryServingStatus failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ServingID != "s1" || entries[0].Status.State != "Running" {
		t.Errorf("unexpected entry[0]: %+v", entries[0])
	}
	if entries[1].ServingID != "s2" || entries[1].Status.State != "Stopped" {
		t.Errorf("unexpected entry[1]: %+v", entries[1])
	}
}

func TestClient_BatchQueryServingStatus_NilData(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(BatchQueryServingStatusResponse{
			Status: Status{Code: 0, Message: "success"},
			Data:   nil,
		})
	})
	defer server.Close()

	entries, err := client.BatchQueryServingStatus(context.Background(), []string{"s1"})
	if err != nil {
		t.Fatalf("BatchQueryServingStatus failed: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries for nil data, got %v", entries)
	}
}

func TestClient_ListDomainDataSource_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/domaindatasource/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(ListDomainDataSourceResponse{
			Status: Status{Code: 0, Message: "success"},
			Data: struct {
				DatasourceList []DomainDataSource `json:"datasource_list"`
			}{
				DatasourceList: []DomainDataSource{
					{DomainID: "alice", DatasourceID: "ds-1", Name: "local-fs", AccessDirectly: true},
					{DomainID: "alice", DatasourceID: "ds-2", Name: "oss", AccessDirectly: false},
				},
			},
		})
	})
	defer server.Close()

	list, err := client.ListDomainDataSource(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ListDomainDataSource failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 datasources, got %d", len(list))
	}
	if list[0].DatasourceID != "ds-1" || !list[0].AccessDirectly {
		t.Errorf("unexpected datasource[0]: %+v", list[0])
	}
}

func TestClient_ListDomainDataSource_Error(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ListDomainDataSourceResponse{
			Status: Status{Code: 1, Message: "domain not found"},
		})
	})
	defer server.Close()

	_, err := client.ListDomainDataSource(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected error for non-zero status")
	}
}

func TestClient_UpdateDomainDataSource_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/domaindatasource/update" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(UpdateDomainDataSourceResponse{
			Status: Status{Code: 0, Message: "success"},
		})
	})
	defer server.Close()

	accessDirectly := true
	err := client.UpdateDomainDataSource(context.Background(), &UpdateDomainDataSourceRequest{
		DomainID:       "alice",
		DatasourceID:   "ds-1",
		AccessDirectly: &accessDirectly,
	})
	if err != nil {
		t.Fatalf("UpdateDomainDataSource failed: %v", err)
	}
}

func TestClient_GenerateKeyCerts_Success(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/certificate/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req GenerateKeyCertsRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.CommonName != "test.example.com" {
			t.Errorf("expected common_name 'test.example.com', got %q", req.CommonName)
		}
		json.NewEncoder(w).Encode(GenerateKeyCertsResponse{
			Status:    Status{Code: 0, Message: "success"},
			Key:       "base64-encoded-key",
			CertChain: []string{"base64-cert-1", "base64-cert-2"},
		})
	})
	defer server.Close()

	resp, err := client.GenerateKeyCerts(context.Background(), &GenerateKeyCertsRequest{
		CommonName:   "test.example.com",
		Organization: "Test Org",
		DurationSec:  86400,
		KeyType:      "PKCS#8",
	})
	if err != nil {
		t.Fatalf("GenerateKeyCerts failed: %v", err)
	}
	if resp.Key != "base64-encoded-key" {
		t.Errorf("expected key 'base64-encoded-key', got %q", resp.Key)
	}
	if len(resp.CertChain) != 2 {
		t.Errorf("expected 2 certs in chain, got %d", len(resp.CertChain))
	}
}

// Bug69 regression: GenerateKeyCerts must return error on non-zero status.
func TestClient_GenerateKeyCerts_ErrorStatus(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(GenerateKeyCertsResponse{
			Status: Status{Code: 1, Message: "CA not configured"},
		})
	})
	defer server.Close()

	_, err := client.GenerateKeyCerts(context.Background(), &GenerateKeyCertsRequest{
		CommonName: "fail.example.com",
	})
	if err == nil {
		t.Fatal("expected error for non-zero status code")
	}
}

// Bug70 regression: CreateServing must return error on non-zero status.
func TestClient_CreateServing_ErrorStatus(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 2, "message": "serving already exists"},
		})
	})
	defer server.Close()

	err := client.CreateServing(context.Background(), &CreateServingRequest{
		ServingID: "dup-serving",
		Initiator: "alice",
	})
	if err == nil {
		t.Fatal("expected error for non-zero status code")
	}
}

// Bug70 regression: UpdateServing must return error on non-zero status.
func TestClient_UpdateServing_ErrorStatus(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 3, "message": "serving not found"},
		})
	})
	defer server.Close()

	err := client.UpdateServing(context.Background(), &UpdateServingRequest{
		ServingID: "missing-serving",
	})
	if err == nil {
		t.Fatal("expected error for non-zero status code")
	}
}

// Bug70 regression: DeleteServing must return error on non-zero status.
func TestClient_DeleteServing_ErrorStatus(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 3, "message": "serving not found"},
		})
	})
	defer server.Close()

	err := client.DeleteServing(context.Background(), "missing-serving")
	if err == nil {
		t.Fatal("expected error for non-zero status code")
	}
}

// Bug71 regression: QueryServing must return error on non-zero status.
func TestClient_QueryServing_ErrorStatus(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(QueryServingResponse{
			Status: Status{Code: 3, Message: "serving not found"},
		})
	})
	defer server.Close()

	_, err := client.QueryServing(context.Background(), "missing-serving")
	if err == nil {
		t.Fatal("expected error for non-zero status code")
	}
}
