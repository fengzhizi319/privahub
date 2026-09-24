package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// ConfigureTrustedProxies sets the proxies whose X-Forwarded-For / X-Real-IP
// headers gin may use for ClientIP (server.trusted_proxies: IPs / CIDRs).
// Empty means none: ClientIP is always the direct peer (RemoteAddr), so
// clients cannot spoof their address for rate limiting, audit logs or the
// inst registration host. trustedPlatform optionally names a CDN / platform
// client-IP header (server.trusted_platform, e.g. "CF-Connecting-IP"; gin
// trusts it unconditionally, so only set it when the platform strips it from
// client requests).
func ConfigureTrustedProxies(e *gin.Engine, proxies []string, trustedPlatform string) error {
	cleaned := make([]string, 0, len(proxies))
	for _, p := range proxies {
		if p = strings.TrimSpace(p); p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		if err := e.SetTrustedProxies(nil); err != nil {
			return err
		}
	} else if err := e.SetTrustedProxies(cleaned); err != nil {
		return fmt.Errorf("server.trusted_proxies: %w", err)
	}
	e.TrustedPlatform = strings.TrimSpace(trustedPlatform)
	return nil
}
