package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateNodeSecurity validates the node-to-node / proxy settings:
//   - server.trusted_proxies entries must be IPs or CIDRs;
//   - in production, server.inner_auth (token, allowed_cidrs or mTLS) is
//     required even when the inner port binds to loopback: every co-located
//     process could otherwise reach it and forge kuscia-origin-source;
//   - in production, an EDGE platform forwards user write requests to the
//     center through kuscia.gateway: it must use https:// unless the gateway
//     is a loopback address (the co-located Kuscia Envoy).
func (c *Config) validateNodeSecurity() error {
	for _, p := range c.Server.TrustedProxies {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if _, err := ParseCIDROrIP(p); err != nil {
			return fmt.Errorf("server.trusted_proxies: %w", err)
		}
	}
	if c.IsDev() {
		return nil
	}
	a := c.Server.InnerAuth
	if token, _ := a.ResolvedToken(); token == "" && len(a.AllowedCIDRs) == 0 && !a.MTLSEnabled() {
		return fmt.Errorf("server.inner_auth is required in production even on a loopback inner port " +
			"(any local process could otherwise forge kuscia-origin-source): set server.inner_auth.token / token_file " +
			"(e.g. PRIVAHUB_SERVER_INNER_AUTH_TOKEN=$(openssl rand -hex 32)), allowed_cidrs or tls" + profileHint)
	}
	if !isEdgeMode(c.Server.Mode) {
		return nil
	}
	gw := strings.TrimSpace(c.Kuscia.Gateway)
	if gw == "" || EdgeGatewaySecure(gw) {
		return nil
	}
	return fmt.Errorf("kuscia.gateway=%q: an EDGE platform forwards user requests to the center through the gateway; "+
		"use an https:// gateway address (plain http is only allowed for a loopback gateway)"+profileHint, gw)
}

func isEdgeMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "edge", "lite":
		return true
	}
	return false
}

// EdgeGatewaySecure reports whether gw (host:port or URL) is an https URL or
// a loopback address.
func EdgeGatewaySecure(gw string) bool {
	gw = strings.TrimSpace(gw)
	if strings.HasPrefix(strings.ToLower(gw), "https://") {
		return true
	}
	raw := gw
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "http") {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
