package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scritchley/orc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func writeTestORC(t *testing.T, path string) {
	t.Helper()
	schema, err := orc.ParseSchema("struct<id:string,age:bigint,income:double,ok:boolean>")
	require.NoError(t, err)
	f, err := os.Create(path)
	require.NoError(t, err)
	w, err := orc.NewWriter(f, orc.SetSchema(schema))
	require.NoError(t, err)
	require.NoError(t, w.Write("u1", int64(18), 4879.7, true))
	require.NoError(t, w.Write("u,2", int64(34), 0.25, false))
	require.NoError(t, w.Write("u3", nil, nil, nil))
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
}

// Regression (system test 2026-09-25): SecretFlow writes component output
// tables as ORC; data/download sent the ORC bytes under a ".csv" name.
func TestDataService_DownloadORCAsCSV(t *testing.T) {
	fk := newDataFakeKuscia(t)
	db := newDataTestDB(t)
	dir := t.TempDir()
	s := NewDataService(db, fk.client(), nil, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "alice"), 0o750))
	writeTestORC(t, filepath.Join(dir, "alice", "job_node_3_output_0"))
	fk.domaindata["alice"] = map[string]kuscia.DomainData{}
	fk.domaindata["alice"]["o1"] = kuscia.DomainData{DomainDataID: "o1", Type: "table", RelativeURI: "job_node_3_output_0"}

	info, err := s.Download(context.Background(), &DownloadDataRequest{NodeID: "alice", DomainDataID: "o1"})
	require.NoError(t, err)
	assert.True(t, info.ORC)
	assert.False(t, info.Empty())
	assert.Equal(t, int64(-1), info.Size, "converted size is unknown: no Content-Length")
	assert.Equal(t, "job_node_3_output_0.csv", info.FileName)
	assert.Equal(t, "id,age,income,ok\nu1,18,4879.7,True\n\"u,2\",34,0.25,False\nu3,,,\n", downloadBody(t, info))
}

func TestDataService_DownloadCorruptORCSentRaw(t *testing.T) {
	fk := newDataFakeKuscia(t)
	db := newDataTestDB(t)
	dir := t.TempDir()
	s := NewDataService(db, fk.client(), nil, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "alice"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alice", "bad"), []byte("ORC-not-really"), 0o600))
	fk.domaindata["alice"] = map[string]kuscia.DomainData{}
	fk.domaindata["alice"]["b1"] = kuscia.DomainData{DomainDataID: "b1", Type: "table", RelativeURI: "bad"}

	info, err := s.Download(context.Background(), &DownloadDataRequest{NodeID: "alice", DomainDataID: "b1"})
	require.NoError(t, err)
	assert.False(t, info.ORC)
	assert.Equal(t, "bad.orc", info.FileName)
	assert.Equal(t, "ORC-not-really", downloadBody(t, info))
}
