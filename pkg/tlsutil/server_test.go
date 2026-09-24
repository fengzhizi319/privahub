package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitee.com/Trisia/gotlcp/tlcp"
	"github.com/emmansun/gmsm/sm2"
	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/pkg/config"
)

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600))
}

func selfSigned(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	certFile, keyFile = filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	writePEM(t, certFile, "CERTIFICATE", der)
	writePEM(t, keyFile, "PRIVATE KEY", kb)
	return
}

func sm2Pair(t *testing.T, dir, name string, usage smx509.KeyUsage) (string, string) {
	t.Helper()
	key, err := sm2.GenerateKey(rand.Reader)
	require.NoError(t, err)
	tmpl := &smx509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: usage, DNSNames: []string{"localhost"},
	}
	der, err := smx509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	kb, err := smx509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	c, k := filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
	writePEM(t, c, "CERTIFICATE", der)
	writePEM(t, k, "PRIVATE KEY", kb)
	return c, k
}

func serveEcho(t *testing.T, l net.Listener) {
	t.Helper()
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 4)
				if _, err := io.ReadFull(c, buf); err == nil {
					_, _ = c.Write(buf)
				}
			}()
		}
	}()
}

func TestListenTLSPolicy(t *testing.T) {
	dir := t.TempDir()
	cert, key := selfSigned(t, dir)
	l, err := Listen("tcp", "127.0.0.1:0", config.ServerTLSConfig{Enabled: true, CertFile: cert, KeyFile: key})
	require.NoError(t, err)
	serveEcho(t, l)
	addr := l.Addr().String()

	// TLS 1.3 works.
	c, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // test
	require.NoError(t, err)
	_, _ = c.Write([]byte("ping"))
	buf := make([]byte, 4)
	_, err = io.ReadFull(c, buf)
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS13), c.ConnectionState().Version)
	c.Close()

	// TLS 1.1 is refused.
	_, err = tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS11}) //nolint:gosec // test
	assert.Error(t, err)

	// TLS 1.2 with a CBC suite is refused.
	_, err = tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12, //nolint:gosec // test
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA}})
	assert.Error(t, err)
}

func TestListenTLSClientCA(t *testing.T) {
	dir := t.TempDir()
	cert, key := selfSigned(t, dir)
	l, err := Listen("tcp", "127.0.0.1:0", config.ServerTLSConfig{Enabled: true, CertFile: cert, KeyFile: key, ClientCAFile: cert})
	require.NoError(t, err)
	serveEcho(t, l)
	c, err := tls.Dial("tcp", l.Addr().String(), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // test
	if err == nil {
		// TLS 1.3 reports the client-cert failure on first read.
		_, _ = c.Write([]byte("ping"))
		_, err = io.ReadFull(c, make([]byte, 4))
		c.Close()
	}
	assert.Error(t, err, "client certificate required")
}

func TestListenTLCPAndBoth(t *testing.T) {
	dir := t.TempDir()
	sc, sk := sm2Pair(t, dir, "sign", smx509.KeyUsageDigitalSignature)
	ec, ek := sm2Pair(t, dir, "enc", smx509.KeyUsageKeyEncipherment|smx509.KeyUsageDataEncipherment)
	cert, key := selfSigned(t, dir)
	for _, stack := range []string{config.TLSStackTLCP, config.TLSStackBoth} {
		cfg := config.ServerTLSConfig{Enabled: true, Stack: stack, CertFile: cert, KeyFile: key,
			TLCP: config.ServerTLCPConfig{SignCertFile: sc, SignKeyFile: sk, EncCertFile: ec, EncKeyFile: ek}}
		require.NoError(t, cfg.Validate())
		l, err := Listen("tcp", "127.0.0.1:0", cfg)
		require.NoError(t, err, stack)
		serveEcho(t, l)
		c, err := tlcp.Dial("tcp", l.Addr().String(), &tlcp.Config{InsecureSkipVerify: true})
		require.NoError(t, err, stack)
		_, _ = c.Write([]byte("ping"))
		_, err = io.ReadFull(c, make([]byte, 4))
		require.NoError(t, err, stack)
		c.Close()

		tc, err := tls.Dial("tcp", l.Addr().String(), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // test
		if stack == config.TLSStackBoth {
			require.NoError(t, err)
			_, _ = tc.Write([]byte("ping"))
			_, err = io.ReadFull(tc, make([]byte, 4))
			require.NoError(t, err)
			tc.Close()
		} else if err == nil {
			tc.Close()
			t.Fatal("tlcp-only stack must refuse standard TLS")
		}
	}
}
