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

// InnerGuard protects the inner port (server.inner_auth): every configured
// mechanism is enforced — the direct peer must be in AllowedCIDRs, and the
// X-Privahub-Inner-Token header must equal the shared token. (mTLS is
// enforced by the listener itself.) With nothing configured it is a no-op;
// config validation then requires the port to be bound to loopback.
func InnerGuard(allowedCIDRs []string, token string) gin.HandlerFunc {
	nets := parseNets(allowedCIDRs)
	return func(c *gin.Context) {
		if len(nets) > 0 && !ipAllowed(peerIP(c.Request), nets) {
			c.AbortWithStatusJSON(http.StatusForbidden, response.Body{Status: &response.Status{
				Code: errcode.AuthFailed.Code, Msg: "inner port: source address not allowed",
			}})
			return
		}
		if token != "" && !tokenEqual(c.GetHeader(innerauth.TokenHeader), token) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Body{Status: &response.Status{
				Code: errcode.AuthFailed.Code, Msg: "inner port: invalid or missing " + innerauth.TokenHeader,
			}})
			return
		}
		c.Next()
	}
}

// MetricsGuard restricts GET /metrics (observability.metrics_access =
// restricted): loopback peers, peers in allowedCIDRs, or requests carrying
// "Authorization: Bearer <token>" are served; others get 403.
func MetricsGuard(allowedCIDRs []string, token string) gin.HandlerFunc {
	nets := parseNets(allowedCIDRs)
	return func(c *gin.Context) {
		ip := peerIP(c.Request)
		if ip != nil && ip.IsLoopback() || ipAllowed(ip, nets) {
			c.Next()
			return
		}
		if token != "" {
			if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") &&
				tokenEqual(strings.TrimPrefix(auth, "Bearer "), token) {
				c.Next()
				return
			}
		}
		c.AbortWithStatus(http.StatusForbidden)
	}
}
