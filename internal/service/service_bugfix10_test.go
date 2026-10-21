package service

// Regression tests for confirmed bugs (round 18).
//
// Bug A: ListNodeResults applied no upper clamp on PageSize, so a huge
//        pageSize could pull the whole ProjectResult table in one query.
// Bug B: checkUploadFileName compared the suffix case-sensitively, rejecting
//        "DATA.CSV".
// Bug C: randomInt31 silently degraded to 0 on a crypto/rand failure, making
//        concurrent uploads race on "<prefix>_0.csv" and surface as a
//        confusing DataFileExistsError.
// Bug D: writeTarGz emitted a "./" root entry that strict extractors reject.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// --- Bug A: unbounded pagination in ListNodeResults ---

func TestNodeResults_PageSizeClamped(t *testing.T) {
	db := newDataTestDB(t)
	ns := NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), nil, db)
	rows := make([]model.ProjectResultDO, 0, 25)
	for i := 0; i < 25; i++ {
		rows = append(rows, model.ProjectResultDO{
			ProjectID: "p1", Kind: "Model", NodeID: "alice", RefID: fmt.Sprintf("out-%03d", i), JobID: "job1",
		})
	}
	require.NoError(t, db.Create(&rows).Error)

	ctx := context.Background()
	// A huge pageSize must not fetch the whole table: normalizePage caps it
	// (size > 1000 falls back to the default 10) while the total is intact.
	vo, err := ns.ListNodeResults(ctx, &ListNodeResultRequest{OwnerID: "alice", PageNumber: 1, PageSize: 100000000})
	require.NoError(t, err)
	assert.Equal(t, 25, vo.TotalNodeResultNums)
	assert.Len(t, vo.NodeAllResultsVOList, 10)

	// A regular page still paginates as requested (no overlap with page 1).
	page1 := vo
	vo, err = ns.ListNodeResults(ctx, &ListNodeResultRequest{OwnerID: "alice", PageNumber: 2, PageSize: 10})
	require.NoError(t, err)
	assert.Len(t, vo.NodeAllResultsVOList, 10)
	seen := map[string]bool{}
	for _, r := range page1.NodeAllResultsVOList {
		seen[r.NodeResultsVO.DomainDataID] = true
	}
	for _, r := range vo.NodeAllResultsVOList {
		assert.False(t, seen[r.NodeResultsVO.DomainDataID], "page 2 overlaps page 1 at %s", r.NodeResultsVO.DomainDataID)
	}

	// PageSize = 0 keeps the historical "no pagination" behavior.
	vo, err = ns.ListNodeResults(ctx, &ListNodeResultRequest{OwnerID: "alice", PageNumber: 1, PageSize: 0})
	require.NoError(t, err)
	assert.Len(t, vo.NodeAllResultsVOList, 25)
}

// --- Bug B: case-sensitive upload suffix ---

func TestUploadFileName_MixedCaseSuffixAccepted(t *testing.T) {
	s := NewDataService(newDataTestDB(t), nil, nil, t.TempDir())
	ctx := context.Background()

	vo, err := s.Upload(ctx, "alice", "DATA.CSV", strings.NewReader("id,v\n1,2\n"))
	require.NoError(t, err)
	assert.Equal(t, "DATA.CSV", vo.Name)
	assert.True(t, strings.HasPrefix(vo.RealName, "DATA_"), "realName = %q", vo.RealName)
	assert.True(t, strings.HasSuffix(strings.ToLower(vo.RealName), ".csv"), "realName = %q", vo.RealName)

	// Non-csv extensions are still rejected.
	_, err = s.Upload(ctx, "alice", "data.xlsx", strings.NewReader("x"))
	assert.Equal(t, errcode.DataFileTypeNotSupport.Code, dataBizCode(err))
}

// --- Bug C: randomInt31 error propagation ---

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestRandomInt31_Range(t *testing.T) {
	for i := 0; i < 100; i++ {
		v, err := randomInt31()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, v, int64(0))
		assert.LessOrEqual(t, v, int64(1<<31-1))
	}
}

func TestUpload_RandFailureReturnsError(t *testing.T) {
	s := NewDataService(newDataTestDB(t), nil, nil, t.TempDir())

	orig := rand.Reader
	rand.Reader = failingReader{}
	defer func() { rand.Reader = orig }()

	// A crypto/rand failure must surface as a real error, not as the
	// confusing DataFileExistsError caused by every upload racing on
	// "<prefix>_0.csv".
	_, err := s.Upload(context.Background(), "alice", "a.csv", strings.NewReader("x"))
	require.Error(t, err)
	assert.Equal(t, errcode.SystemUnknownError.Code, dataBizCode(err))
}

// --- Bug D: writeTarGz root entry ---

func TestWriteTarGz_NoRootEntry(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "model-out")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "f.txt"), []byte("hi"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "g.txt"), []byte("yo"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, writeTarGz(&buf, root))
	gz, err := gzip.NewReader(&buf)
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
	assert.NotContains(t, names, ".", "root directory must not be emitted as a ./ entry")
	assert.NotContains(t, names, "./")
	for _, n := range names {
		assert.NotEqual(t, ".", n)
	}
	assert.Contains(t, names, "model-out/f.txt")
	assert.Contains(t, names, "model-out/sub/g.txt")
}
