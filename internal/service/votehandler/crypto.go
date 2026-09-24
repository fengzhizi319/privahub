package votehandler

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// This file is the Go counterpart of Java EncryptUtils (SHA256withRSA
// signing / verification and certificate helpers).
//
// Kuscia GenerateKeyCerts returns the private key as base64(PEM PKCS#8) and
// each certificate as base64(PEM). Java stores certificates either in that
// form or already decoded to PEM text, so every parser here accepts PEM text,
// base64(PEM) and base64(DER).

// decodeB64 decodes standard or URL-safe base64, with or without padding and
// embedded newlines.
func decodeB64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("invalid base64")
}

// pemOrDER returns the DER bytes of the first PEM block of s, or of s itself
// when it is base64 of a PEM/DER value.
func pemOrDER(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty value")
	}
	if strings.Contains(s, "-----BEGIN") {
		blk, _ := pem.Decode([]byte(s))
		if blk == nil {
			return nil, errors.New("invalid PEM")
		}
		return blk.Bytes, nil
	}
	raw, err := decodeB64(s)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(raw, []byte("-----BEGIN")) {
		blk, _ := pem.Decode(raw)
		if blk == nil {
			return nil, errors.New("invalid PEM")
		}
		return blk.Bytes, nil
	}
	return raw, nil
}

// ParseCertificate parses an X.509 certificate given as PEM, base64(PEM) or base64(DER).
func ParseCertificate(s string) (*x509.Certificate, error) {
	der, err := pemOrDER(s)
	if err != nil {
		return nil, fmt.Errorf("decode certificate: %w", err)
	}
	return x509.ParseCertificate(der)
}

// ParsePrivateKey parses an RSA private key (PKCS#8 or PKCS#1) given as PEM,
// base64(PEM) or base64(DER).
func ParsePrivateKey(s string) (*rsa.PrivateKey, error) {
	der, err := pemOrDER(s)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return rk, nil
	}
	return x509.ParsePKCS1PrivateKey(der)
}

// SignSHA256withRSA signs data and returns the base64 signature (Java EncryptUtils.signSHA256withRSA).
func SignSHA256withRSA(data []byte, privateKey string) (string, error) {
	k, err := ParsePrivateKey(privateKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	sig, err := rsa.SignPKCS1v15(nil, k, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifySHA256withRSA verifies a base64 signature with the public key of cert
// (Java EncryptUtils.verifySHA256withRSA). It returns nil when valid.
func VerifySHA256withRSA(data []byte, cert, signature string) error {
	c, err := ParseCertificate(cert)
	if err != nil {
		return err
	}
	pub, ok := c.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("certificate public key is not RSA")
	}
	sig, err := decodeB64(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	sum := sha256.Sum256(data)
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
}

// CompareCertPubKey reports whether both certificates carry the same public
// key (Java EncryptUtils.compareCertPubKey).
func CompareCertPubKey(a, b string) bool {
	ca, err := ParseCertificate(a)
	if err != nil {
		return false
	}
	cb, err := ParseCertificate(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ca.RawSubjectPublicKeyInfo, cb.RawSubjectPublicKeyInfo)
}

// ValidateCertChain checks that the first certificate is issued (and signed)
// by the last one (Java EncryptUtils.validateCertChain).
func ValidateCertChain(chain []string) bool {
	if len(chain) == 0 {
		return false
	}
	root, err := ParseCertificate(chain[len(chain)-1])
	if err != nil {
		return false
	}
	leaf, err := ParseCertificate(chain[0])
	if err != nil {
		return false
	}
	if !bytes.Equal(leaf.RawIssuer, root.RawSubject) {
		return false
	}
	return leaf.CheckSignatureFrom(root) == nil
}

// certChainToPEM converts base64(PEM) chain entries to PEM text (Java
// `new String(Base64Utils.decode(e))`); PEM entries are kept.
func certChainToPEM(chain []string) []string {
	out := make([]string, 0, len(chain))
	for _, c := range chain {
		if strings.Contains(c, "-----BEGIN") {
			out = append(out, c)
			continue
		}
		if raw, err := decodeB64(c); err == nil {
			out = append(out, string(raw))
			continue
		}
		out = append(out, c)
	}
	return out
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func unb64(s string) ([]byte, error) { return decodeB64(s) }
