package service

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// DataService implements Java DataServiceImpl (upload / createData / download).
type DataService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
	env          *EnvService
	storeDir     string
	pool         *kuscia.ClientPool
	log          *zap.Logger
}

// Upload store permissions. Uploaded CSVs are read by Kuscia DataProxy under a
// different uid, so group read is granted; "other" gets nothing.
const (
	uploadDirPerm  os.FileMode = 0o750
	uploadFilePerm os.FileMode = 0o640
)

// NewDataService creates a DataService. storeDir is the upload root
// (Java secretpad.data.dir-path); empty => $PRIVAHUB_DATA_DIR or ./data.
func NewDataService(db *gorm.DB, kc *kuscia.Client, env *EnvService, storeDir string) *DataService {
	if storeDir == "" {
		storeDir = os.Getenv("PRIVAHUB_DATA_DIR")
	}
	if storeDir == "" {
		storeDir = "./data"
	}
	if abs, err := filepath.Abs(storeDir); err == nil {
		storeDir = abs
	}
	// 0750: owner rwx, group r-x so a Kuscia/DataProxy reader that shares the
	// volume via a common group (fsGroup) can traverse; no world access.
	_ = os.MkdirAll(storeDir, uploadDirPerm) //nolint:gosec // G703: storeDir is operator config (data dir / PRIVAHUB_DATA_DIR), never request input
	return &DataService{db: db, kusciaClient: kc, env: env, storeDir: storeDir, log: zap.NewNop()}
}

// UploadDataResultVO is Java UploadDataResultVO.
type UploadDataResultVO struct {
	Name           string `json:"name"`
	RealName       string `json:"realName"`
	Datasource     string `json:"datasource"`
	DatasourceType string `json:"datasourceType"`
}

// DatatableSchema is Java DatatableSchema. The frontend also sends
// colName/colType/colComment, which are accepted as aliases.
type DatatableSchema struct {
	FeatureName        string `json:"featureName"`
	FeatureType        string `json:"featureType"`
	FeatureDescription string `json:"featureDescription"`
	ColName            string `json:"colName,omitempty"`
	ColType            string `json:"colType,omitempty"`
	ColComment         string `json:"colComment,omitempty"`
}

// CreateDataRequest is Java CreateDataRequest.
type CreateDataRequest struct {
	NodeID          string            `json:"nodeId"`
	Name            string            `json:"name"`
	RealName        string            `json:"realName"`
	TableName       string            `json:"tableName"`
	Description     string            `json:"description"`
	DatasourceType  string            `json:"datasourceType"`
	DatasourceName  string            `json:"datasourceName"`
	DatatableSchema []DatatableSchema `json:"datatableSchema"`
	NullStrs        []string          `json:"nullStrs"`
}

// DownloadDataRequest is Java DownloadDataRequest.
type DownloadDataRequest struct {
	NodeID       string `json:"nodeId"`
	DomainDataID string `json:"domainDataId"`
}

// DownloadInfo describes the payload of /data/download.
type DownloadInfo struct {
	FileName string
	// Path is the file or directory to stream ("" => nothing to send; the
	// handler writes "No data").
	Path string
	// Archive streams Path as tar.gz (directories, Model / Rule results).
	Archive bool
	// Size is the plain file size (-1 for archives: unknown, chunked).
	Size int64
}

// Empty reports whether there is no payload ("No data").
func (d *DownloadInfo) Empty() bool { return d == nil || d.Path == "" || (!d.Archive && d.Size == 0) }

// WriteTo streams the payload to w without buffering it in memory: a plain
// file is copied with io.Copy, an archive is tar.gz-encoded on the fly.
func (d *DownloadInfo) WriteTo(w io.Writer) (int64, error) {
	if d.Empty() {
		return 0, nil
	}
	cw := &countingWriter{w: w}
	if d.Archive {
		err := writeTarGz(cw, d.Path)
		return cw.n, err
	}
	f, err := os.Open(d.Path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	_, err = io.Copy(cw, f)
	return cw.n, err
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

const supportedUploadSuffix = ".csv"

// checkUploadFileName mirrors Java fileNameCheck.
func checkUploadFileName(name string) error {
	if name == "" {
		return bizErr(errcode.DataFileNameEmpty, "")
	}
	if strings.ContainsAny(name, "/\\") {
		return bizErr(errcode.DataIllegalParamsError, "file name cannot contains \\ or /")
	}
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return bizErr(errcode.DataFileTypeNotSupport, "does not support  type file.")
	}
	if suffix := name[i:]; suffix != supportedUploadSuffix {
		return bizErr(errcode.DataFileTypeNotSupport, "does not support "+suffix+" type file.")
	}
	return nil
}

func randomInt31() int64 {
	v, err := rand.Int(rand.Reader, big.NewInt(1<<31-1))
	if err != nil {
		return 0
	}
	return v.Int64()
}

// inDir reports whether path is inside dir (both absolute & cleaned).
func inDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Upload mirrors Java DataServiceImpl.upload: only *.csv, no path separators,
// random "<prefix>_<rand>.csv" name to avoid overwrites, whitelisted store
// path, UTF-8 BOM stripped. Size is limited only by the router body limit.
func (s *DataService) Upload(ctx context.Context, nodeID, fileName string, src io.Reader) (*UploadDataResultVO, error) {
	if err := checkUploadFileName(fileName); err != nil {
		return nil, err
	}
	if nodeID == "" {
		return nil, bizErr(errcode.DataIllegalParamsError, "Node-Id cannot be empty")
	}
	if strings.ContainsAny(nodeID, "/\\") || nodeID == "." || nodeID == ".." {
		return nil, bizErr(errcode.DataIllegalParamsError, "node ID cannot contains \\ or /")
	}
	dir := filepath.Join(s.storeDir, nodeID)
	i := strings.LastIndex(fileName, ".")
	prefix, suffix := fileName[:i], fileName[i:]
	var realName, target string
	for try := 0; try < 5; try++ {
		realName = fmt.Sprintf("%s_%d%s", prefix, randomInt31(), suffix)
		target = filepath.Join(dir, realName)
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
	}
	if !inDir(s.storeDir, target) {
		return nil, bizErr(errcode.DataIllegalParamsError, "file path not in whitelist")
	}
	if err := os.MkdirAll(dir, uploadDirPerm); err != nil {
		return nil, bizWrap(errcode.SystemUnknownError, err, "Failed to create new dir")
	}
	// target is a random name under dir, verified by inDir(s.storeDir, target)
	// above. 0640 instead of 0600: the reader is Kuscia DataProxy / the task
	// container (a different uid — root in the Docker lite image, or a
	// non-root uid sharing the volume through a common group/fsGroup in K8s);
	// group read keeps that working while world access stays denied.
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, uploadFilePerm) //nolint:gosec // G302/G304: path contained by inDir; 0640 needed for cross-uid DataProxy read via shared group

	if err != nil {
		if os.IsExist(err) {
			return nil, bizErr(errcode.DataFileExistsError, "")
		}
		return nil, bizWrap(errcode.SystemUnknownError, err, "")
	}
	br := bufio.NewReader(src)
	if head, _ := br.Peek(3); bytes.Equal(head, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	if _, err := io.Copy(out, br); err != nil {
		out.Close()
		_ = os.Remove(target)
		return nil, bizWrap(errcode.SystemUnknownError, err, "")
	}
	if err := out.Close(); err != nil {
		return nil, bizWrap(errcode.SystemUnknownError, err, "")
	}
	return &UploadDataResultVO{
		Name: fileName, RealName: realName,
		Datasource: DefaultLocalDatasourceID, DatasourceType: DefaultLocalKusciaType,
	}, nil
}

// CreateData mirrors Java DataManager.createData: registers the uploaded file
// as a Kuscia DomainData (relative_uri = realName) and returns its id.
func (s *DataService) CreateData(ctx context.Context, req *CreateDataRequest) (string, error) {
	if req.NodeID == "" || req.Name == "" || req.TableName == "" || req.DatasourceType == "" || req.DatasourceName == "" {
		return "", bizErr(errcode.ParamError, "nodeId/name/tableName/datasourceType/datasourceName cannot be blank")
	}
	if s.env != nil && s.env.IsAutonomy() && s.env.GetPlatformNodeId() != "" && req.NodeID != s.env.GetPlatformNodeId() {
		return "", bizErr(errcode.NodeNotExistError, "The nodeId is not the platform node id.")
	}
	if s.kusciaClient == nil {
		return "", bizErr(errcode.KusciaRpcError, "kuscia client not configured")
	}
	realName := req.RealName
	if realName == "" {
		realName = req.Name
	}
	if strings.Contains(realName, "..") {
		return "", bizErr(errcode.DataIllegalParamsError, "realName contains invalid character")
	}
	nullStrs := req.NullStrs
	if nullStrs == nil {
		nullStrs = []string{}
	}
	nullJSON, _ := json.Marshal(nullStrs)
	cols := make([]kuscia.DataColumn, 0, len(req.DatatableSchema))
	for _, c := range req.DatatableSchema {
		col := kuscia.DataColumn{Name: c.FeatureName, Type: c.FeatureType, Comment: c.FeatureDescription}
		if col.Name == "" {
			col.Name = c.ColName
		}
		if col.Type == "" {
			col.Type = c.ColType
		}
		if col.Comment == "" {
			col.Comment = c.ColComment
		}
		cols = append(cols, col)
	}
	id, err := s.kc(req.NodeID).CreateDomainDataWithID(ctx, &kuscia.CreateDomainDataRequest{
		DomainDataID: randomLetters(8),
		DomainID:     req.NodeID,
		Name:         req.TableName,
		Type:         kuscia.DomainDataTypeTable,
		RelativeURI:  realName,
		Attributes: map[string]string{
			attrDatasourceType: req.DatasourceType,
			attrDatasourceName: req.DatasourceName,
			attrNullStrs:       string(nullJSON),
			attrDescription:    req.Description,
		},
		Columns: cols,
	})
	if err != nil {
		return "", bizWrap(errcode.DataIllegalParamsError, err, "")
	}
	return id, nil
}

// Download mirrors Java DataServiceImpl.download: resolves the relative URI
// through Kuscia DomainData + project_result, validates it stays inside
// ${storeDir}/<nodeId>, and returns a CSV, or a tar.gz for directories and
// Model/Rule results. Missing files yield an empty payload ("No data").
func (s *DataService) Download(ctx context.Context, req *DownloadDataRequest) (*DownloadInfo, error) {
	if req.NodeID == "" || req.DomainDataID == "" {
		return nil, bizErr(errcode.ParamError, "nodeId/domainDataId cannot be blank")
	}
	if strings.ContainsAny(req.NodeID, "/\\") || req.NodeID == ".." {
		return nil, bizErr(errcode.DataIllegalParamsError, "node ID cannot contains \\ or /")
	}
	if s.kusciaClient == nil {
		return nil, bizErr(errcode.KusciaRpcError, "kuscia client not configured")
	}
	resp, err := s.kc(req.NodeID).QueryDomainData(ctx, req.NodeID, req.DomainDataID)
	if err != nil {
		return nil, bizWrap(errcode.DatatableQueryDatatableFailed, err, "")
	}
	kind := resp.Data.Type
	if s.db != nil {
		var pr model.ProjectResultDO
		if err := s.db.WithContext(ctx).Where("node_id = ? AND ref_id = ?", req.NodeID, req.DomainDataID).First(&pr).Error; err == nil {
			kind = pr.Kind
		} else {
			// Java requires the project_result row; tolerated here because
			// result persistence is still being migrated. The Kuscia type is used.
			s.log.Warn("project_result not found for download, using kuscia type",
				zap.String("node", req.NodeID), zap.String("domainData", req.DomainDataID))
		}
	}
	relativeURI := resp.Data.RelativeURI
	if relativeURI == "" {
		return nil, bizErr(errcode.DataIllegalParamsError, "relative uri is empty!")
	}
	if strings.Contains(relativeURI, "..") {
		return nil, bizErr(errcode.DataIllegalParamsError, "relative uri "+relativeURI+" contains invalid character")
	}
	dir := filepath.Join(s.storeDir, req.NodeID)
	path := filepath.Join(dir, relativeURI)
	if !inDir(dir, path) {
		return nil, bizErr(errcode.DataFileNotExistsError, "")
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		if realDir, err2 := filepath.EvalSymlinks(dir); err2 == nil && !inDir(realDir, real) {
			return nil, bizErr(errcode.DataFileNotExistsError, "")
		}
	}
	base := filepath.Base(relativeURI)
	st, err := os.Stat(path)
	if err != nil {
		return &DownloadInfo{FileName: base + ".csv"}, nil
	}
	lk := strings.ToLower(kind)
	if st.IsDir() || strings.Contains(lk, "model") || strings.Contains(lk, "rule") {
		return &DownloadInfo{FileName: base + ".tar.gz", Path: path, Archive: true, Size: -1}, nil
	}
	if !st.Mode().IsRegular() {
		return nil, bizErr(errcode.DataFileNotExistsError, "")
	}
	return &DownloadInfo{FileName: base + ".csv", Path: path, Size: st.Size()}, nil
}

// writeTarGz streams a file or directory tree as tar.gz to w (entries
// relative to its parent; symlinks are skipped).
func writeTarGz(w io.Writer, root string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	parent := filepath.Dir(root)
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(parent, p)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(p) //nolint:gosec // G304: p is walked under root, which DataService.download contained with inDir + EvalSymlinks; symlinks are skipped

			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// WithKusciaPool routes per-node Kuscia calls through the multi-node pool.
func (s *DataService) WithKusciaPool(p *kuscia.ClientPool) *DataService {
	s.pool = p
	return s
}

// kc returns the Kuscia client serving nodeID (pool entry or the default client).
func (s *DataService) kc(nodeID string) *kuscia.Client {
	if s.pool != nil {
		if c := s.pool.Get(nodeID); c != nil {
			return c
		}
	}
	return s.kusciaClient
}
