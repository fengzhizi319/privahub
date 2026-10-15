package config

import (
	"fmt"
	"strconv"
	"strings"
)

// KusciaNodeEnvPrefix is the per-node override prefix for kuscia.nodes:
//
//	PRIVAHUB_KUSCIA_NODES_<DOMAIN>_HOST
//	PRIVAHUB_KUSCIA_NODES_<DOMAIN>_PORT       (KusciaAPI gRPC port)
//	PRIVAHUB_KUSCIA_NODES_<DOMAIN>_HTTP_PORT  (KusciaAPI HTTP port)
//	PRIVAHUB_KUSCIA_NODES_<DOMAIN>_PROTOCOL
//
// <DOMAIN> is the node's domain_id upper-cased with every character outside
// [A-Z0-9] replaced by "_" (kuscia-system -> KUSCIA_SYSTEM). viper cannot map
// an env var onto one element of a slice of structs, so these are read
// directly from the process environment (like PRIVAHUB_CRYPTO_KEY_<VERSION>).
// They only override nodes declared in the config file; unknown domains are
// ignored.
const KusciaNodeEnvPrefix = "PRIVAHUB_KUSCIA_NODES_"

// KusciaNodeEnvKey returns the env var name segment for a node domain id.
func KusciaNodeEnvKey(domainID string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(domainID) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// applyNodeEnvOverrides applies PRIVAHUB_KUSCIA_NODES_<DOMAIN>_* (environ is
// a list of "K=V" entries) to c.Nodes.
func (c *KusciaConfig) applyNodeEnvOverrides(environ []string) error {
	if len(c.Nodes) == 0 {
		return nil
	}
	env := map[string]string{}
	for _, kv := range environ {
		name, val, ok := strings.Cut(kv, "=")
		if ok && strings.HasPrefix(name, KusciaNodeEnvPrefix) {
			env[name] = strings.TrimSpace(val)
		}
	}
	if len(env) == 0 {
		return nil
	}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		if n.DomainID == "" {
			continue
		}
		p := KusciaNodeEnvPrefix + KusciaNodeEnvKey(n.DomainID) + "_"
		if v := env[p+"HOST"]; v != "" {
			n.Host = v
		}
		if v := env[p+"PROTOCOL"]; v != "" {
			n.Protocol = v
		}
		for _, f := range []struct {
			key string
			dst *int
		}{{"PORT", &n.Port}, {"HTTP_PORT", &n.HTTPPort}} {
			v := env[p+f.key]
			if v == "" {
				continue
			}
			port, err := strconv.Atoi(v)
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("%s%s=%q: must be a TCP port (1-65535)", p, f.key, v)
			}
			*f.dst = port
		}
	}
	return nil
}
