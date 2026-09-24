package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// fakeS3 answers GetBucketLocation / HeadObject for bucket "b1" and object "b1/data/t.csv".
func fakeS3(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			w.WriteHeader(http.StatusOK) // endpoint detection
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=ak1/") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<Error><Code>InvalidAccessKeyId</Code></Error>`))
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/b1":
			_, _ = w.Write([]byte(`<LocationConstraint/>`))
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<Error><Code>NoSuchBucket</Code></Error>`))
		case r.Method == http.MethodHead && r.URL.Path == "/b1/data/t.csv":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dataBizCode(err error) int {
	if be, ok := AsBizError(err); ok && be.Code != nil {
		return be.Code.Code
	}
	return -1
}

func TestIPFilter(t *testing.T) {
	f := NewIPFilter(nil)
	assert.Error(t, f.CheckURL("http://127.0.0.1:9000"))
	assert.Error(t, f.CheckURL("http://192.168.1.10/x"))
	assert.Equal(t, errcode.FeatureTableIpFilter.Code, dataBizCode(f.CheckURL("10.1.2.3:9000")))
	assert.NoError(t, f.CheckURL("http://8.8.8.8"))
	off := false
	assert.NoError(t, NewIPFilter(&config.IPBlockConfig{Enable: &off}).CheckURL("http://127.0.0.1"))
	custom := NewIPFilter(&config.IPBlockConfig{List: []string{"8.8.8.0/24"}})
	assert.Error(t, custom.CheckURL("http://8.8.8.8"))
	assert.NoError(t, custom.CheckURL("http://127.0.0.1"))
}

func TestOssService_SignedChecks(t *testing.T) {
	s3 := fakeS3(t)
	oss := NewOssServiceWithFilter(NewDisabledIPFilter())
	ctx := context.Background()
	cfg := &OssConfig{Endpoint: s3.URL, AccessKeyID: "ak1", AccessKeySecret: "sk"}
	require.NoError(t, oss.CheckBucketExists(ctx, cfg, "b1"))
	assert.Equal(t, errcode.DatasourceDataSourceBucketNotExist.Code, dataBizCode(oss.CheckBucketExists(ctx, cfg, "nope")))
	bad := &OssConfig{Endpoint: s3.URL, AccessKeyID: "wrong", AccessKeySecret: "sk"}
	assert.Equal(t, errcode.DatasourceDataSourceCredentialsInvalid.Code, dataBizCode(oss.CheckBucketExists(ctx, bad, "b1")))
	ok, err := oss.CheckObjectExists(ctx, cfg, "b1", "data/t.csv")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, _ = oss.CheckObjectExists(ctx, cfg, "b1", "missing.csv")
	assert.False(t, ok)
	// default filter blocks loopback endpoints
	assert.Error(t, NewOssService().CheckBucketExists(ctx, cfg, "b1"))
	assert.Error(t, NewOssService().CheckBucketExists(ctx, &OssConfig{}, "b1"))
}

func TestOdpsChecker_Signature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "ODPS id1:") || r.Header.Get("Date") == "" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<Error><Code>InvalidAccessKeyId</Code><Message>bad</Message></Error>`))
			return
		}
		switch r.URL.Path {
		case "/api/projects/p1", "/api/projects/p1/tables/t1":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewOdpsChecker()
	ctx := context.Background()
	require.NoError(t, c.TestConnection(ctx, &OdpsConfig{Endpoint: srv.URL + "/api", AccessID: "id1", AccessKey: "k", Project: "p1"}))
	err := c.TestConnection(ctx, &OdpsConfig{Endpoint: srv.URL + "/api", AccessID: "x", AccessKey: "k", Project: "p1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidAccessKeyId")
	assert.True(t, c.TableExists(ctx, &OdpsConfig{Endpoint: srv.URL + "/api", AccessID: "id1", AccessKey: "k"}, "p1", "t1"))
	assert.False(t, c.TableExists(ctx, &OdpsConfig{Endpoint: srv.URL + "/api", AccessID: "id1", AccessKey: "k"}, "p1", "t2"))
	// canonical string layout
	h := http.Header{}
	h.Set("Date", "D")
	h.Set("x-odps-a", "1")
	assert.Equal(t, "GET\n\n\nD\nx-odps-a:1\n/projects/p", odpsCanonical("GET", "/projects/p", nil, h))
}

func TestMysqlChecker_OpenError(t *testing.T) {
	m := NewMysqlChecker()
	m.open = func(string) (*sql.DB, error) { return nil, errors.New("boom") }
	err := m.TestConnection(context.Background(), &MysqlConfig{Endpoint: "h:3306", User: "u", Password: "p", Database: "d"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MySQL test connection failed")
	assert.False(t, m.TableExists(context.Background(), &MysqlConfig{}, "d", "t"))
	// Connections use the per-checker guarded dialer (SSRF check at connect time).
	dsn := NewMysqlChecker().dsn(&MysqlConfig{Endpoint: "h:3306", User: "u", Password: "p", Database: "d"})
	assert.Contains(t, dsn, "u:p@privahub-guarded-")
	assert.Contains(t, dsn, "(h:3306)/d")
}

func newTestDatasourceService(t *testing.T) (*DatasourceService, *DatatableService, *dataFakeKuscia) {
	fk := newDataFakeKuscia(t)
	db := newDataTestDB(t)
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob"})
	dt := NewDatatableService(nil, nil, db, fk.client()).WithIPFilter(NewDisabledIPFilter())
	ds := NewDatasourceService(db, fk.client()).WithIPFilter(NewDisabledIPFilter()).WithDatatableFinder(dt)
	return ds, dt, fk
}

func TestDatasourceService_CreateOSS_PartialFailure(t *testing.T) {
	ds, _, fk := newTestDatasourceService(t)
	s3 := fakeS3(t)
	fk.failDomains["bob"] = true
	info, _ := json.Marshal(OssDatasourceInfo{Endpoint: s3.URL, Bucket: "b1", AK: "ak1", SK: "sk", Prefix: "data/"})
	vo, err := ds.CreateDatasource(context.Background(), &CreateDatasourceRequest{
		OwnerID: "alice", NodeIDs: []string{"alice", "bob", "alice"}, Type: "oss", Name: "my-oss", DataSourceInfo: info,
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(vo.DatasourceID, "oss-"))
	assert.Len(t, vo.DatasourceID, 4+16)
	assert.Contains(t, vo.FailedCreatedNodes, "bob")
	got := fk.datasources["alice"][vo.DatasourceID]
	assert.Equal(t, "oss", got.Type)
	require.NotNil(t, got.Info.Oss)
	assert.Equal(t, "b1", got.Info.Oss.Bucket)
	assert.Equal(t, "ak1", got.Info.Oss.AccessKeyID)
	assert.Equal(t, "data/", got.Info.Oss.Prefix)
	// all failed -> error
	fk.failDomains["alice"] = true
	_, err = ds.CreateDatasource(context.Background(), &CreateDatasourceRequest{
		OwnerID: "alice", NodeIDs: []string{"alice"}, Type: "OSS", Name: "x", DataSourceInfo: info,
	})
	assert.Equal(t, errcode.DatasourceDataSourceCreateFail.Code, dataBizCode(err))
}

func TestDatasourceService_CreateValidationAndChecks(t *testing.T) {
	ds, _, fk := newTestDatasourceService(t)
	ctx := context.Background()
	_, err := ds.CreateDatasource(ctx, &CreateDatasourceRequest{OwnerID: "alice", Type: "OSS", Name: "n", NodeIDs: []string{"alice"}})
	assert.Error(t, err)
	_, err = ds.CreateDatasource(ctx, &CreateDatasourceRequest{OwnerID: "alice", Type: "FOO", Name: "n", NodeIDs: []string{"alice"}, DataSourceInfo: json.RawMessage(`{}`)})
	assert.Error(t, err)
	// MySQL connectivity failure is surfaced and nothing is created in Kuscia.
	m := NewMysqlChecker()
	m.open = func(string) (*sql.DB, error) { return nil, errors.New("refused") }
	ds.WithMysqlChecker(m)
	info, _ := json.Marshal(MysqlDatasourceInfo{Endpoint: "db:3306", User: "u", Password: "p", Database: "d"})
	_, err = ds.CreateDatasource(ctx, &CreateDatasourceRequest{OwnerID: "alice", NodeIDs: []string{"alice"}, Type: "MYSQL", Name: "m", DataSourceInfo: info})
	assert.Equal(t, errcode.DatasourceDataSourceCreateFail.Code, dataBizCode(err))
	assert.Equal(t, 0, fk.called("/api/v1/domaindatasource/create"))
	// Test endpoint reports the failure without error.
	vo, err := ds.TestDatasource(ctx, &TestDatasourceRequest{Type: "MYSQL", DataSourceInfo: info})
	require.NoError(t, err)
	assert.False(t, vo.Success)
}

func seedDatasources(fk *dataFakeKuscia) {
	fk.datasources["alice"] = map[string]kuscia.DomainDataSource{
		"oss-a":               {DomainID: "alice", DatasourceID: "oss-a", Name: "ossA", Type: "oss", Info: &kuscia.DataSourceInfo{Oss: &kuscia.OssDataSourceInfo{Endpoint: "http://e", Bucket: "b", AccessKeyID: "AKIDXYZ", AccessKeySecret: "SECRET"}}},
		"mysql-b":             {DomainID: "alice", DatasourceID: "mysql-b", Name: "mysqlB", Type: "mysql", Status: "Unavailable"},
		"default-data-source": {DomainID: "alice", DatasourceID: "default-data-source", Name: "default-data-source", Type: "localfs"},
	}
	fk.datasources["bob"] = map[string]kuscia.DomainDataSource{
		"oss-a": {DomainID: "bob", DatasourceID: "oss-a", Name: "ossA", Type: "oss"},
	}
	fk.domaindata["alice"] = map[string]kuscia.DomainData{
		"t1": {DomainDataID: "t1", Name: "table1", Type: "table", DatasourceID: "mysql-b", Author: "alice"},
	}
}

func TestDatasourceService_ListAggregateFilterPage(t *testing.T) {
	ds, _, fk := newTestDatasourceService(t)
	seedDatasources(fk)
	ds.WithEnv(NewEnvService("autonomy", "alice", "inst1", nil))
	// AUTONOMY lists the nodes of the owner institution (Java listDatasource).
	ds.db.Model(&model.NodeDO{}).Where("node_id IN ?", []string{"alice", "bob"}).Update("inst_id", "inst1")
	ctx := context.Background()
	vo, err := ds.ListDatasources(ctx, &DatasourceListRequest{OwnerID: "inst1", Page: 1, Size: 100})
	require.NoError(t, err)
	byID := map[string]DatasourceListInfoAggregate{}
	for _, a := range vo.Infos {
		byID[a.DatasourceID] = a
	}
	assert.Len(t, byID["oss-a"].Nodes, 2)
	assert.Equal(t, "OSS", byID["oss-a"].Type)
	assert.Equal(t, []string{"table1"}, byID["mysql-b"].RelatedDatas)
	assert.Contains(t, byID, DefaultHTTPDatasourceID)

	vo, err = ds.ListDatasources(ctx, &DatasourceListRequest{OwnerID: "inst1", Types: []string{"MYSQL"}, Status: "unavailable"})
	require.NoError(t, err)
	require.Len(t, vo.Infos, 1)
	assert.Equal(t, "mysql-b", vo.Infos[0].DatasourceID)

	vo, err = ds.ListDatasources(ctx, &DatasourceListRequest{OwnerID: "inst1", Name: "oss", Page: 1, Size: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 1, vo.Total)
	assert.EqualValues(t, 1, vo.TotalPage)
	_, err = ds.ListDatasources(ctx, &DatasourceListRequest{OwnerID: "inst1", Page: 5, Size: 10})
	assert.Equal(t, errcode.SystemOutOfRangeError.Code, dataBizCode(err))
}

func TestDatasourceService_DetailNodesDelete(t *testing.T) {
	ds, _, fk := newTestDatasourceService(t)
	seedDatasources(fk)
	ctx := context.Background()
	d, err := ds.GetDatasourceDetail(ctx, &DatasourceDetailRequest{OwnerID: "alice", DatasourceID: "oss-a", Type: "OSS"})
	require.NoError(t, err)
	info := d.Info.(*OssDatasourceInfo)
	assert.Equal(t, "AKI****", info.AK)
	assert.Equal(t, "******", info.SK, "secret keys are fully masked")
	assert.NotContains(t, info.SK, "SEC")
	assert.Equal(t, "OSS", d.Type)

	_, err = ds.GetDatasourceDetail(ctx, &DatasourceDetailRequest{OwnerID: "alice", DatasourceID: "none", Type: "OSS"})
	assert.Equal(t, errcode.DatasourceDataSourceNotFound.Code, dataBizCode(err))

	nodes, err := ds.GetDatasourceNodes(ctx, &DatasourceNodesRequest{OwnerID: "alice", DatasourceID: "oss-a"})
	require.NoError(t, err)
	assert.Equal(t, "Alice", nodes.Nodes[0].NodeName)

	// related datatable -> refuse
	err = ds.DeleteDatasource(ctx, &DeleteDatasourceRequest{OwnerID: "alice", DatasourceID: "mysql-b", Type: "MYSQL"})
	assert.Equal(t, errcode.DatasourceDataSourceDeleteFail.Code, dataBizCode(err))
	assert.Contains(t, fk.datasources["alice"], "mysql-b")

	require.NoError(t, ds.DeleteDatasource(ctx, &DeleteDatasourceRequest{OwnerID: "alice", DatasourceID: "oss-a", Type: "OSS"}))
	assert.NotContains(t, fk.datasources["alice"], "oss-a")
	err = ds.DeleteDatasource(ctx, &DeleteDatasourceRequest{OwnerID: "alice", DatasourceID: "oss-a", Type: "OSS"})
	assert.Equal(t, errcode.DatasourceDataSourceDeleteFail.Code, dataBizCode(err))
}

// AUTONOMY: nodes of another institution are rejected with INST_NOT_MATCH_NODE
// (Java checkNodesInInst), for datasource/create and datatable/create; the
// institution only lists its own nodes.
func TestAutonomy_NodesMustBelongToInst(t *testing.T) {
	ds, dt, fk := newTestDatasourceService(t)
	seedDatasources(fk)
	env := NewEnvService("autonomy", "alice", "inst1", nil)
	ds.WithEnv(env)
	dt.WithEnv(env)
	ds.db.Model(&model.NodeDO{}).Where("node_id = ?", "alice").Update("inst_id", "inst1")
	ds.db.Model(&model.NodeDO{}).Where("node_id = ?", "bob").Update("inst_id", "inst2")
	ctx := context.Background()

	info, _ := json.Marshal(OssDatasourceInfo{Endpoint: "http://e", Bucket: "b", AK: "ak", SK: "sk"})
	_, err := ds.CreateDatasource(ctx, &CreateDatasourceRequest{OwnerID: "inst1", NodeIDs: []string{"alice", "bob"},
		Type: "OSS", Name: "x", DataSourceInfo: info})
	assert.Equal(t, errcode.InstNotMatchNode.Code, dataBizCode(err))
	assert.Equal(t, 0, fk.called("/api/v1/domaindatasource/create"))

	_, err = dt.CreateDatatableCompat(ctx, &CreateDatatableCompatRequest{OwnerID: "inst1", NodeIDs: []string{"bob"},
		DatatableName: "t", DatasourceID: "oss-a", DatasourceName: "ossA", DatasourceType: "OSS", RelativeURI: "a.csv",
		Columns: []TableColumnCompat{{ColName: "id", ColType: "str"}}})
	assert.Equal(t, errcode.InstNotMatchNode.Code, dataBizCode(err))

	vo, err := ds.ListDatasources(ctx, &DatasourceListRequest{OwnerID: "inst1", Page: 1, Size: 100})
	require.NoError(t, err)
	for _, a := range vo.Infos {
		for _, n := range a.Nodes {
			assert.Equal(t, "alice", n.NodeID, "datasource/list must stay within the institution")
		}
	}
	list, err := dt.ListDatatablesByOwnerID(ctx, &ListDatatableRequest{OwnerID: "inst1"})
	require.NoError(t, err)
	for _, it := range list.DatatableNodeVOList {
		assert.Equal(t, "alice", it.NodeID, "datatable/list must stay within the institution")
	}
}

// datatable/list degrades when a node's KusciaAPI fails: the call succeeds
// with the data still available instead of failing as a whole.
func TestDatatableList_KusciaFailureDegrades(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	seedTables(t, dt, fk)
	fk.failList["alice"] = true
	vo, err := dt.ListDatatablesByOwnerID(context.Background(), &ListDatatableRequest{OwnerID: "alice"})
	require.NoError(t, err)
	require.NotNil(t, vo)
	// Kuscia tables are missing; locally known HTTP feature tables remain.
	for _, it := range vo.DatatableNodeVOList {
		assert.Equal(t, DatasourceTypeHTTP, it.DatatableVO.DatasourceType)
	}
}
