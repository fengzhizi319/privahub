package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// OssConfig holds S3-compatible object storage connection parameters.
type OssConfig struct {
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Region          string `json:"region"`
	UseHTTPS        bool   `json:"use_https"`
}

// OssService provides S3-compatible object storage connectivity checks.
type OssService struct {
	httpClient *http.Client
}

// NewOssService creates a new OssService.
func NewOssService() *OssService {
	return &OssService{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			// Disable redirects to prevent SSRF via redirect chains
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// validateEndpoint checks that the endpoint is not a loopback, link-local,
// metadata, or private network address to prevent SSRF attacks.
func validateEndpoint(endpoint string) error {
	host := endpoint
	if h, _, err := net.SplitHostPort(endpoint); err == nil {
		host = h
	}

	// Block obvious dangerous hosts
	lower := strings.ToLower(host)
	if lower == "localhost" || lower == "metadata.google.internal" ||
		strings.HasSuffix(lower, ".internal") ||
		lower == "169.254.169.254" {
		return fmt.Errorf("endpoint '%s' is not allowed", endpoint)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Not an IP literal; allow DNS names (they are configured by admins)
		return nil
	}

	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() {
		return fmt.Errorf("endpoint '%s' resolves to a restricted network range", endpoint)
	}

	return nil
}

// CheckBucketExists verifies that a bucket exists and is accessible.
func (s *OssService) CheckBucketExists(ctx context.Context, cfg *OssConfig, bucketName string) error {
	if cfg.Endpoint == "" || bucketName == "" {
		return fmt.Errorf("endpoint and bucket name are required")
	}
	if err := validateEndpoint(cfg.Endpoint); err != nil {
		return err
	}

	scheme := "http"
	if cfg.UseHTTPS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s.%s", scheme, bucketName, cfg.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bucket connectivity check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("bucket '%s' does not exist", bucketName)
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("access denied to bucket '%s'", bucketName)
	}
	// 200 or 301/307 (redirect) means bucket exists
	return nil
}

// CheckObjectExists verifies that a specific object exists in a bucket.
func (s *OssService) CheckObjectExists(ctx context.Context, cfg *OssConfig, bucketName, objectKey string) (bool, error) {
	if cfg.Endpoint == "" || bucketName == "" || objectKey == "" {
		return false, fmt.Errorf("endpoint, bucket name, and object key are required")
	}
	if err := validateEndpoint(cfg.Endpoint); err != nil {
		return false, err
	}

	scheme := "http"
	if cfg.UseHTTPS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s.%s/%s", scheme, bucketName, cfg.Endpoint, objectKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("object connectivity check failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	case http.StatusForbidden:
		return false, fmt.Errorf("access denied to object '%s' in bucket '%s'", objectKey, bucketName)
	default:
		return false, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
}
