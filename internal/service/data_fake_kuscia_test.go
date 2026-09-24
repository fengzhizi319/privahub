package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// dataFakeKuscia is an in-memory KusciaAPI HTTP server for data-management tests.
type dataFakeKuscia struct {
	mu          sync.Mutex
	srv         *httptest.Server
	datasources map[string]map[string]kuscia.DomainDataSource // domain -> id -> ds
	domaindata  map[string]map[string]kuscia.DomainData       // domain -> id -> dd
	grants      map[string]kuscia.GrantDomainDataRequest      // grantID -> req
	jobs        map[string]kuscia.CreateJobRequest
	jobState    map[string]string
	failDomains map[string]bool // create* on these domains fails
	failList    map[string]bool // domaindata/list on these domains fails
	calls       []string        // "path domain"
	lastBodies  map[string][]byte
}

func newDataFakeKuscia(t *testing.T) *dataFakeKuscia {
	f := &dataFakeKuscia{
		datasources: map[string]map[string]kuscia.DomainDataSource{},
		domaindata:  map[string]map[string]kuscia.DomainData{},
		grants:      map[string]kuscia.GrantDomainDataRequest{},
		jobs:        map[string]kuscia.CreateJobRequest{},
		jobState:    map[string]string{},
		failDomains: map[string]bool{},
		failList:    map[string]bool{},
		lastBodies:  map[string][]byte{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *dataFakeKuscia) client() *kuscia.Client {
	u, _ := url.Parse(f.srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return kuscia.NewClient(&kuscia.ClientConfig{Host: u.Hostname(), Port: port, Protocol: "notls"})
}

func (f *dataFakeKuscia) ok(w http.ResponseWriter, data interface{}) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": 0, "message": "success"}, "data": data})
}

func (f *dataFakeKuscia) fail(w http.ResponseWriter, code int, msg string) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": code, "message": msg}})
}

func (f *dataFakeKuscia) called(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, path) {
			n++
		}
	}
	return n
}

func (f *dataFakeKuscia) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastBodies[r.URL.Path] = body
	var m map[string]json.RawMessage
	_ = json.Unmarshal(body, &m)
	str := func(k string) string {
		var s string
		_ = json.Unmarshal(m[k], &s)
		return s
	}
	f.calls = append(f.calls, r.URL.Path+" "+str("domain_id"))
	switch r.URL.Path {
	case "/api/v1/domaindatasource/create":
		var req kuscia.CreateDomainDataSourceRequest
		_ = json.Unmarshal(body, &req)
		if f.failDomains[req.DomainID] {
			f.fail(w, 11100, "create failed on "+req.DomainID)
			return
		}
		if f.datasources[req.DomainID] == nil {
			f.datasources[req.DomainID] = map[string]kuscia.DomainDataSource{}
		}
		f.datasources[req.DomainID][req.DatasourceID] = kuscia.DomainDataSource{
			DomainID: req.DomainID, DatasourceID: req.DatasourceID, Name: req.Name, Type: req.Type, Info: req.Info, Status: "Available",
		}
		f.ok(w, map[string]string{"datasource_id": req.DatasourceID})
	case "/api/v1/domaindatasource/list":
		list := []kuscia.DomainDataSource{}
		for _, d := range f.datasources[str("domain_id")] {
			list = append(list, d)
		}
		f.ok(w, map[string]interface{}{"datasource_list": list})
	case "/api/v1/domaindatasource/query":
		d, ok := f.datasources[str("domain_id")][str("datasource_id")]
		if !ok {
			f.fail(w, 11700, "datasource not exists")
			return
		}
		f.ok(w, d)
	case "/api/v1/domaindatasource/delete":
		delete(f.datasources[str("domain_id")], str("datasource_id"))
		f.ok(w, nil)
	case "/api/v1/domaindata/create":
		var req kuscia.CreateDomainDataRequest
		_ = json.Unmarshal(body, &req)
		if f.failDomains[req.DomainID] {
			f.fail(w, 11500, "create failed on "+req.DomainID)
			return
		}
		if f.domaindata[req.DomainID] == nil {
			f.domaindata[req.DomainID] = map[string]kuscia.DomainData{}
		}
		f.domaindata[req.DomainID][req.DomainDataID] = kuscia.DomainData{
			DomainDataID: req.DomainDataID, Name: req.Name, Type: req.Type, RelativeURI: req.RelativeURI,
			DomainID: req.DomainID, DatasourceID: req.DatasourceID, Attributes: req.Attributes, Columns: req.Columns,
			Partition: req.Partition, Author: req.DomainID, Vendor: "manual", Status: "Available", FileFormat: req.FileFormat,
		}
		f.ok(w, map[string]string{"domaindata_id": req.DomainDataID})
	case "/api/v1/domaindata/list":
		var req kuscia.ListDomainDataRequest
		_ = json.Unmarshal(body, &req)
		if f.failList[req.Data.DomainID] {
			f.fail(w, 11500, "kuscia unavailable")
			return
		}
		list := []kuscia.DomainData{}
		for _, d := range f.domaindata[req.Data.DomainID] {
			if req.Data.DomainDataType != "" && d.Type != req.Data.DomainDataType {
				continue
			}
			list = append(list, d)
		}
		f.ok(w, map[string]interface{}{"domaindata_list": list})
	case "/api/v1/domaindata/query":
		var req kuscia.QueryDomainDataRequest
		_ = json.Unmarshal(body, &req)
		d, ok := f.domaindata[req.Data.DomainID][req.Data.DomainDataID]
		if !ok {
			f.fail(w, int(kuscia.ErrCodeDomainDataNotExists), "domaindata not exists")
			return
		}
		f.ok(w, d)
	case "/api/v1/domaindata/delete":
		delete(f.domaindata[str("domain_id")], str("domaindata_id"))
		f.ok(w, nil)
	case "/api/v1/domaindatagrant/create":
		var req kuscia.GrantDomainDataRequest
		_ = json.Unmarshal(body, &req)
		id := req.DomainDataGrantID
		if id == "" {
			id = "grant-" + strconv.Itoa(len(f.grants)+1)
		}
		f.grants[id] = req
		f.ok(w, map[string]string{"domaindatagrant_id": id})
	case "/api/v1/domaindatagrant/query":
		g, ok := f.grants[str("domaindatagrant_id")]
		if !ok {
			f.fail(w, 11800, "grant not exists")
			return
		}
		f.ok(w, map[string]interface{}{"data": map[string]string{
			"domaindatagrant_id": str("domaindatagrant_id"), "domaindata_id": g.DomainDataID, "grant_domain": g.GrantDomain, "domain_id": g.DomainID,
		}})
	case "/api/v1/job/create":
		var req kuscia.CreateJobRequest
		_ = json.Unmarshal(body, &req)
		f.jobs[req.JobID] = req
		f.ok(w, map[string]string{"job_id": req.JobID})
	case "/api/v1/job/query":
		st := f.jobState[str("job_id")]
		if st == "" {
			st = "Running"
		}
		f.ok(w, map[string]interface{}{"job_id": str("job_id"), "status": map[string]string{"state": st}})
	default:
		f.fail(w, 404, "unknown path "+r.URL.Path)
	}
}

// newDataTestDB opens an isolated in-memory sqlite DB with the data tables.
func newDataTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.NodeDO{}, &model.NodeRouteDO{}, &model.ProjectDO{}, &model.ProjectDatatableDO{},
		&model.FeatureTableDO{}, &model.ProjectFeatureTableDO{}, &model.TeeNodeDatatableManagementDO{},
		&model.DatasourceDO{}, &model.DatasourceNodeDO{}, &model.ProjectResultDO{}, &model.ProjectJobTaskDO{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
