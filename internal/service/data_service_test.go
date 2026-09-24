package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func TestDataService_UploadCreateDownload(t *testing.T) {
	fk := newDataFakeKuscia(t)
	db := newDataTestDB(t)
	dir := t.TempDir()
	s := NewDataService(db, fk.client(), nil, dir)
	ctx := context.Background()

	_, err := s.Upload(ctx, "alice", "a.txt", strings.NewReader("x"))
	assert.Equal(t, errcode.DataFileTypeNotSupport.Code, dataBizCode(err))
	_, err = s.Upload(ctx, "alice", "../a.csv", strings.NewReader("x"))
	assert.Equal(t, errcode.DataIllegalParamsError.Code, dataBizCode(err))
	_, err = s.Upload(ctx, "../alice", "a.csv", strings.NewReader("x"))
	assert.Equal(t, errcode.DataIllegalParamsError.Code, dataBizCode(err))

	vo, err := s.Upload(ctx, "alice", "a.csv", strings.NewReader("\xEF\xBB\xBFid,v\n1,2\n"))
	require.NoError(t, err)
	assert.Equal(t, "a.csv", vo.Name)
	assert.True(t, strings.HasPrefix(vo.RealName, "a_") && strings.HasSuffix(vo.RealName, ".csv"))
	assert.Equal(t, "default-data-source", vo.Datasource)
	assert.Equal(t, "localfs", vo.DatasourceType)
	content, err := os.ReadFile(filepath.Join(dir, "alice", vo.RealName))
	require.NoError(t, err)
	assert.Equal(t, "id,v\n1,2\n", string(content))

	id, err := s.CreateData(ctx, &CreateDataRequest{NodeID: "alice", Name: "a.csv", RealName: vo.RealName, TableName: "tbl",
		DatasourceType: "LOCAL", DatasourceName: "default-data-source",
		DatatableSchema: []DatatableSchema{{FeatureName: "id", FeatureType: "str", FeatureDescription: "d"}, {ColName: "v", ColType: "int"}}})
	require.NoError(t, err)
	dd := fk.domaindata["alice"][id]
	assert.Equal(t, vo.RealName, dd.RelativeURI)
	assert.Equal(t, "tbl", dd.Name)
	assert.Equal(t, "v", dd.Columns[1].Name)
	assert.Equal(t, "[]", dd.Attributes["NullStrs"])

	info, err := s.Download(ctx, &DownloadDataRequest{NodeID: "alice", DomainDataID: id})
	require.NoError(t, err)
	assert.Equal(t, "id,v\n1,2\n", downloadBody(t, info))
	assert.Equal(t, vo.RealName+".csv", info.FileName)

	// directory result -> tar.gz
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "alice", "out1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alice", "out1", "f.txt"), []byte("hi"), 0o644))
	fk.domaindata["alice"]["m1"] = kuscia.DomainData{DomainDataID: "m1", Type: "model", RelativeURI: "out1"}
	db.Create(&model.ProjectResultDO{ProjectID: "p", Kind: "Model", NodeID: "alice", RefID: "m1"})
	info, err = s.Download(ctx, &DownloadDataRequest{NodeID: "alice", DomainDataID: "m1"})
	require.NoError(t, err)
	assert.Equal(t, "out1.tar.gz", info.FileName)
	gz, err := gzip.NewReader(bytes.NewReader([]byte(downloadBody(t, info))))
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	names := []string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		names = append(names, h.Name)
	}
	assert.Contains(t, names, "out1/f.txt")

	// missing file -> empty payload ("No data")
	fk.domaindata["alice"]["e1"] = kuscia.DomainData{DomainDataID: "e1", Type: "table", RelativeURI: "nothing"}
	info, err = s.Download(ctx, &DownloadDataRequest{NodeID: "alice", DomainDataID: "e1"})
	require.NoError(t, err)
	assert.True(t, info.Empty())
	assert.Empty(t, downloadBody(t, info))

	// traversal
	fk.domaindata["alice"]["x1"] = kuscia.DomainData{DomainDataID: "x1", Type: "table", RelativeURI: "../../etc/passwd"}
	_, err = s.Download(ctx, &DownloadDataRequest{NodeID: "alice", DomainDataID: "x1"})
	assert.Equal(t, errcode.DataIllegalParamsError.Code, dataBizCode(err))
	_, err = s.Download(ctx, &DownloadDataRequest{NodeID: "alice", DomainDataID: "nope"})
	assert.Equal(t, errcode.DatatableQueryDatatableFailed.Code, dataBizCode(err))
}

func TestFeatureTableService_CreateAndAuthList(t *testing.T) {
	db := newDataTestDB(t)
	s := NewFeatureTableService(db)
	ctx := context.Background()
	req := &CreateFeatureDatasourceRequest{OwnerID: "alice", NodeIDs: []string{"alice", "bob"}, FeatureTableName: "f1",
		Type: "HTTP", URL: "http://127.0.0.1:8080/f", Columns: []TableColumnCompat{{ColName: "a", ColType: "int"}}}
	assert.Equal(t, errcode.FeatureTableIpFilter.Code, dataBizCode(s.CreateFeatureTable(ctx, req)))
	off := false
	s.WithIPFilter(NewIPFilter(&config.IPBlockConfig{Enable: &off}))
	require.NoError(t, s.CreateFeatureTable(ctx, req))
	bad := *req
	bad.Type = "OSS"
	assert.Error(t, s.CreateFeatureTable(ctx, &bad))

	list, err := s.FeatureDatasourceList(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Len(t, list[0].FeatureTableID, 8)
	var ft model.FeatureTableDO
	require.NoError(t, db.Where("feature_table_id = ?", list[0].FeatureTableID).First(&ft).Error)
	assert.Equal(t, StatusUnavailable, ft.Status)

	db.Create(&model.ProjectFeatureTableDO{ProjectID: "p1", NodeID: "alice", FeatureTableID: ft.FeatureTableID, TableConfigs: "[]", Source: "x"})
	auth, err := s.ProjectFeatureTableList(ctx, "alice", "p1")
	require.NoError(t, err)
	require.Len(t, auth, 1)
	assert.Equal(t, "f1", auth[0].FeatureTableName)
	assert.Equal(t, "a", auth[0].Columns[0].ColName)
	empty, err := s.ProjectFeatureTableList(ctx, "alice", "p2")
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestCloudLogService(t *testing.T) {
	db := newDataTestDB(t)
	ctx := context.Background()
	s := NewCloudLogService(db, nil, config.SLSConfig{})
	vo, err := s.FetchLog(ctx, &GraphNodeCloudLogsRequest{ProjectID: "p"})
	require.NoError(t, err)
	assert.False(t, vo.Config)

	s = NewCloudLogService(db, nil, config.SLSConfig{AK: "a", SK: "b", Host: "h", Project: "pr"})
	vo, err = s.FetchLog(ctx, &GraphNodeCloudLogsRequest{ProjectID: "p"})
	require.NoError(t, err)
	assert.True(t, vo.Config)
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})
	db.Create(&model.ProjectJobTaskDO{ProjectID: "p", JobID: "j", TaskID: "t", Parties: `["alice"]`, Status: "SUCCEED", GraphNodeID: "g1"})
	yes := true
	vo, err = s.FetchLog(ctx, &GraphNodeCloudLogsRequest{ProjectID: "p", GraphNodeID: "g1", QueryParties: &yes})
	require.NoError(t, err)
	require.Len(t, vo.NodeParties, 1)
	assert.Equal(t, "Alice", vo.NodeParties[0].NodeName)
	_, err = s.FetchLog(ctx, &GraphNodeCloudLogsRequest{ProjectID: "p", JobID: "x", TaskID: "y"})
	assert.Equal(t, errcode.ProjectJobTaskNotExists.Code, dataBizCode(err))
	// signature is deterministic and uses the LOG scheme
	assert.True(t, strings.HasPrefix(slsSign("ak", "sk", "GET", "/logstores/l", nil, map[string][]string{"Date": {"d"}}), "LOG ak:"))
}

// downloadBody streams a download payload into memory (tests only).
func downloadBody(t *testing.T, info *DownloadInfo) string {
	t.Helper()
	var buf bytes.Buffer
	_, err := info.WriteTo(&buf)
	require.NoError(t, err)
	return buf.String()
}
