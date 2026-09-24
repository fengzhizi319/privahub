package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// emptyPayloadSHA256 is the hex SHA-256 of an empty body.
const emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// OssConfig holds S3-compatible object storage connection parameters
// (Java AwsOssConfig).
type OssConfig struct {
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	// Region used for SigV4; empty => "us-east-1" (what the Java AWS SDK uses
	// for non-AWS endpoints). MinIO / Aliyun OSS S3-compat accept it.
	Region string `json:"region"`
	// UseHTTPS applies only when Endpoint has no scheme.
	UseHTTPS bool `json:"use_https"`
	// VirtualHost selects virtual-hosted style (bucket.endpoint) instead of
	// path style (endpoint/bucket). MinIO needs path style.
	VirtualHost bool `json:"virtualhost"`
}

// OssService provides signed (AWS SigV4) S3-compatible bucket / object
// existence checks, mirroring Java OssServiceImpl (AWS SDK doesBucketExistV2 /
// doesObjectExist).
type OssService struct {
	httpClient *http.Client
	ipFilter   *IPFilter
	now        func() time.Time
}

// NewOssService creates a new OssService with the Java default IP block list.
func NewOssService() *OssService {
	return NewOssServiceWithFilter(NewIPFilter(nil))
}

// NewOssServiceWithFilter creates an OssService using the given SSRF filter
// (nil => allow all).
func NewOssServiceWithFilter(f *IPFilter) *OssService {
	if f == nil {
		f = NewDisabledIPFilter()
	}
	return &OssService{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			// Disable redirects to prevent SSRF via redirect chains.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		ipFilter: f,
		now:      time.Now,
	}
}

// s3ErrorBody is the S3 XML error document.
type s3ErrorBody struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func (s *OssService) buildURL(cfg *OssConfig, bucket, key string, query url.Values) (*url.URL, error) {
	ep := strings.TrimSpace(cfg.Endpoint)
	if ep == "" {
		return nil, errors.New("endpoint is required")
	}
	if !strings.Contains(ep, "://") {
		if cfg.UseHTTPS {
			ep = "https://" + ep
		} else {
			ep = "http://" + ep
		}
	}
	u, err := url.Parse(ep)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid endpoint %q", cfg.Endpoint)
	}
	base := strings.TrimRight(u.Path, "/")
	segs := []string{}
	if cfg.VirtualHost {
		u.Host = bucket + "." + u.Host
	} else {
		segs = append(segs, bucket)
	}
	if key != "" {
		segs = append(segs, strings.Split(strings.TrimLeft(key, "/"), "/")...)
	}
	raw := make([]string, len(segs))
	for i, sgm := range segs {
		raw[i] = url.PathEscape(sgm)
	}
	u.Path = base + "/" + strings.Join(segs, "/")
	u.RawPath = base + "/" + strings.Join(raw, "/")
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u, nil
}

func (s *OssService) signedDo(ctx context.Context, cfg *OssConfig, method string, u *url.URL) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Amz-Content-Sha256", emptyPayloadSHA256)
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	creds := aws.Credentials{AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.AccessKeySecret}
	signer := v4.NewSigner(func(o *v4.SignerOptions) { o.DisableURIPathEscaping = true })
	if err := signer.SignHTTP(ctx, creds, req, emptyPayloadSHA256, "s3", region, s.now().UTC()); err != nil {
		return nil, err
	}
	return s.httpClient.Do(req)
}

// CheckBucketExists verifies that a bucket exists and the credentials are
// accepted. Errors carry Java DatasourceErrorCode semantics:
//   - NoSuchBucket                         -> DATA_SOURCE_BUCKET_NOT_EXIST
//   - InvalidAccessKeyId/SignatureDoesNotMatch -> DATA_SOURCE_CREDENTIALS_INVALID
//   - PermanentRedirect                    -> DATA_SOURCE_BUCKET_NOT_MATCH_ENDPOINT
//   - HTTP 400                             -> DATA_SOURCE_ENDPOINT_API_PORT_ERROR
//   - other S3 errors                      -> DATASOURCE_UNKNOWN_EXCEPTION
//   - transport errors                     -> DATA_SOURCE_CREATE_FAIL
func (s *OssService) CheckBucketExists(ctx context.Context, cfg *OssConfig, bucketName string) error {
	if cfg == nil || cfg.Endpoint == "" || bucketName == "" {
		return bizErr(errcode.DatasourceDataSourceCreateFail, "endpoint and bucket name are required")
	}
	if err := s.ipFilter.CheckURL(cfg.Endpoint); err != nil {
		return err
	}
	// GetBucketLocation is a cheap signed call that returns an XML error body
	// (HEAD responses carry no error code).
	u, err := s.buildURL(cfg, bucketName, "", url.Values{"location": {""}})
	if err != nil {
		return bizWrap(errcode.DatasourceDataSourceCreateFail, err, "")
	}
	resp, err := s.signedDo(ctx, cfg, http.MethodGet, u)
	if err != nil {
		return bizWrap(errcode.DatasourceDataSourceCreateFail, err, "")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var e s3ErrorBody
	_ = xml.Unmarshal(body, &e)
	switch {
	case strings.EqualFold(e.Code, "NoSuchBucket") || (e.Code == "" && resp.StatusCode == http.StatusNotFound):
		return bizErr(errcode.DatasourceDataSourceBucketNotExist, "")
	case strings.EqualFold(e.Code, "PermanentRedirect") || resp.StatusCode == http.StatusMovedPermanently:
		return bizErr(errcode.DatasourceDataSourceBucketNotMatchEndpoint, "")
	case strings.EqualFold(e.Code, "InvalidAccessKeyId") || strings.EqualFold(e.Code, "SignatureDoesNotMatch"):
		return bizErr(errcode.DatasourceDataSourceCredentialsInvalid, "")
	case strings.EqualFold(e.Code, "AccessDenied"):
		// Bucket exists but the key has no permission for this sub-resource
		// (AWS SDK doesBucketExistV2 treats 403 as "exists").
		return nil
	case resp.StatusCode == http.StatusBadRequest:
		return bizErr(errcode.DatasourceDataSourceEndpointApiPortError, "")
	case resp.StatusCode == http.StatusForbidden:
		return bizErr(errcode.DatasourceDataSourceCredentialsInvalid, "")
	default:
		msg := e.Message
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return &BizError{Code: errcode.DatasourceUnknownException, Msg: msg}
	}
}

// CheckObjectExists verifies that an object exists (signed HEAD). Like Java
// doesObjectExist wrapped in OssServiceImpl, any failure yields false; the
// error is returned for logging.
func (s *OssService) CheckObjectExists(ctx context.Context, cfg *OssConfig, bucketName, objectKey string) (bool, error) {
	if cfg == nil || cfg.Endpoint == "" || bucketName == "" || objectKey == "" {
		return false, fmt.Errorf("endpoint, bucket name, and object key are required")
	}
	if err := s.ipFilter.CheckURL(cfg.Endpoint); err != nil {
		return false, err
	}
	u, err := s.buildURL(cfg, bucketName, objectKey, nil)
	if err != nil {
		return false, err
	}
	resp, err := s.signedDo(ctx, cfg, http.MethodHead, u)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
}
