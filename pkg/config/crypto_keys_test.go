package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Obviously fake, low-entropy test keys (32 hex chars).
var (
	testKeyV1 = strings.Repeat("ab", 16)
	testKeyV2 = strings.Repeat("cd", 16)
	// digits-only hex must survive YAML parsing as a string, not a number.
	testKeyV3 = strings.Repeat("12", 16)
)

func writeCfg(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "privahub.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_AdminPasswordFromEnv(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, "server:\n  mode: master\nauth:\n  jwt_secret: "+strongSecret+"\n")
	t.Setenv("PRIVAHUB_PROFILE", "dev")
	t.Setenv("PRIVAHUB_AUTH_ADMIN_PASSWORD", "Env-Admin-Pass-1!")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.AdminPassword != "Env-Admin-Pass-1!" {
		t.Fatalf("admin password from env not applied: %q", cfg.Auth.AdminPassword)
	}
}

// Every scalar leaf key of Config is reachable from PRIVAHUB_* even when it
// has no default and is not in the config file.
func TestLoad_EnvBindsKeysWithoutDefaults(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, "server:\n  mode: master\nauth:\n  jwt_secret: "+strongSecret+"\n")
	t.Setenv("PRIVAHUB_PROFILE", "dev")
	t.Setenv("PRIVAHUB_SERVER_NODE_ID", "env-node")
	t.Setenv("PRIVAHUB_KUSCIA_GATEWAY", "10.0.0.1:1080")
	t.Setenv("PRIVAHUB_DATABASE_MIGRATE_DIR", "/tmp/mig")
	t.Setenv("PRIVAHUB_AUTH_JWT_PREVIOUS_SECRETS", "old-one,old-two")
	t.Setenv("PRIVAHUB_AUTH_COOKIE_SECURE", "true")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.NodeID != "env-node" || cfg.Kuscia.Gateway != "10.0.0.1:1080" || cfg.Database.MigrateDir != "/tmp/mig" {
		t.Errorf("env not applied: node=%q gw=%q mig=%q", cfg.Server.NodeID, cfg.Kuscia.Gateway, cfg.Database.MigrateDir)
	}
	if len(cfg.Auth.JWTPreviousSecrets) != 2 {
		t.Errorf("slice env not applied: %v", cfg.Auth.JWTPreviousSecrets)
	}
	if cfg.Auth.CookieSecure == nil || !*cfg.Auth.CookieSecure {
		t.Errorf("*bool env not applied: %v", cfg.Auth.CookieSecure)
	}
}

func TestLoad_CryptoKeysFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys.yaml")
	if err := os.WriteFile(keys, []byte("v1: "+testKeyV1+"\nV3: "+testKeyV3+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := writeCfg(t, dir, "server:\n  mode: master\nauth:\n  jwt_secret: "+strongSecret+
		"\ncrypto:\n  keys:\n    v1: "+testKeyV2+"\n    v0: "+testKeyV2+"\n  active_version: v2\n")
	t.Setenv("PRIVAHUB_PROFILE", "dev")
	t.Setenv("PRIVAHUB_CRYPTO_KEYS_FILE", keys)
	t.Setenv("PRIVAHUB_CRYPTO_KEY_V2", testKeyV2)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	k := cfg.Crypto.Keys
	if k["v1"] != testKeyV1 || k["v2"] != testKeyV2 || k["v3"] != testKeyV3 || k["v0"] != testKeyV2 {
		t.Fatalf("merged keys wrong: %v", k)
	}
	kr, err := cfg.Crypto.Keyring()
	if err != nil || kr == nil || kr.ActiveVersion() != "v2" {
		t.Fatalf("keyring: %v %v", kr, err)
	}
	for _, w := range cfg.SecurityWarnings() {
		if strings.Contains(w, "keys_file") {
			t.Errorf("0600 keys file must not warn: %s", w)
		}
	}
}

func TestLoadExternalKeys_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(keys, []byte(`{"v1":"`+testKeyV1+`"}`), 0o400); err != nil {
		t.Fatal(err)
	}
	c := CryptoConfig{KeysFile: keys}
	if err := c.loadExternalKeys([]string{"PRIVAHUB_CRYPTO_KEY_V1=" + testKeyV2, "PRIVAHUB_CRYPTO_KEYS_FILE=" + keys, "OTHER=x"}); err != nil {
		t.Fatal(err)
	}
	if c.Keys["v1"] != testKeyV2 || len(c.Keys) != 1 {
		t.Fatalf("env must override file: %v", c.Keys)
	}
	if len(c.warnings) != 0 {
		t.Errorf("0400 must not warn: %v", c.warnings)
	}
}

func TestLoadExternalKeys_Errors(t *testing.T) {
	dir := t.TempDir()
	loose := filepath.Join(dir, "loose.yaml")
	if err := os.WriteFile(loose, []byte("v1: "+testKeyV1+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o644); err != nil {
		t.Fatal(err)
	}
	c := CryptoConfig{KeysFile: loose}
	if err := c.loadExternalKeys(nil); err != nil {
		t.Fatal(err)
	}
	if len(c.warnings) != 1 || !strings.Contains(c.warnings[0], "0644") {
		t.Errorf("loose perms must warn: %v", c.warnings)
	}

	bad := filepath.Join(dir, "bad.yaml")
	_ = os.WriteFile(bad, []byte("- not\n- a map\n"), 0o600)
	empty := filepath.Join(dir, "empty.yaml")
	_ = os.WriteFile(empty, []byte("{}"), 0o600)
	badVer := filepath.Join(dir, "badver.yaml")
	_ = os.WriteFile(badVer, []byte("\"v 1\": "+testKeyV1+"\n"), 0o600)
	for name, cc := range map[string]struct {
		file string
		env  []string
	}{
		"missing file": {file: filepath.Join(dir, "nope.yaml")},
		"not a map":    {file: bad},
		"no keys":      {file: empty},
		"bad version":  {file: badVer},
		"bad env ver":  {env: []string{"PRIVAHUB_CRYPTO_KEY_V$1=" + testKeyV1}},
		"directory":    {file: dir},
	} {
		c := CryptoConfig{KeysFile: cc.file}
		if err := c.loadExternalKeys(cc.env); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
