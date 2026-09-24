package config

import (
	"crypto/tls"
	"fmt"
	"strings"

	pcrypto "github.com/fengzhizi319/privahub/pkg/crypto"
)

// DefaultUploadMaxSize is the default body limit of upload routes (2GiB).
// Only an explicit server.upload_max_size=-1 disables the limit.
const DefaultUploadMaxSize int64 = 2 << 30

// TLS listener stacks (server.tls.stack).
const (
	TLSStackTLS  = "tls"  // standard TLS 1.2/1.3 only (default)
	TLSStackTLCP = "tlcp" // 国密 TLCP (GB/T 38636-2020, SM2 dual certificates) only
	TLSStackBoth = "both" // same port auto-detects TLCP or TLS by the record header
)

// ServerTLSConfig configures HTTPS on the main listener (server.tls.*).
//
//	server:
//	  tls:
//	    enabled: true
//	    cert_file: /etc/privahub/tls/server.crt
//	    key_file:  /etc/privahub/tls/server.key
//	    client_ca_file: ""      # optional: require client certificates (mTLS)
//	    min_version: "1.2"      # 1.2 | 1.3
//	    stack: tls              # tls | tlcp | both
//	    tlcp:                   # SM2 sign/enc certificates for tlcp|both
//	      sign_cert_file: ...
//	      sign_key_file: ...
//	      enc_cert_file: ...
//	      enc_key_file: ...
type ServerTLSConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	CertFile     string `mapstructure:"cert_file"`
	KeyFile      string `mapstructure:"key_file"`
	ClientCAFile string `mapstructure:"client_ca_file"`
	// MinVersion is "1.2" (default) or "1.3"; older versions are refused.
	MinVersion string `mapstructure:"min_version"`
	// Stack selects tls (default), tlcp or both (see TLSStack*).
	Stack string           `mapstructure:"stack"`
	TLCP  ServerTLCPConfig `mapstructure:"tlcp"`
}

// ServerTLCPConfig holds the SM2 certificates for the 国密 TLCP stack.
type ServerTLCPConfig struct {
	SignCertFile string `mapstructure:"sign_cert_file"`
	SignKeyFile  string `mapstructure:"sign_key_file"`
	EncCertFile  string `mapstructure:"enc_cert_file"`
	EncKeyFile   string `mapstructure:"enc_key_file"`
}

// NormalizedStack returns the lower-cased stack, defaulting to tls.
func (t ServerTLSConfig) NormalizedStack() string {
	s := strings.ToLower(strings.TrimSpace(t.Stack))
	if s == "" {
		return TLSStackTLS
	}
	return s
}

// TLSMinVersion maps MinVersion to a crypto/tls constant (default TLS 1.2).
func (t ServerTLSConfig) TLSMinVersion() (uint16, error) {
	switch strings.TrimSpace(strings.TrimPrefix(strings.ToLower(t.MinVersion), "tls")) {
	case "", "1.2", "12":
		return tls.VersionTLS12, nil
	case "1.3", "13":
		return tls.VersionTLS13, nil
	}
	return 0, fmt.Errorf("server.tls.min_version %q is not allowed (use 1.2 or 1.3)", t.MinVersion)
}

// Validate checks the TLS settings when enabled.
func (t ServerTLSConfig) Validate() error {
	if !t.Enabled {
		return nil
	}
	if _, err := t.TLSMinVersion(); err != nil {
		return err
	}
	stack := t.NormalizedStack()
	switch stack {
	case TLSStackTLS, TLSStackTLCP, TLSStackBoth:
	default:
		return fmt.Errorf("server.tls.stack %q is invalid (use tls, tlcp or both)", t.Stack)
	}
	if stack != TLSStackTLCP && (t.CertFile == "" || t.KeyFile == "") {
		return fmt.Errorf("server.tls.enabled=true needs server.tls.cert_file and server.tls.key_file")
	}
	if stack != TLSStackTLS && (t.TLCP.SignCertFile == "" || t.TLCP.SignKeyFile == "" ||
		t.TLCP.EncCertFile == "" || t.TLCP.EncKeyFile == "") {
		return fmt.Errorf("server.tls.stack=%s needs server.tls.tlcp.{sign_cert_file,sign_key_file,enc_cert_file,enc_key_file}", stack)
	}
	return nil
}

// MainPortSecure reports whether the main port is reached over TLS, either
// terminated by this process or by a declared TLS proxy. Used for HSTS and
// Secure cookies.
func (s ServerConfig) MainPortSecure() bool {
	return s.TLS.Enabled || s.BehindTLSProxy
}

// secureKusciaProtocols are the KusciaAPI protocols accepted in production.
var secureKusciaProtocols = map[string]bool{"tls": true, "mtls": true, "https": true}

// normalizeTransport applies defaults that viper cannot express.
func (c *Config) normalizeTransport() {
	if c.Server.UploadMaxSize == 0 {
		c.Server.UploadMaxSize = DefaultUploadMaxSize
	}
	if c.Server.UploadMaxSize < 0 {
		c.Server.UploadMaxSize = -1
	}
}

// validateTransportSecurity validates main-port TLS, KusciaAPI transport and
// the at-rest encryption keys. In production (non dev/test/local profiles):
//   - the main port must use TLS (server.tls.enabled) or sit behind a declared
//     TLS-terminating proxy (server.behind_tls_proxy=true + trusted proxies);
//   - kuscia.protocol (and every kuscia.nodes[].protocol) must be tls or mtls,
//     and kuscia.insecure_skip_verify is refused;
//   - an at-rest encryption key (crypto.keys / crypto.aes_key) is required.
func (c *Config) validateTransportSecurity() error {
	c.normalizeTransport()
	if err := c.Server.TLS.Validate(); err != nil {
		return err
	}
	if _, err := c.Crypto.Keyring(); err != nil {
		return err
	}
	if c.IsDev() {
		return nil
	}
	if !c.Server.TLS.Enabled {
		if !c.Server.BehindTLSProxy {
			return fmt.Errorf("the main port serves plain HTTP: enable server.tls (cert_file/key_file) or set " +
				"server.behind_tls_proxy=true with server.trusted_proxies when a TLS-terminating proxy fronts it" + profileHint)
		}
		if !c.hasTrustedProxies() {
			return fmt.Errorf("server.behind_tls_proxy=true requires server.trusted_proxies (the TLS proxy addresses)" + profileHint)
		}
	}
	if p := strings.ToLower(strings.TrimSpace(c.Kuscia.Protocol)); !secureKusciaProtocols[p] {
		return fmt.Errorf("kuscia.protocol=%q is not allowed in production: use tls or mtls (notls only with PRIVAHUB_PROFILE=dev|test)", c.Kuscia.Protocol)
	}
	for _, n := range c.Kuscia.Nodes {
		if p := strings.ToLower(strings.TrimSpace(n.Protocol)); p != "" && !secureKusciaProtocols[p] {
			return fmt.Errorf("kuscia.nodes[%s].protocol=%q is not allowed in production: use tls or mtls", n.DomainID, n.Protocol)
		}
	}
	if c.Kuscia.InsecureSkipVerify {
		return fmt.Errorf("kuscia.insecure_skip_verify=true is only allowed with PRIVAHUB_PROFILE=dev|test: configure kuscia.ca_file")
	}
	kr, _ := c.Crypto.Keyring()
	if kr == nil {
		return fmt.Errorf("no at-rest encryption key: set PRIVAHUB_CRYPTO_AES_KEY (e.g. `openssl rand -hex 32`) or crypto.keys + crypto.active_version" + profileHint)
	}
	return nil
}

// Keyring builds the at-rest envelope keyring from crypto.keys and the
// legacy crypto.aes_key (registered as v1 unless keys defines v1). It returns
// (nil, nil) when no key is configured (the sample key is refused outside
// dev by validateSecurity).
func (c CryptoConfig) Keyring() (*pcrypto.Keyring, error) {
	keys := map[string]string{}
	for v, k := range c.Keys {
		if strings.TrimSpace(k) != "" {
			keys[strings.ToLower(strings.TrimSpace(v))] = k
		}
	}
	if _, ok := keys["v1"]; !ok && c.AESKey != "" {
		keys["v1"] = c.AESKey
	}
	if len(keys) == 0 {
		return nil, nil
	}
	active := strings.ToLower(strings.TrimSpace(c.ActiveVersion))
	if active == "" {
		switch _, ok := keys["v1"]; {
		case len(keys) == 1:
			for v := range keys {
				active = v
			}
		case ok:
			active = "v1"
		default:
			return nil, fmt.Errorf("crypto.active_version is required when crypto.keys has several versions")
		}
	}
	kr, err := pcrypto.NewKeyring(keys, active)
	if err != nil {
		return nil, fmt.Errorf("crypto: %w", err)
	}
	return kr, nil
}

// hasTrustedProxies reports whether server.trusted_proxies is configured.
func (c *Config) hasTrustedProxies() bool {
	for _, p := range c.Server.TrustedProxies {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	return false
}
