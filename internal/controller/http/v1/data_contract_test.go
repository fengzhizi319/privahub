package v1

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// dataContractKuscia serves the few KusciaAPI routes the data handlers need.
func dataContractKuscia(t *testing.T) *kuscia.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := func(data interface{}) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": 0}, "data": data})
		}
		switch r.URL.Path {
		case "/api/v1/domaindata/list":
			ok(map[string]interface{}{"domaindata_list": []map[string]interface{}{{
				"domaindata_id": "t1", "name": "users", "type": "table", "relative_uri": "u.csv",
				"datasource_id": "default-data-source", "author": "alice", "status": "Available",
				"file_format": "CSV", "columns": []map[string]string{{"name": "id", "type": "str", "comment": "c"}},
			}}})
		case "/api/v1/domaindata/query":
			ok(map[string]interface{}{"domaindata_id": "r1", "type": "table", "relative_uri": "missing-file", "author": "alice"})
		case "/api/v1/domaindata/create":
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			ok(map[string]interface{}{"domaindata_id": req["domaindata_id"]})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": 1, "message": "nope"}})
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return kuscia.NewClient(&kuscia.ClientConfig{Host: u.Hostname(), Port: port})
}

func dataContractDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NodeDO{}, &model.ProjectDO{}, &model.ProjectDatatableDO{}, &model.FeatureTableDO{},
		&model.ProjectFeatureTableDO{}, &model.TeeNodeDatatableManagementDO{}, &model.ProjectResultDO{}))
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func dataPostJSON(r http.Handler, path string, body interface{}) map[string]interface{} {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

func TestDataContract_DatatableListAndUploadDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	kc := dataContractKuscia(t)
	db := dataContractDB(t)
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})
	dtSvc := service.NewDatatableService(nil, nil, db, kc)
	dataH := NewDataHandler(db, kc).WithServices(service.NewDataService(db, kc, nil, t.TempDir()), nil, nil)
	r := gin.New()
	// Upload checks the Node-Id against the caller (EDGE owner alice).
	r.Use(func(c *gin.Context) {
		middleware.SetUser(c, &auth.UserContext{Name: "eu", OwnerType: auth.OwnerTypeEdge, OwnerID: "alice"})
		c.Next()
	})
	dtH := NewDatatableHandler(dtSvc, kc)
	r.POST("/datatable/list", dtH.List)
	r.POST("/datatable/delete", dtH.Delete)
	r.POST("/data/upload", dataH.Upload)
	r.POST("/data/download", dataH.Download)
	r.POST("/cloud_log/sls", dataH.CloudLogSls)
	r.POST("/feature_datasource/auth/list", dataH.FeatureDatasourceAuthList)

	out := dataPostJSON(r, "/datatable/list", map[string]interface{}{"ownerId": "alice", "pageSize": 10, "pageNumber": 1})
	data := out["data"].(map[string]interface{})
	assert.EqualValues(t, 1, data["totalDatatableNums"])
	item := data["datatableNodeVOList"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "Alice", item["nodeName"])
	vo := item["datatableVO"].(map[string]interface{})
	assert.Equal(t, "t1", vo["datatableId"])
	assert.Equal(t, "CSV", vo["type"])
	assert.Equal(t, "LOCAL", vo["datasourceType"])
	assert.Equal(t, "c", vo["schema"].([]interface{})[0].(map[string]interface{})["colComment"])
	_, hasAuth := vo["authProjects"]
	assert.False(t, hasAuth, "authProjects must be omitted (not null) when empty")

	// delete requires no project id
	out = dataPostJSON(r, "/datatable/delete", map[string]interface{}{"nodeId": "alice"})
	assert.NotEqualValues(t, 0, out["status"].(map[string]interface{})["code"])

	// upload multipart
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "x.csv")
	_, _ = fw.Write([]byte("a,b\n1,2\n"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/data/upload?Node-Id=alice", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var up map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &up))
	ud := up["data"].(map[string]interface{})
	assert.Equal(t, "x.csv", ud["name"])
	assert.True(t, strings.HasPrefix(ud["realName"].(string), "x_"))
	assert.Equal(t, "default-data-source", ud["datasource"])

	// upload to a foreign node is denied (any non-CENTER owner, not only EDGE)
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ = mw.CreateFormFile("file", "y.csv")
	_, _ = fw.Write([]byte("a\n1\n"))
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/data/upload?Node-Id=bob", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Body.String(), "202011602", "foreign Node-Id upload must be denied")

	// download of a missing result file -> "No data" attachment
	b, _ := json.Marshal(map[string]string{"nodeId": "alice", "domainDataId": "r1"})
	req = httptest.NewRequest(http.MethodPost, "/data/download", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	body, _ := io.ReadAll(w.Body)
	assert.Equal(t, "No data", string(body))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment;filename*=UTF-8''missing-file.csv")

	// cloud log unready structure
	out = dataPostJSON(r, "/cloud_log/sls", map[string]interface{}{"projectId": "p"})
	assert.Equal(t, map[string]interface{}{"config": false}, out["data"])

	out = dataPostJSON(r, "/feature_datasource/auth/list", map[string]interface{}{"projectId": "p", "nodeId": "alice"})
	assert.Equal(t, []interface{}{}, out["data"])
}
