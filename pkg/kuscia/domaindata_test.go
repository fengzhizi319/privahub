package kuscia

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// recordServer returns a server that records the last request body per path
// and answers with the given canned data per path.
func recordServer(t *testing.T, answers map[string]string) (map[string]map[string]interface{}, *Client) {
	bodies := map[string]map[string]interface{}{}
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		bodies[r.URL.Path] = m
		data, ok := answers[r.URL.Path]
		if !ok {
			_, _ = w.Write([]byte(`{"status":{"code":11404,"message":"not found"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":{"code":0,"message":"success"},"data":` + data + `}`))
	})
	t.Cleanup(server.Close)
	return bodies, client
}

func TestDomainDataSourceAPIs(t *testing.T) {
	bodies, c := recordServer(t, map[string]string{
		"/api/v1/domaindatasource/create":     `{"datasource_id":"oss-1"}`,
		"/api/v1/domaindatasource/query":      `{"domain_id":"alice","datasource_id":"oss-1","type":"oss","status":"Available","info":{"oss":{"endpoint":"http://e","bucket":"b","access_key_id":"ak"}}}`,
		"/api/v1/domaindatasource/batchQuery": `{"datasource_list":[{"domain_id":"alice","datasource_id":"oss-1"}]}`,
		"/api/v1/domaindatasource/delete":     `{}`,
	})
	ctx := context.Background()
	f := false
	id, err := c.CreateDomainDataSource(ctx, &CreateDomainDataSourceRequest{
		DomainID: "alice", DatasourceID: "oss-1", Type: "oss", Name: "n", AccessDirectly: &f,
		Info: &DataSourceInfo{Oss: &OssDataSourceInfo{Endpoint: "http://e", Bucket: "b", AccessKeyID: "ak", AccessKeySecret: "sk", Prefix: "p/"}},
	})
	if err != nil || id != "oss-1" {
		t.Fatalf("create: %v %s", err, id)
	}
	oss := bodies["/api/v1/domaindatasource/create"]["info"].(map[string]interface{})["oss"].(map[string]interface{})
	if oss["access_key_secret"] != "sk" || oss["prefix"] != "p/" {
		t.Fatalf("unexpected oss info %v", oss)
	}
	if bodies["/api/v1/domaindatasource/create"]["access_directly"] != false {
		t.Fatalf("access_directly must be sent")
	}
	ds, err := c.QueryDomainDataSource(ctx, "alice", "oss-1")
	if err != nil || ds.Info.Oss.Bucket != "b" || ds.Status != "Available" {
		t.Fatalf("query: %v %+v", err, ds)
	}
	if bodies["/api/v1/domaindatasource/query"]["domain_id"] != "alice" {
		t.Fatalf("query must be flat")
	}
	list, err := c.BatchQueryDomainDataSource(ctx, []QueryDomainDataSourceRequestData{{DomainID: "alice", DatasourceID: "oss-1"}})
	if err != nil || len(list) != 1 {
		t.Fatalf("batch: %v", err)
	}
	if err := c.DeleteDomainDataSource(ctx, "alice", "oss-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestDomainDataAPIs(t *testing.T) {
	bodies, c := recordServer(t, map[string]string{
		"/api/v1/domaindata/create":     `{"domaindata_id":"d1"}`,
		"/api/v1/domaindata/batchQuery": `{"domaindata_list":[{"domaindata_id":"d1","file_format":"CSV","partition":{"type":"odps","fields":[{"name":"dt","type":"str"}]}},{"domaindata_id":"d2","file_format":1}]}`,
		"/api/v1/domaindata/list":       `{"domaindata_list":[]}`,
	})
	ctx := context.Background()
	id, err := c.CreateDomainDataWithID(ctx, &CreateDomainDataRequest{
		DomainID: "alice", DomainDataID: "d1", Name: "t", Type: "table", RelativeURI: "t.csv", FileFormat: FileFormatCSV,
		Attributes: map[string]string{"DatasourceType": "LOCAL"}, Columns: []DataColumn{{Name: "a", Type: "int", Comment: "c"}},
		Partition: &Partition{Type: "odps", Fields: []DataColumn{{Name: "dt", Type: "str"}}}, Author: "ignored",
	})
	if err != nil || id != "d1" {
		t.Fatalf("create: %v", err)
	}
	b := bodies["/api/v1/domaindata/create"]
	if b["file_format"] != "CSV" || b["author"] != nil {
		t.Fatalf("unexpected create body %v", b)
	}
	if b["columns"].([]interface{})[0].(map[string]interface{})["comment"] != "c" {
		t.Fatalf("column comment missing")
	}
	list, err := c.BatchQueryDomainData(ctx, []QueryDomainDataRequestData{{DomainID: "alice", DomainDataID: "d1"}})
	if err != nil || len(list) != 2 || list[0].Partition.Fields[0].Name != "dt" || list[1].FileFormat != FileFormatCSV {
		t.Fatalf("batch: %v %+v", err, list)
	}
	if _, err := c.ListDomainDataWithFilter(ctx, &ListDomainDataRequestData{DomainID: "alice", DomainDataType: "table", DomainDataVendor: "manual"}); err != nil {
		t.Fatal(err)
	}
	lb := bodies["/api/v1/domaindata/list"]["data"].(map[string]interface{})
	if lb["domaindata_vendor"] != "manual" || lb["domaindata_type"] != "table" {
		t.Fatalf("list filter not sent: %v", lb)
	}
	if _, err := c.QueryDomainData(ctx, "alice", "zz"); !IsNotFound(err) {
		t.Fatalf("expected not found APIError, got %v", err)
	}
}

func TestDomainDataGrantAPIs(t *testing.T) {
	bodies, c := recordServer(t, map[string]string{
		"/api/v1/domaindatagrant/create":     `{"domaindatagrant_id":"g1"}`,
		"/api/v1/domaindatagrant/query":      `{"data":{"domaindatagrant_id":"g1","domaindata_id":"d1","grant_domain":"tee","limit":{"expiration_time":"1700000000"}},"status":{"phase":"Ready"}}`,
		"/api/v1/domaindatagrant/batchQuery": `[{"data":{"domaindatagrant_id":"g1"}}]`,
	})
	ctx := context.Background()
	id, err := c.CreateDomainDataGrant(ctx, &CreateDomainDataGrantRequest{DomainDataGrantID: "g1", DomainID: "alice", DomainDataID: "d1", GrantDomain: "tee",
		Limit: &GrantLimit{ExpirationTime: 5, UseCount: 1}})
	if err != nil || id != "g1" {
		t.Fatalf("create: %v", err)
	}
	if bodies["/api/v1/domaindatagrant/create"]["domaindatagrant_id"] != "g1" {
		t.Fatalf("grant id not sent")
	}
	if bodies["/api/v1/domaindatagrant/create"]["limit"].(map[string]interface{})["expiration_time"] != "5" {
		t.Fatalf("int64 must be string-encoded")
	}
	g, err := c.QueryDomainDataGrant(ctx, "alice", "g1")
	if err != nil || g.Data.GrantDomain != "tee" || g.Data.Limit.ExpirationTime != 1700000000 || g.Status.Phase != "Ready" {
		t.Fatalf("query: %v %+v", err, g)
	}
	gs, err := c.BatchQueryDomainDataGrant(ctx, []QueryDomainDataGrantRequestData{{DomainID: "alice", DomainDataGrantID: "g1"}})
	if err != nil || len(gs) != 1 {
		t.Fatalf("batch: %v", err)
	}
}
