// Package tlsutil builds the listeners of the Privahub main port:
// standard TLS (1.2+, AEAD-only suites, optional client CA) and optionally
// the 国密 TLCP stack (GB/T 38636-2020, SM2 dual certificates, SM4-GCM
// suites) via gitee.com/Trisia/gotlcp, alone or auto-detected on the same
// port ("both"), mirroring PrivShield pkg/tlsutil.
package tlsutil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"

	"gitee.com/Trisia/gotlcp/pa"
	"gitee.com/Trisia/gotlcp/tlcp"
	"github.com/emmansun/gmsm/smx509"

	"github.com/fengzhizi319/privahub/pkg/config"
)

// secureTLS12Suites are the TLS 1.2 suites allowed (ECDHE + AEAD only).
// TLS 1.3 suites are not configurable in Go and are all AEAD.
var secureTLS12Suites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
}

// HardenedConfig returns a base tls.Config with the minimum version and
// cipher-suite policy applied.
func HardenedConfig(minVersion uint16) *tls.Config {
	if minVersion < tls.VersionTLS12 {
		minVersion = tls.VersionTLS12
	}
	return &tls.Config{
		MinVersion:       minVersion,
		CipherSuites:     secureTLS12Suites,
		CurvePreferences: []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256, tls.CurveP384},
		NextProtos:       []string{"h2", "http/1.1"},
	}
}

// ServerTLSConfig builds the standard TLS side of server.tls.
func ServerTLSConfig(c config.ServerTLSConfig) (*tls.Config, error) {
	minV, err := c.TLSMinVersion()
	if err != nil {
		return nil, err
	}
	cfg := HardenedConfig(minV)
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("server.tls: load cert/key: %w", err)
	}
	cfg.Certificates = []tls.Certificate{cert}
	if c.ClientCAFile != "" {
		pool, err := loadPool(c.ClientCAFile)
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}

// TLCPConfig builds the 国密 TLCP side of server.tls (stack tlcp|both).
func TLCPConfig(c config.ServerTLSConfig) (*tlcp.Config, error) {
	sign, err := tlcp.LoadX509KeyPair(c.TLCP.SignCertFile, c.TLCP.SignKeyFile)
	if err != nil {
		return nil, fmt.Errorf("server.tls.tlcp: load sign cert/key: %w", err)
	}
	enc, err := tlcp.LoadX509KeyPair(c.TLCP.EncCertFile, c.TLCP.EncKeyFile)
	if err != nil {
		return nil, fmt.Errorf("server.tls.tlcp: load enc cert/key: %w", err)
	}
	cfg := &tlcp.Config{
		// Order is the gotlcp slot convention: [sign, enc].
		Certificates: []tlcp.Certificate{sign, enc},
		// SM4-GCM only (no CBC / SM1 legacy suites).
		CipherSuites: []uint16{tlcp.ECDHE_SM4_GCM_SM3, tlcp.ECC_SM4_GCM_SM3},
	}
	if c.ClientCAFile != "" {
		pem, err := os.ReadFile(c.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("server.tls.client_ca_file: %w", err)
		}
		pool := smx509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("server.tls.client_ca_file %s: no certificates", c.ClientCAFile)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tlcp.RequireAndVerifyClientCert
	}
	return cfg, nil
}

// Listen opens the main-port listener for server.tls. With TLS disabled it
// returns a plain TCP listener.
func Listen(network, addr string, c config.ServerTLSConfig) (net.Listener, error) {
	if !c.Enabled {
		return listenTCP(network, addr)
	}
	stack := c.NormalizedStack()
	var (
		tlsCfg  *tls.Config
		tlcpCfg *tlcp.Config
		err     error
	)
	if stack == config.TLSStackTLS || stack == config.TLSStackBoth {
		if tlsCfg, err = ServerTLSConfig(c); err != nil {
			return nil, err
		}
	}
	if stack == config.TLSStackTLCP || stack == config.TLSStackBoth {
		if tlcpCfg, err = TLCPConfig(c); err != nil {
			return nil, err
		}
	}
	inner, err := listenTCP(network, addr)
	if err != nil {
		return nil, err
	}
	switch stack {
	case config.TLSStackTLS:
		return tls.NewListener(inner, tlsCfg), nil
	case config.TLSStackTLCP:
		return tlcp.NewListener(inner, tlcpCfg), nil
	case config.TLSStackBoth:
		// HTTP/2 is not negotiated on the auto-detecting listener.
		tlsCfg.NextProtos = []string{"http/1.1"}
		if l := pa.NewListener(inner, tlcpCfg, tlsCfg); l != nil {
			return l, nil
		}
		_ = inner.Close()
		return nil, errors.New("server.tls: protocol-switch listener rejected config")
	}
	_ = inner.Close()
	return nil, fmt.Errorf("server.tls.stack %q is invalid", c.Stack)
}

// MutualTLSConfig returns a hardened config that requires client certificates
// signed by caFile (used by the inner port).
func MutualTLSConfig(caFile string) (*tls.Config, error) {
	pool, err := loadPool(caFile)
	if err != nil {
		return nil, err
	}
	cfg := HardenedConfig(tls.VersionTLS12)
	cfg.ClientCAs = pool
	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	return cfg, nil
}

func loadPool(file string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read CA %s: %w", file, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates in CA %s", file)
	}
	return pool, nil
}

// listenTCP opens the raw listener via net.ListenConfig. The listener outlives
// any request context, so a background context is correct here.
func listenTCP(network, addr string) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(context.Background(), network, addr)
}
