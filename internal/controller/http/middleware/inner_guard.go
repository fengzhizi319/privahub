package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// peerIP returns the direct peer address of the request (RemoteAddr only;
// X-Forwarded-For style headers are never trusted here).
func peerIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}

func parseNets(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if n, err := config.ParseCIDROrIP(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func ipAllowed(ip net.IP, nets []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func tokenEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// InnerGuardConfig configures InnerGuardWith (server.inner_auth).
type InnerGuardConfig struct {
	AllowedCIDRs []string
	Token        string
	// RequireClientCert is set when the inner listener uses mTLS: the node
	// identity is then derived from the verified client certificate.
	RequireClientCert bool
	// GatewayNames are client-certificate names (CN / DNS SAN) of Kuscia
	// gateways that relay other nodes' requests and may assert any
	// kuscia-origin-source.
	GatewayNames []string
}

// InnerGuard protects the inner port (server.inner_auth): every configured
// mechanism is enforced — the direct peer must be in AllowedCIDRs, and the
// X-Privahub-Inner-Token header must equal the shared token. (mTLS is
// enforced by the listener itself.) With nothing configured it is a no-op;
// config validation then requires a dev profile and a loopback bind.
func InnerGuard(allowedCIDRs []string, token string) gin.HandlerFunc {
	return InnerGuardWith(InnerGuardConfig{AllowedCIDRs: allowedCIDRs, Token: token})
}

// InnerGuardWith is InnerGuard plus the mTLS node identity check: with
// RequireClientCert, a request must carry a verified client certificate and,
// unless the certificate names a trusted relay gateway (GatewayNames), its CN
// or a DNS SAN must equal kuscia-origin-source — a node cannot claim another
// node's identity.
func InnerGuardWith(cfg InnerGuardConfig) gin.HandlerFunc {
	nets := parseNets(cfg.AllowedCIDRs)
	gateways := map[string]bool{}
	for _, g := range cfg.GatewayNames {
		if g = strings.ToLower(strings.TrimSpace(g)); g != "" {
			gateways[g] = true
		}
	}
	deny := func(c *gin.Context, status int, msg string) {
		c.AbortWithStatusJSON(status, response.Body{Status: &response.Status{
			Code: errcode.AuthFailed.Code, Msg: msg,
		}})
	}
	return func(c *gin.Context) {
		if len(nets) > 0 && !ipAllowed(peerIP(c.Request), nets) {
			deny(c, http.StatusForbidden, "inner port: source address not allowed")
			return
		}
		if cfg.Token != "" && !tokenEqual(c.GetHeader(innerauth.TokenHeader), cfg.Token) {
			deny(c, http.StatusUnauthorized, "inner port: invalid or missing "+innerauth.TokenHeader)
			return
		}
		if cfg.RequireClientCert {
			names := clientCertNames(c.Request)
			if len(names) == 0 {
				deny(c, http.StatusUnauthorized, "inner port: client certificate required")
				return
			}
			if !anyName(names, gateways) {
				origin := strings.ToLower(strings.TrimSpace(c.GetHeader(KusciaOriginSourceHeader)))
				if origin == "" || !anyName(names, map[string]bool{origin: true}) {
					deny(c, http.StatusForbidden, "inner port: kuscia-origin-source does not match the client certificate")
					return
				}
			}
		}
		c.Next()
	}
}

// clientCertNames returns the lower-cased CN and DNS SANs of the verified
// client leaf certificate (none without TLS / client certificate).
func clientCertNames(r *http.Request) []string {
	if r == nil || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil
	}
	leaf := r.TLS.PeerCertificates[0]
	var out []string
	if cn := strings.TrimSpace(leaf.Subject.CommonName); cn != "" {
		out = append(out, strings.ToLower(cn))
	}
	for _, d := range leaf.DNSNames {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, strings.ToLower(d))
		}
	}
	return out
}

func anyName(names []string, set map[string]bool) bool {
	for _, n := range names {
		if set[n] {
			return true
		}
	}
	return false
}

// MetricsGuardConfig configures MetricsGuardWith.
type MetricsGuardConfig struct {
	AllowedCIDRs []string
	Token        string
	// BehindProxy is set when a reverse proxy fronts the main port
	// (server.trusted_proxies / trusted_platform / behind_tls_proxy): every
	// proxied request then arrives from the proxy's (often loopback) address,
	// so loopback peers are no longer trusted and the token (or an allowlisted
	// client address) is required.
	BehindProxy bool
}

// MetricsGuard restricts GET /metrics (observability.metrics_access =
// restricted): loopback peers, peers in allowedCIDRs, or requests carrying
// "Authorization: Bearer <token>" are served; others get 403.
func MetricsGuard(allowedCIDRs []string, token string) gin.HandlerFunc {
	return MetricsGuardWith(MetricsGuardConfig{AllowedCIDRs: allowedCIDRs, Token: token})
}

// MetricsGuardWith is MetricsGuard with proxy awareness. Addresses are taken
// from c.ClientIP(), which only honours forwarding headers of
// server.trusted_proxies (ConfigureTrustedProxies).
func MetricsGuardWith(cfg MetricsGuardConfig) gin.HandlerFunc {
	nets := parseNets(cfg.AllowedCIDRs)
	return func(c *gin.Context) {
		ip := net.ParseIP(c.ClientIP())
		if !cfg.BehindProxy && ip != nil && ip.IsLoopback() && peerIP(c.Request).IsLoopback() {
			c.Next()
			return
		}
		if ipAllowed(ip, nets) {
			c.Next()
			return
		}
		if cfg.Token != "" {
			if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") &&
				tokenEqual(strings.TrimPrefix(auth, "Bearer "), cfg.Token) {
				c.Next()
				return
			}
		}
		c.AbortWithStatus(http.StatusForbidden)
	}
}
