package votehandler

import (
	"bytes"
	"container/list"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// Vote signature hardening (on top of the Java-compatible SHA256withRSA
// signature of VoteRequestMessage / VoteReplyMessage):
//
//   - the leaf certificate must carry KeyUsage digitalSignature and chain to
//     the signer's domain certificate as registered in Kuscia (the trust
//     anchor is Kuscia's copy, the message-supplied chain[1] must equal it);
//   - the signer must be the party the message speaks for (reply: the body's
//     voter is the invite's voter, the vote id matches the request);
//   - new signatures carry signed_at (unix ms) + nonce inside the signed
//     body: the leaf must have been valid at signed_at, signed_at may not be
//     in the future, a reply cannot predate its request, and a nonce reused
//     with a different body is rejected as a replay (nonces kept with a TTL);
//   - legacy signatures (no signed_at, from older peers) are still accepted:
//     re-verification of stored messages checks the signature and chain only,
//     while fresh verification requires the leaf to be valid now or expired
//     for less than LegacySignatureWindow;
//   - without Kuscia (offline mode) verification is skipped only when strict
//     signatures are off (dev/test); in production it fails closed.

const maxClockSkew = 5 * time.Minute

// LegacySignatureWindow bounds how long after the leaf certificate expired a
// legacy (untimestamped) signature is still accepted by fresh verification
// (PRIVAHUB_VOTE_LEGACY_SIGNATURE_WINDOW, default 720h).
var LegacySignatureWindow = envDuration("PRIVAHUB_VOTE_LEGACY_SIGNATURE_WINDOW", 30*24*time.Hour)

// NonceTTL is how long signature nonces are remembered
// (PRIVAHUB_VOTE_NONCE_TTL, default 720h).
var NonceTTL = envDuration("PRIVAHUB_VOTE_NONCE_TTL", 30*24*time.Hour)

var timeNow = time.Now

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			return time.Duration(h) * time.Hour
		}
	}
	return def
}

// SignMeta is embedded (signed) in new request/reply bodies.
type SignMeta struct {
	SignedAt int64  `json:"signed_at,omitempty"` // unix milliseconds
	Nonce    string `json:"nonce,omitempty"`
}

func newSignMeta() SignMeta {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return SignMeta{SignedAt: timeNow().UnixMilli(), Nonce: hex.EncodeToString(b)}
}

func (m SignMeta) timestamped() bool { return m.SignedAt > 0 && m.Nonce != "" }

func (m SignMeta) time() time.Time { return time.UnixMilli(m.SignedAt) }

var errSignature = errors.New("vote signature rejected")

func sigErr(format string, a ...interface{}) error {
	return fmt.Errorf("%w: %s", errSignature, fmt.Sprintf(format, a...))
}

// verifySignerChain checks the message certificate chain against the
// signer's Kuscia domain certificate.
//
//	chain[0]  leaf (vote key) issued by the domain key
//	chain[1]  the signer's domain certificate (must equal trustedDomainCert)
//
// fresh selects the time policy (see package comment).
func verifySignerChain(chain []string, trustedDomainCert string, meta SignMeta, fresh bool) error {
	if len(chain) < 2 {
		return sigErr("certificate chain missing")
	}
	if trustedDomainCert == "" {
		return sigErr("signer domain certificate unknown")
	}
	anchor, err := ParseCertificate(trustedDomainCert)
	if err != nil {
		return sigErr("parse domain certificate: %v", err)
	}
	presented, err := ParseCertificate(chain[1])
	if err != nil {
		return sigErr("parse chain certificate: %v", err)
	}
	if !bytes.Equal(anchor.RawSubjectPublicKeyInfo, presented.RawSubjectPublicKeyInfo) {
		return sigErr("certificate chain does not belong to the signer")
	}
	leaf, err := ParseCertificate(chain[0])
	if err != nil {
		return sigErr("parse leaf certificate: %v", err)
	}
	if !bytes.Equal(leaf.RawIssuer, anchor.RawSubject) {
		return sigErr("leaf not issued by the signer domain")
	}
	if err := leaf.CheckSignatureFrom(anchor); err != nil {
		return sigErr("leaf not signed by the signer domain: %v", err)
	}
	if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return sigErr("leaf certificate lacks keyUsage digitalSignature")
	}
	now := timeNow()
	switch {
	case meta.timestamped():
		at := meta.time()
		if at.Before(leaf.NotBefore.Add(-maxClockSkew)) || at.After(leaf.NotAfter.Add(maxClockSkew)) {
			return sigErr("leaf certificate not valid at signing time")
		}
		if at.Before(anchor.NotBefore.Add(-maxClockSkew)) || at.After(anchor.NotAfter.Add(maxClockSkew)) {
			return sigErr("domain certificate not valid at signing time")
		}
		if fresh && at.After(now.Add(maxClockSkew)) {
			return sigErr("signing time is in the future")
		}
	case fresh:
		if now.Before(leaf.NotBefore.Add(-maxClockSkew)) || now.After(leaf.NotAfter.Add(LegacySignatureWindow)) {
			return sigErr("legacy signature outside its validity window")
		}
	}
	return nil
}

// --- nonce replay cache ---

type nonceEntry struct {
	nonce   string
	digest  [32]byte
	expires time.Time
}

// nonceCache remembers nonce -> body digest with a TTL (bounded LRU). The
// same (nonce, body) may be verified any number of times (stored messages are
// re-verified); the same nonce with another body is a replay/forgery.
type nonceCache struct {
	mu    sync.Mutex
	max   int
	items map[string]*list.Element
	order *list.List
}

func newNonceCache(max int) *nonceCache {
	return &nonceCache{max: max, items: map[string]*list.Element{}, order: list.New()}
}

var voteNonces = newNonceCache(200000)

// check records (nonce, body) and reports whether it is acceptable.
func (c *nonceCache) check(nonce string, body []byte) bool {
	d := sha256.Sum256(body)
	now := timeNow()
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[nonce]; ok {
		e := el.Value.(*nonceEntry)
		if now.Before(e.expires) {
			return e.digest == d
		}
		c.order.Remove(el)
		delete(c.items, nonce)
	}
	c.items[nonce] = c.order.PushFront(&nonceEntry{nonce: nonce, digest: d, expires: now.Add(NonceTTL)})
	for c.order.Len() > c.max {
		old := c.order.Back()
		c.order.Remove(old)
		delete(c.items, old.Value.(*nonceEntry).nonce)
	}
	return true
}

// checkNonce applies the replay cache to timestamped messages.
func checkNonce(meta SignMeta, signedBody string) error {
	if !meta.timestamped() {
		return nil
	}
	if !voteNonces.check(meta.Nonce, []byte(signedBody)) {
		return sigErr("nonce replayed with a different message")
	}
	return nil
}
