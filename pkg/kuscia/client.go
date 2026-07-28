// Package kuscia provides a lightweight HTTP client for the Kuscia API.
// It communicates with Kuscia's HTTP external API (default port 8082)
// using JSON-encoded request/response bodies matching the protobuf schema.
package kuscia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClientConfig holds connection parameters for the Kuscia API.
type ClientConfig struct {
	// Host is the Kuscia API HTTP host (e.g. "127.0.0.1").
	Host string `mapstructure:"host"`
	// Port is the Kuscia API HTTP external port (default 8082).
	Port int `mapstructure:"port"`
	// Protocol is "notls" or "tls".
	Protocol string `mapstructure:"protocol"`
	// Timeout for HTTP requests.
	Timeout time.Duration `mapstructure:"timeout"`
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
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Kuscia API client.
func NewClient(cfg *ClientConfig) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	scheme := "http"
	if cfg.Protocol == "tls" {
		scheme = "https"
	}
	return &Client{
		baseURL: fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port),
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
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

// doRequest performs an HTTP POST to the Kuscia API and decodes the response.
func (c *Client) doRequest(ctx context.Context, path string, reqBody interface{}, respBody interface{}) error {
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("kuscia: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("kuscia: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kuscia: HTTP %d from %s: %s", resp.StatusCode, path, string(respData))
	}

	if respBody != nil {
		if err := json.Unmarshal(respData, respBody); err != nil {
			return fmt.Errorf("kuscia: unmarshal response from %s: %w", path, err)
		}
	}

	return nil
}

// Ping checks connectivity to the Kuscia API.
func (c *Client) Ping(ctx context.Context) error {
	var resp struct {
		Status Status `json:"status"`
	}
	if err := c.doRequest(ctx, "/api/v1alpha1/health/query", map[string]interface{}{}, &resp); err != nil {
		return err
	}
	if !resp.Status.IsSuccess() {
		return fmt.Errorf("kuscia: health check failed: %s", resp.Status.Message)
	}
	return nil
}
