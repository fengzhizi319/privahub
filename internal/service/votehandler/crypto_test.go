package votehandler

import (
	"testing"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	p := newTestPKI(t, "alice")
	data := []byte(b64(`{"vote_request_id":"v1"}`))
	sig, err := SignSHA256withRSA(data, p.keyB64)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// leaf cert accepted as PEM, base64(PEM) and base64(DER)
	for name, cert := range map[string]string{"pem": p.leafPEM, "b64pem": b64(p.leafPEM)} {
		if err := VerifySHA256withRSA(data, cert, sig); err != nil {
			t.Errorf("%s: verify: %v", name, err)
		}
	}
	c, _ := ParseCertificate(p.leafPEM)
	if err := VerifySHA256withRSA(data, b64(string(c.Raw)), sig); err != nil {
		t.Errorf("der: verify: %v", err)
	}
	if err := VerifySHA256withRSA([]byte("tampered"), p.leafPEM, sig); err == nil {
		t.Error("tampered data must not verify")
	}
	other := newTestPKI(t, "bob")
	if err := VerifySHA256withRSA(data, other.leafPEM, sig); err == nil {
		t.Error("foreign cert must not verify")
	}
}

func TestCertHelpers(t *testing.T) {
	a := newTestPKI(t, "alice")
	b := newTestPKI(t, "bob")
	if !ValidateCertChain([]string{b64(a.leafPEM), b64(a.caPEM)}) {
		t.Error("valid chain rejected")
	}
	if !ValidateCertChain(certChainToPEM([]string{b64(a.leafPEM), b64(a.caPEM)})) {
		t.Error("valid PEM chain rejected")
	}
	if ValidateCertChain([]string{a.leafPEM, b.caPEM}) {
		t.Error("foreign CA accepted")
	}
	if ValidateCertChain(nil) {
		t.Error("empty chain accepted")
	}
	if !CompareCertPubKey(b64(a.caPEM), a.caPEM) {
		t.Error("same cert should compare equal")
	}
	if CompareCertPubKey(a.caPEM, b.caPEM) || CompareCertPubKey("", a.caPEM) {
		t.Error("different certs compare equal")
	}
	if _, err := ParsePrivateKey("not a key"); err == nil {
		t.Error("garbage key parsed")
	}
}
