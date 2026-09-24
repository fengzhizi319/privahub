package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// IPFilter mirrors Java IpFilterUtil + IpBlockConfig: user supplied endpoints
// (OSS/ODPS/MySQL/HTTP feature services) whose IP falls in a blocked CIDR are
// rejected to prevent SSRF. The block list is configurable
// (data.ip_block.{enable,list}); dev profiles disable it to allow MinIO on
// localhost.
//
// Two layers are enforced:
//   - CheckURL: a pre-flight check of every address the host resolves to
//     (user-facing error codes, Java semantics);
//   - DialContext / HTTPClient: a net.Dialer Control hook that checks the
//     actual remote IP of every connection attempt, which defeats DNS
//     rebinding (resolve-check-connect races) and covers every resolved
//     address. All outbound probes to user supplied endpoints use it.
type IPFilter struct {
	enabled bool
	nets    []*net.IPNet
	// resolve resolves a host to all its IPs (overridable in tests).
	resolve func(host string) ([]net.IP, error)

	detectOnce   sync.Once
	detectClient *http.Client
}

// ErrIPBlocked is returned by the guarded dialer for blocked destinations.
var ErrIPBlocked = errors.New("destination address is not allowed")

// NewIPFilter builds a filter from config (nil => defaults, enabled).
func NewIPFilter(cfg *config.IPBlockConfig) *IPFilter {
	c := config.IPBlockConfig{}
	if cfg != nil {
		c = *cfg
	}
	f := &IPFilter{enabled: c.Enabled(), resolve: defaultResolve}
	ranges := append(append([]string{}, c.Ranges()...), config.AlwaysBlockedIPRanges...)
	for _, cidr := range ranges {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if n, err := config.ParseCIDROrIP(cidr); err == nil {
			f.nets = append(f.nets, n)
		}
	}
	return f
}

// NewDisabledIPFilter returns a filter that allows everything.
func NewDisabledIPFilter() *IPFilter {
	return &IPFilter{enabled: false, resolve: defaultResolve}
}

// Enabled reports whether the filter blocks anything.
func (f *IPFilter) Enabled() bool { return f != nil && f.enabled }

func defaultResolve(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
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
	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}
	return ips, nil
}

// IsIPInRange reports whether ip is blocked. IPv4-mapped IPv6 addresses are
// checked as IPv4.
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
// FeatureTableIpFilter when any of its addresses is blocked (Java
// urlIsIpInRange semantics, extended to every resolved address).
func (f *IPFilter) CheckURL(raw string) error {
	if f == nil || !f.enabled {
		return nil
	}
	host, err := hostOf(raw)
	if err != nil {
		return bizWrap(errcode.FeatureTableIpNotKnown, err, raw)
	}
	ips, err := f.resolve(host)
	if err != nil {
		return bizWrap(errcode.FeatureTableIpNotKnown, err, raw)
	}
	for _, ip := range ips {
		if f.IsIPInRange(ip) {
			return bizErr(errcode.FeatureTableIpFilter, raw)
		}
	}
	return nil
}

// control is the net.Dialer Control hook: it runs after name resolution with
// the concrete remote address of each connection attempt.
func (f *IPFilter) control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIPBlocked, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || f.IsIPInRange(ip) {
		return ErrIPBlocked
	}
	return nil
}

// Dialer returns a net.Dialer that refuses blocked remote addresses.
func (f *IPFilter) Dialer(timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	if f.Enabled() {
		d.Control = f.control
	}
	return d
}

// DialContext dials addr, refusing blocked remote addresses at connect time.
func (f *IPFilter) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return f.Dialer(5*time.Second).DialContext(ctx, network, addr)
}

// HTTPClient returns a client for user supplied endpoints: connect-time IP
// enforcement, no environment proxies (a proxy would hide the real target),
// no redirects.
func (f *IPFilter) HTTPClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           f.Dialer(5 * time.Second).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Detection mirrors Java HttpUtils.detection: the URL must pass the IP filter,
// then a HEAD request (no redirects, connect-time IP enforcement) must not
// fail at the transport level and must not answer 500. Returns (reachable,
// filterError).
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
	f.detectOnce.Do(func() { f.detectClient = f.HTTPClient(5 * time.Second) })
	resp, err := f.detectClient.Do(req)
	if err != nil {
		if errors.Is(err, ErrIPBlocked) {
			return false, bizErr(errcode.FeatureTableIpFilter, rawURL)
		}
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode != http.StatusInternalServerError, nil
}
