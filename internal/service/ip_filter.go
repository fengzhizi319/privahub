package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// IPFilter mirrors Java IpFilterUtil + IpBlockConfig: user supplied endpoints
// (OSS/ODPS/HTTP feature services) whose resolved IP falls in a blocked CIDR
// are rejected to prevent SSRF. The block list is configurable
// (data.ip_block.{enable,list}); dev profiles disable it to allow MinIO on
// localhost.
type IPFilter struct {
	enabled bool
	nets    []*net.IPNet
	// resolve resolves a host to an IP (overridable in tests).
	resolve func(host string) (net.IP, error)
}

// NewIPFilter builds a filter from config (nil => Java defaults, enabled).
func NewIPFilter(cfg *config.IPBlockConfig) *IPFilter {
	c := config.IPBlockConfig{}
	if cfg != nil {
		c = *cfg
	}
	f := &IPFilter{enabled: c.Enabled(), resolve: defaultResolve}
	for _, cidr := range c.Ranges() {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if !strings.Contains(cidr, "/") {
			cidr += "/32"
		}
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			f.nets = append(f.nets, n)
		}
	}
	return f
}

// NewDisabledIPFilter returns a filter that allows everything.
func NewDisabledIPFilter() *IPFilter {
	return &IPFilter{enabled: false, resolve: defaultResolve}
}

func defaultResolve(host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no address for %s", host)
	}
	return addrs[0].IP, nil
}

// IsIPInRange reports whether ip is blocked.
func (f *IPFilter) IsIPInRange(ip net.IP) bool {
	if f == nil || !f.enabled || ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range f.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// hostOf extracts the host of a URL or host[:port] endpoint.
func hostOf(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty url")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("url %q has no host", raw)
	}
	return u.Hostname(), nil
}

// CheckURL returns FeatureTableIpNotKnown when the host cannot be resolved and
// FeatureTableIpFilter when it is blocked (Java urlIsIpInRange semantics).
func (f *IPFilter) CheckURL(raw string) error {
	if f == nil || !f.enabled {
		return nil
	}
	host, err := hostOf(raw)
	if err != nil {
		return bizWrap(errcode.FeatureTableIpNotKnown, err, raw)
	}
	ip, err := f.resolve(host)
	if err != nil {
		return bizWrap(errcode.FeatureTableIpNotKnown, err, raw)
	}
	if f.IsIPInRange(ip) {
		return bizErr(errcode.FeatureTableIpFilter, raw)
	}
	return nil
}

// detectionClient mirrors Java HttpUtils (no redirects, short connect timeout).
var detectionClient = &http.Client{
	Timeout: 5 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// Detection mirrors Java HttpUtils.detection: the URL must pass the IP filter,
// then a HEAD request must not fail at the transport level and must not
// answer 500. Returns (reachable, filterError).
func (f *IPFilter) Detection(ctx context.Context, rawURL string) (bool, error) {
	if _, err := url.ParseRequestURI(rawURL); err != nil || !strings.Contains(rawURL, "://") {
		return false, bizErr(errcode.FeatureTableIpFilter, rawURL)
	}
	if err := f.CheckURL(rawURL); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return false, nil
	}
	resp, err := detectionClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode != http.StatusInternalServerError, nil
}
