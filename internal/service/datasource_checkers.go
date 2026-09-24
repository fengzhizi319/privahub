package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // ODPS request signing mandates HMAC-SHA1
	"database/sql"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
)

// --- MySQL (Java MysqlManager) ---

// MysqlConfig mirrors Java MysqlConfig.
type MysqlConfig struct {
	Endpoint string // host:port
	User     string
	Password string
	Database string
}

// MysqlChecker performs real MySQL connectivity checks via go-sql-driver/mysql.
// Connections go through a per-checker registered dial function that
// enforces the SSRF IPFilter on the actual remote address.
type MysqlChecker struct {
	Timeout time.Duration
	// open is overridable in tests.
	open func(dsn string) (*sql.DB, error)
	// network is the go-sql-driver network name bound to the guarded dialer.
	network string
}

var mysqlDialSeq atomic.Int64

// NewMysqlChecker creates a MysqlChecker with a 5s timeout and no IP
// filtering (use NewMysqlCheckerWithFilter for user supplied endpoints).
func NewMysqlChecker() *MysqlChecker {
	return NewMysqlCheckerWithFilter(NewDisabledIPFilter())
}

// NewMysqlCheckerWithFilter creates a MysqlChecker whose connections are
// refused when the remote IP is blocked by f (checked at connect time, so DNS
// rebinding cannot bypass it).
func NewMysqlCheckerWithFilter(f *IPFilter) *MysqlChecker {
	if f == nil {
		f = NewDisabledIPFilter()
	}
	network := fmt.Sprintf("privahub-guarded-%d", mysqlDialSeq.Add(1))
	mysql.RegisterDialContext(network, func(ctx context.Context, addr string) (net.Conn, error) {
		return f.DialContext(ctx, "tcp", addr)
	})
	return &MysqlChecker{Timeout: 5 * time.Second, network: network,
		open: func(dsn string) (*sql.DB, error) { return sql.Open("mysql", dsn) }}
}

func (m *MysqlChecker) dsn(cfg *MysqlConfig) string {
	c := mysql.NewConfig()
	c.User = cfg.User
	c.Passwd = cfg.Password
	c.Net = "tcp"
	if m.network != "" {
		c.Net = m.network
	}
	c.Addr = cfg.Endpoint
	c.DBName = cfg.Database
	c.Timeout = m.Timeout
	c.ReadTimeout = m.Timeout
	c.WriteTimeout = m.Timeout
	return c.FormatDSN()
}

// TestConnection runs "SELECT 1" (Java MysqlManager.testConnection).
func (m *MysqlChecker) TestConnection(ctx context.Context, cfg *MysqlConfig) error {
	db, err := m.open(m.dsn(cfg))
	if err != nil {
		return fmt.Errorf("MySQL test connection failed: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("MySQL test connection failed: %w", err)
	}
	return nil
}

// TableExists checks information_schema (Java MysqlManager.testTableExists);
// any error yields false.
func (m *MysqlChecker) TableExists(ctx context.Context, cfg *MysqlConfig, database, table string) bool {
	db, err := m.open(m.dsn(cfg))
	if err != nil {
		return false
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?",
		database, table).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// --- ODPS / MaxCompute (Java OdpsManager) ---

// OdpsConfig mirrors Java OdpsConfig.
type OdpsConfig struct {
	Endpoint  string // e.g. https://service.cn-hangzhou.maxcompute.aliyun.com/api
	AccessID  string
	AccessKey string
	Project   string
}

// OdpsChecker checks ODPS connectivity through the ODPS REST API with the
// standard "ODPS <ak>:<HMAC-SHA1>" request signature (same algorithm as the
// official Java/Python SDKs). No Go SDK is vendored, so instead of the Java
// "show flags;" SQL task (which needs the full task/instance protocol) the
// check reads the project metadata (GET /projects/{project}), which validates
// endpoint, credentials and project access in one signed call. Table existence
// uses GET /projects/{project}/tables/{table}.
type OdpsChecker struct {
	httpClient *http.Client
	now        func() time.Time
}

// NewOdpsChecker creates an OdpsChecker without IP filtering (use
// NewOdpsCheckerWithFilter for user supplied endpoints).
func NewOdpsChecker() *OdpsChecker {
	return NewOdpsCheckerWithFilter(NewDisabledIPFilter())
}

// NewOdpsCheckerWithFilter creates an OdpsChecker whose HTTP client refuses
// blocked remote addresses at connect time and never follows redirects.
func NewOdpsCheckerWithFilter(f *IPFilter) *OdpsChecker {
	if f == nil {
		f = NewDisabledIPFilter()
	}
	return &OdpsChecker{httpClient: f.HTTPClient(10 * time.Second), now: time.Now}
}

type odpsError struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// odpsCanonical builds the ODPS string-to-sign.
func odpsCanonical(method, resource string, query url.Values, header http.Header) string {
	var lines []string
	lines = append(lines, method)
	signHeaders := map[string]string{
		"content-md5":  header.Get("Content-MD5"),
		"content-type": header.Get("Content-Type"),
		"date":         header.Get("Date"),
	}
	for k, v := range header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-odps-") && len(v) > 0 {
			signHeaders[lk] = v[0]
		}
	}
	keys := make([]string, 0, len(signHeaders))
	for k := range signHeaders {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.HasPrefix(k, "x-odps-") {
			lines = append(lines, k+":"+signHeaders[k])
		} else {
			lines = append(lines, signHeaders[k])
		}
	}
	res := resource
	if len(query) > 0 {
		qk := make([]string, 0, len(query))
		for k := range query {
			qk = append(qk, k)
		}
		sort.Strings(qk)
		parts := make([]string, 0, len(qk))
		for _, k := range qk {
			if v := query.Get(k); v != "" {
				parts = append(parts, k+"="+v)
			} else {
				parts = append(parts, k)
			}
		}
		res += "?" + strings.Join(parts, "&")
	}
	lines = append(lines, res)
	return strings.Join(lines, "\n")
}

func (o *OdpsChecker) get(ctx context.Context, cfg *OdpsConfig, resource string) (*http.Response, error) {
	ep := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if !strings.Contains(ep, "://") {
		ep = "http://" + ep
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep+resource, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Date", o.now().UTC().Format(http.TimeFormat))
	req.Header.Set("x-odps-user-agent", "privahub")
	mac := hmac.New(sha1.New, []byte(cfg.AccessKey))
	mac.Write([]byte(odpsCanonical(http.MethodGet, resource, nil, req.Header)))
	req.Header.Set("Authorization", "ODPS "+cfg.AccessID+":"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return o.httpClient.Do(req)
}

// TestConnection validates endpoint + credentials + project (Java OdpsManager.testConnection).
func (o *OdpsChecker) TestConnection(ctx context.Context, cfg *OdpsConfig) error {
	resp, err := o.get(ctx, cfg, "/projects/"+url.PathEscape(cfg.Project))
	if err != nil {
		return fmt.Errorf("odps test connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var e odpsError
	_ = xml.Unmarshal(body, &e)
	if e.Code != "" {
		return fmt.Errorf("odps test connection failed: %s %s", e.Code, e.Message)
	}
	return fmt.Errorf("odps test connection failed: HTTP %d", resp.StatusCode)
}

// TableExists reports whether project.table exists; any error yields false.
func (o *OdpsChecker) TableExists(ctx context.Context, cfg *OdpsConfig, project, table string) bool {
	resp, err := o.get(ctx, cfg, "/projects/"+url.PathEscape(project)+"/tables/"+url.PathEscape(table))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
