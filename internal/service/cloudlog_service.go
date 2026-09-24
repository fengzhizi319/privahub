package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // SLS request signing mandates HMAC-SHA1
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// slsLogStorePrefix is Java SLSCloudLogServiceImpl.LOG_STORE_PREFIX.
const slsLogStorePrefix = "secretpad-engin-log-"

// GraphNodeCloudLogsRequest is Java GraphNodeCloudLogsRequest.
type GraphNodeCloudLogsRequest struct {
	ProjectID    string `json:"projectId"`
	GraphNodeID  string `json:"graphNodeId"`
	JobID        string `json:"jobId"`
	TaskID       string `json:"taskId"`
	QueryParties *bool  `json:"queryParties"`
	NodeID       string `json:"nodeId"`
}

// CloudGraphNodeTaskLogsVO is Java CloudGraphNodeTaskLogsVO. Unset fields are
// omitted (Java serializes them as null).
type CloudGraphNodeTaskLogsVO struct {
	Status      string           `json:"status,omitempty"`
	Logs        []string         `json:"logs,omitempty"`
	Config      bool             `json:"config"`
	NodeParties []NodeSimpleInfo `json:"nodeParties,omitempty"`
}

// CloudLogService mirrors Java CloudLogController + SLSCloudLogServiceImpl.
// When SLS is not fully configured it returns Java's "unready" result
// ({config:false}). When configured, it queries the per-node logstore
// "secretpad-engin-log-<nodeId>" through the SLS GetLogs REST API with the
// standard "LOG ak:signature" (HMAC-SHA1) authentication. Project/logstore
// auto-creation of the Java implementation is not migrated (create them
// out-of-band).
type CloudLogService struct {
	db         *gorm.DB
	env        *EnvService
	cfg        config.SLSConfig
	httpClient *http.Client
	now        func() time.Time
}

// NewCloudLogService creates a CloudLogService.
func NewCloudLogService(db *gorm.DB, env *EnvService, cfg config.SLSConfig) *CloudLogService {
	// The SLS host is operator configuration (not user input), so no IP
	// filter; redirects are still never followed (signed requests, SSRF).
	hc := &http.Client{Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &CloudLogService{db: db, env: env, cfg: cfg, httpClient: hc, now: time.Now}
}

// Configured reports whether SLS is configured.
func (s *CloudLogService) Configured() bool { return s != nil && s.cfg.Configured() }

// FetchLog mirrors Java getCloudLog / SLSCloudLogServiceImpl.fetchLog.
func (s *CloudLogService) FetchLog(ctx context.Context, req *GraphNodeCloudLogsRequest) (*CloudGraphNodeTaskLogsVO, error) {
	if !s.Configured() {
		return &CloudGraphNodeTaskLogsVO{Config: false}, nil
	}
	if req.ProjectID == "" {
		return nil, bizErr(errcode.ParamError, "projectId cannot be blank")
	}
	var task model.ProjectJobTaskDO
	found := false
	switch {
	case req.JobID != "" && req.TaskID != "":
		found = s.db.WithContext(ctx).Where("project_id = ? AND job_id = ? AND task_id = ?", req.ProjectID, req.JobID, req.TaskID).
			First(&task).Error == nil
	case req.QueryParties != nil && *req.QueryParties && req.GraphNodeID != "":
		if s.latestTask(ctx, req, &task) {
			return &CloudGraphNodeTaskLogsVO{Config: true, NodeParties: s.nodeParties(ctx, task.Parties)}, nil
		}
	case req.GraphNodeID == "" && req.JobID == "" && req.TaskID == "":
		return &CloudGraphNodeTaskLogsVO{Config: true}, nil
	default:
		found = s.latestTask(ctx, req, &task)
	}
	if !found {
		return nil, bizErr(errcode.ProjectJobTaskNotExists, "")
	}
	if s.env != nil && s.env.IsAutonomy() && !strings.EqualFold(s.env.GetPlatformNodeId(), req.NodeID) {
		return nil, bizErr(errcode.ProjectJobNodePermissionError, "")
	}
	query := fmt.Sprintf(`* |select content where "__tag__:__path__" like '%%%s%%' limit %d`, task.TaskID, 2147483647)
	logs, err := s.getLogs(ctx, slsLogStorePrefix+req.NodeID, query)
	if err != nil {
		return nil, bizWrap(errcode.ProjectJobCloudLogError, err, err.Error())
	}
	return &CloudGraphNodeTaskLogsVO{Status: task.Status, Logs: logs, Config: true}, nil
}

func (s *CloudLogService) latestTask(ctx context.Context, req *GraphNodeCloudLogsRequest, task *model.ProjectJobTaskDO) bool {
	return s.db.WithContext(ctx).Where("project_id = ? AND graph_node_id = ?", req.ProjectID, req.GraphNodeID).
		Order("gmt_create DESC").First(task).Error == nil
}

func (s *CloudLogService) nodeParties(ctx context.Context, parties string) []NodeSimpleInfo {
	var ids []string
	if err := json.Unmarshal([]byte(parties), &ids); err != nil {
		for _, p := range strings.Split(parties, ",") {
			if p = strings.TrimSpace(p); p != "" {
				ids = append(ids, p)
			}
		}
	}
	out := make([]NodeSimpleInfo, 0, len(ids))
	if len(ids) == 0 {
		return out
	}
	var nodes []model.NodeDO
	s.db.WithContext(ctx).Where("node_id IN ?", ids).Find(&nodes)
	for _, n := range nodes {
		out = append(out, NodeSimpleInfo{NodeID: n.NodeID, NodeName: n.Name})
	}
	return out
}

// slsSign builds the SLS Authorization header value.
func slsSign(ak, sk, method, resource string, params url.Values, header http.Header) string {
	var logHeaders []string
	for k, v := range header {
		lk := strings.ToLower(k)
		if (strings.HasPrefix(lk, "x-log-") || strings.HasPrefix(lk, "x-acs-")) && len(v) > 0 {
			logHeaders = append(logHeaders, lk+":"+v[0])
		}
	}
	sort.Strings(logHeaders)
	res := resource
	if len(params) > 0 {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+params.Get(k))
		}
		res += "?" + strings.Join(parts, "&")
	}
	sts := strings.Join([]string{
		method, header.Get("Content-MD5"), header.Get("Content-Type"), header.Get("Date"),
		strings.Join(logHeaders, "\n"), res,
	}, "\n")
	mac := hmac.New(sha1.New, []byte(sk))
	mac.Write([]byte(sts))
	return "LOG " + ak + ":" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// getLogs calls SLS GetLogs and returns the "content" values.
func (s *CloudLogService) getLogs(ctx context.Context, logstore, query string) ([]string, error) {
	host := strings.TrimSpace(s.cfg.Host)
	scheme := "https"
	if i := strings.Index(host, "://"); i >= 0 {
		scheme, host = host[:i], host[i+3:]
	}
	host = strings.TrimRight(host, "/")
	resource := "/logstores/" + logstore
	params := url.Values{}
	params.Set("type", "log")
	params.Set("from", "0")
	params.Set("to", strconv.FormatInt(s.now().Unix(), 10))
	params.Set("query", query)
	params.Set("topic", "")
	u := fmt.Sprintf("%s://%s.%s%s?%s", scheme, s.cfg.Project, host, resource, strings.ReplaceAll(params.Encode(), "+", "%20"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Date", s.now().UTC().Format(http.TimeFormat))
	req.Header.Set("x-log-apiversion", "0.6.0")
	req.Header.Set("x-log-signaturemethod", "hmac-sha1")
	req.Header.Set("x-log-bodyrawsize", "0")
	req.Header.Set("Authorization", slsSign(s.cfg.AK, s.cfg.SK, http.MethodGet, resource, params, req.Header))
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			ErrorCode    string `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
		}
		_ = json.Unmarshal(body, &e)
		if e.ErrorCode != "" {
			return nil, fmt.Errorf("%s: %s", e.ErrorCode, e.ErrorMessage)
		}
		return nil, fmt.Errorf("sls HTTP %d", resp.StatusCode)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	logs := make([]string, 0, len(items))
	for _, it := range items {
		if v, ok := it["content"]; ok {
			logs = append(logs, fmt.Sprint(v))
		}
	}
	return logs, nil
}
