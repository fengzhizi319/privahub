package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// CryptoKeyEnvPrefix is the per-version at-rest key environment variable
// prefix: PRIVAHUB_CRYPTO_KEY_V2=<hex> registers key version "v2". viper
// cannot map an env var onto one entry of the crypto.keys map, so these are
// read directly from the process environment.
const CryptoKeyEnvPrefix = "PRIVAHUB_CRYPTO_KEY_"

var keyVersionRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

// loadExternalKeys merges crypto.keys_file and PRIVAHUB_CRYPTO_KEY_<VERSION>
// (from environ, "K=V" entries) into c.Keys. Precedence, lowest first:
// crypto.keys in the config file, crypto.keys_file, per-version env vars.
func (c *CryptoConfig) loadExternalKeys(environ []string) error {
	merged := map[string]string{}
	for v, k := range c.Keys {
		merged[strings.ToLower(strings.TrimSpace(v))] = k
	}
	if path := strings.TrimSpace(c.KeysFile); path != "" {
		fileKeys, warn, err := readKeysFile(path)
		if err != nil {
			return err
		}
		if warn != "" {
			c.warnings = append(c.warnings, warn)
		}
		for v, k := range fileKeys {
			merged[v] = k
		}
	}
	for _, kv := range environ {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, CryptoKeyEnvPrefix) {
			continue
		}
		ver := strings.ToLower(strings.TrimPrefix(name, CryptoKeyEnvPrefix))
		if !keyVersionRe.MatchString(ver) {
			return fmt.Errorf("%s: invalid key version %q (use letters, digits, . _ -)", name, ver)
		}
		if strings.TrimSpace(val) == "" {
			continue
		}
		merged[ver] = strings.TrimSpace(val)
	}
	if len(merged) > 0 {
		c.Keys = merged
	}
	return nil
}

// readKeysFile parses a YAML/JSON version->key map. It returns a warning when
// the file is readable by group/other (expected 0600 or 0400).
func readKeysFile(path string) (map[string]string, string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("crypto.keys_file: %w", err)
	}
	if !st.Mode().IsRegular() {
		return nil, "", fmt.Errorf("crypto.keys_file %s: not a regular file", path)
	}
	var warn string
	if perm := st.Mode().Perm(); perm&0o077 != 0 {
		warn = fmt.Sprintf("crypto.keys_file %s has permissions %04o: restrict it to 0600 or 0400", path, perm)
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304: operator-configured key file path (crypto.keys_file), never request input
	if err != nil {
		return nil, "", fmt.Errorf("crypto.keys_file: %w", err)
	}
	var raw map[string]string
	if err := yaml.Unmarshal(b, &raw); err != nil { // JSON is valid YAML
		return nil, "", fmt.Errorf("crypto.keys_file %s: expected a map of version to key: %w", path, err)
	}
	out := make(map[string]string, len(raw))
	for v, k := range raw {
		ver := strings.ToLower(strings.TrimSpace(v))
		if !keyVersionRe.MatchString(ver) {
			return nil, "", fmt.Errorf("crypto.keys_file %s: invalid key version %q", path, v)
		}
		if k = strings.TrimSpace(k); k != "" {
			out[ver] = k
		}
	}
	if len(out) == 0 {
		return nil, "", fmt.Errorf("crypto.keys_file %s: no keys", path)
	}
	return out, warn, nil
}
