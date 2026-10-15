// Package kuscia provides a lightweight HTTP client for the Kuscia API.
//
// Kuscia is the Kubernetes-based orchestration engine for privacy-preserving
// computing jobs. This client communicates with KusciaAPI's HTTP external
// port (default 8082) using JSON bodies that mirror the protobuf schema in
// kuscia/proto/api/v1alpha1/kusciaapi (proto field names, snake_case).
//
// Security (mirrors Java KusciaGrpcConfig / GrpcKusciaApiChannelFactory):
//   - Protocol "notls": plain HTTP.
//   - Protocol "tls":   HTTPS, server verified with CAFile (if set).
//   - Protocol "mtls":  HTTPS + client certificate (CertFile/KeyFile) + CA.
//   - Token: sent as the `Token` header on every request (KusciaAPI
//     token auth, see kuscia pkg/web/interceptor). Either the literal token or
//     a path to a file containing it.
//
// Any KusciaAPI response whose status.code != 0 is returned as *APIError so
// callers can inspect the Kuscia error code (e.g. 11404 route not exists).
//
// The client is safe for concurrent use.
package kuscia

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Kuscia error codes (kuscia/proto/api/v1alpha1/errorcode/error_code.proto).
const (
	ErrCodeDomainNotExists      int32 = 11305
	ErrCodeDomainRouteNotExists int32 = 11404
	ErrCodeDomainDataNotExists  int32 = 11506
)

// TokenHeader is the KusciaAPI token auth header.
const TokenHeader = "Token"

// ClientConfig holds connection parameters for the Kuscia API.
type ClientConfig struct {
	// Host is the Kuscia API HTTP host (e.g. "127.0.0.1").
	Host string `mapstructure:"host" json:"host"`
	// Port is the Kuscia API HTTP external port (default 8082).
	Port int `mapstructure:"port" json:"port"`
	// Protocol is "notls", "tls" or "mtls".
	Protocol string `mapstructure:"protocol" json:"protocol"`
	// Timeout for HTTP requests.
	Timeout time.Duration `mapstructure:"timeout" json:"timeout,omitempty"`
	// CertFile / KeyFile: client certificate for mTLS (PEM).
	CertFile string `mapstructure:"cert_file" json:"cert_file,omitempty"`
	KeyFile  string `mapstructure:"key_file" json:"key_file,omitempty"`
	// CAFile: CA used to verify the KusciaAPI server certificate (PEM).
	CAFile string `mapstructure:"ca_file" json:"ca_file,omitempty"`
	// Token: KusciaAPI token or path to a token file.
	Token string `mapstructure:"token" json:"token,omitempty"`
	// ServerName overrides the TLS SNI / verification name.
	ServerName string `mapstructure:"server_name" json:"server_name,omitempty"`
	// InsecureSkipVerify disables server cert verification (dev only; used
	// when protocol is tls/mtls and no CAFile is configured).
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify" json:"insecure_skip_verify,omitempty"`
	// HTTPProxy selects the proxy for KusciaAPI requests (kuscia.http_proxy).
	// Empty (default) connects directly: KusciaAPI lives on loopback / the
	// private network, and a developer's HTTP(S)_PROXY would otherwise answer
	// for unresolvable Kuscia hosts (e.g. "HTTP 502" for a missing tee lite).
	// "env" uses HTTP(S)_PROXY / NO_PROXY; any other value is a proxy URL.
	HTTPProxy string `mapstructure:"http_proxy" json:"http_proxy,omitempty"`
}

// HTTPProxyFromEnv is the ClientConfig.HTTPProxy value that honours the
// HTTP_PROXY / HTTPS_PROXY / NO_PROXY environment variables.
const HTTPProxyFromEnv = "env"

// proxyFunc returns the http.Transport.Proxy for ClientConfig.HTTPProxy
// (nil = direct connection).
func proxyFunc(v string) (func(*http.Request) (*url.URL, error), error) {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "none", "direct":
		return nil, nil
	case HTTPProxyFromEnv, "environment":
		return http.ProxyFromEnvironment, nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("kuscia: invalid http_proxy %q (use \"\", %q or a proxy URL)", v, HTTPProxyFromEnv)
	}
	return http.ProxyURL(u), nil
}

// DefaultClientConfig returns a default configuration for local development.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Host:     "127.0.0.1",
		Port:     8082,
		Protocol: "notls",
		Timeout:  30 * time.Second,
	}
}

// Client is a lightweight HTTP client for the Kuscia API.
type Client struct {
	baseURL    string       // e.g. "http://127.0.0.1:8082"
	httpClient *http.Client // shared HTTP client with connection pooling
	token      string       // KusciaAPI Token header value
	initErr    error        // TLS/token load error; returned by every request
	cfg        ClientConfig
}

// NewClient creates a new Kuscia API client. TLS material problems do not
// panic: the error is returned by every subsequent request (see Err()).
func NewClient(cfg *ClientConfig) *Client {
	c, err := NewClientE(cfg)
	if err != nil {
		c.initErr = err
	}
	return c
}

// NewClientE creates a new Kuscia API client and reports TLS/token loading errors.
func NewClientE(cfg *ClientConfig) (*Client, error) {
	if cfg == nil {
		cfg = DefaultClientConfig()
	}
	cp := *cfg
	if cp.Timeout == 0 {
		cp.Timeout = 30 * time.Second
	}
	proto := strings.ToLower(strings.TrimSpace(cp.Protocol))
	scheme := "http"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	var initErr error
	proxy, perr := proxyFunc(cp.HTTPProxy)
	if perr != nil {
		initErr = perr
	}
	transport.Proxy = proxy
	if proto == "tls" || proto == "mtls" || proto == "https" {
		scheme = "https"
		tlsCfg, err := buildTLSConfig(&cp, proto == "mtls")
		if err != nil {
			if initErr == nil {
				initErr = err
			}
		} else {
			transport.TLSClientConfig = tlsCfg
		}
	}
	token, err := loadToken(cp.Token)
	if err != nil && initErr == nil {
		initErr = err
	}
	host := cp.Host
	if strings.Contains(host, "://") {
		// tolerate "http://host" style
		host = host[strings.Index(host, "://")+3:]
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, host)
	if cp.Port > 0 && !strings.Contains(host, ":") {
		baseURL = fmt.Sprintf("%s://%s:%d", scheme, host, cp.Port)
	}
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: cp.Timeout, Transport: transport},
		token:      token,
		initErr:    initErr,
		cfg:        cp,
	}
	return c, initErr
}

// Err returns the client initialization error (TLS/token), if any.
func (c *Client) Err() error { return c.initErr }

// BaseURL returns the endpoint URL of this client.
func (c *Client) BaseURL() string { return c.baseURL }

// Config returns a copy of the configuration used to build this client.
func (c *Client) Config() ClientConfig { return c.cfg }

func buildTLSConfig(cfg *ClientConfig, requireClientCert bool) (*tls.Config, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.ServerName}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("kuscia: read ca file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("kuscia: ca file %s contains no valid certificate", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	} else if cfg.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // explicit dev opt-in
	}
	if cfg.CertFile != "" || cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("kuscia: load client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	} else if requireClientCert {
		return nil, errors.New("kuscia: mtls requires cert_file and key_file")
	}
	return tlsCfg, nil
}

// loadToken returns the literal token or the content of the token file.
func loadToken(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if st, err := os.Stat(v); err == nil && !st.IsDir() {
		b, err := os.ReadFile(v)
		if err != nil {
			return "", fmt.Errorf("kuscia: read token file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return v, nil
}

// Status represents the Kuscia API response status.
type Status struct {
	Code    int32  `json:"code"`
	Message string `json:"message"`
}

// IsSuccess returns true if the status code indicates success.
func (s *Status) IsSuccess() bool {
	return s.Code == 0
}

// APIError is returned when KusciaAPI answers with status.code != 0 (or a
// non-200 HTTP status).
type APIError struct {
	Path       string
	HTTPStatus int
	Code       int32
	Message    string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("kuscia: %s failed: [%d] %s", e.Path, e.Code, e.Message)
	}
	return fmt.Sprintf("kuscia: HTTP %d from %s: %s", e.HTTPStatus, e.Path, e.Message)
}

// ErrorCode extracts the Kuscia status code from err (0 if not an APIError).
func ErrorCode(err error) int32 {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// IsNotFound reports whether err is a Kuscia "not exists" error.
func IsNotFound(err error) bool {
	switch ErrorCode(err) {
	case ErrCodeDomainNotExists, ErrCodeDomainRouteNotExists, ErrCodeDomainDataNotExists:
		return true
	}
	return false
}

type statusEnvelope struct {
	Status *Status `json:"status"`
}

// doRequest performs an HTTP POST to the Kuscia API and decodes the JSON
// response. A non-zero status.code is returned as *APIError.
func (c *Client) doRequest(ctx context.Context, path string, reqBody interface{}, respBody interface{}) error {
	if c.initErr != nil {
		return c.initErr
	}
	url := c.baseURL + path

	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("kuscia: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyReader)
	if err != nil {
		return fmt.Errorf("kuscia: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set(TokenHeader, c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("kuscia: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	// Limit response body to 10 MB.
	const maxResponseSize = 10 << 20
	respData, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("kuscia: read response: %w", err)
	}

	var env statusEnvelope
	_ = json.Unmarshal(respData, &env)

	if resp.StatusCode != http.StatusOK {
		ae := &APIError{Path: path, HTTPStatus: resp.StatusCode, Message: string(respData)}
		if env.Status != nil && env.Status.Code != 0 {
			ae.Code, ae.Message = env.Status.Code, env.Status.Message
		}
		return ae
	}

	if respBody != nil {
		if err := json.Unmarshal(respData, respBody); err != nil {
			return fmt.Errorf("kuscia: unmarshal response from %s: %w", path, err)
		}
	}
	if env.Status != nil && env.Status.Code != 0 {
		return &APIError{Path: path, HTTPStatus: resp.StatusCode, Code: env.Status.Code, Message: env.Status.Message}
	}
	return nil
}

// Ping checks connectivity to the Kuscia API.
func (c *Client) Ping(ctx context.Context) error {
	var resp struct {
		Status Status `json:"status"`
	}
	if err := c.doRequest(ctx, "/healthZ", map[string]interface{}{}, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: health check failed: %s", resp.Status.Message)
	}
	return nil
}
