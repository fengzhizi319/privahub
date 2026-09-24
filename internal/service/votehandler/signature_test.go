package votehandler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

// repliedCenterVote runs a center node-route vote up to bob's signed reply.
func repliedCenterVote(t *testing.T) (*testEnv, model.VoteInviteDO, model.VoteRequestDO) {
	t.Helper()
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	ctx := centerCtx()
	voteID, err := e.svc.CreateApproval(ctx, "alice", VoteTypeNodeRoute, &VoteConfig{
		SrcNodeID: "alice", DesNodeID: "bob", SrcNodeAddr: "http://alice:1080", DesNodeAddr: "http://bob:1080"})
	if err != nil {
		t.Fatal(err)
	}
	e.mon.SyncRequests(context.Background())
	if err := e.svc.Reply(ctx, ActionApproved, "ok", "bob", voteID); err != nil {
		t.Fatal(err)
	}
	return e, e.invite(t, voteID, "bob"), e.request(t, voteID)
}

func TestVoteSignature_NewFormatTimestamped(t *testing.T) {
	e, inv, vr := repliedCenterVote(t)
	_, reqBody, err := parseRequest(vr.RequestMsg)
	if err != nil || !reqBody.timestamped() {
		t.Fatalf("request body not timestamped: %+v %v", reqBody, err)
	}
	var reply VoteReplyMessage
	_ = json.Unmarshal([]byte(inv.VoteMsg), &reply)
	raw, _ := unb64(reply.Body)
	var body VoteReplyBody
	_ = json.Unmarshal(raw, &body)
	if !body.timestamped() {
		t.Fatalf("reply body not timestamped: %s", raw)
	}
	if !e.mon.VerifyReply(context.Background(), &inv, &vr) {
		t.Fatal("valid reply rejected")
	}
	// Re-verification of the same stored reply stays valid (idempotent nonce).
	if !e.mon.VerifyReply(context.Background(), &inv, &vr) {
		t.Fatal("re-verification rejected")
	}
}

func TestVoteSignature_ReplyBoundToInvite(t *testing.T) {
	e, inv, vr := repliedCenterVote(t)
	// Reply moved to the invite of a participant the signer cannot speak
	// for (not embedded in this center, not the signer's institution).
	other := inv
	other.VoteParticipantID = "carol"
	if e.mon.VerifyReply(context.Background(), &other, &vr) {
		t.Error("reply accepted for a foreign participant's invite")
	}
	// Signed APPROVE presented as a rejection.
	flipped := inv
	flipped.Action = ActionRejected
	if e.mon.VerifyReply(context.Background(), &flipped, &vr) {
		t.Error("signed action mismatch accepted")
	}
	// Reply replayed against another vote.
	vr2 := vr
	vr2.VoteID = "other-vote"
	if e.mon.VerifyReply(context.Background(), &inv, &vr2) {
		t.Error("reply accepted for another vote")
	}
}

func TestVoteSignature_StrictOffline(t *testing.T) {
	e, inv, vr := repliedCenterVote(t)
	e.svc.d.Kuscia = nil
	if !e.mon.VerifyReply(context.Background(), &inv, &vr) {
		t.Error("dev/test offline mode should skip the Kuscia chain check")
	}
	e.svc.d.StrictSignatures = true
	if e.mon.VerifyReply(context.Background(), &inv, &vr) {
		t.Error("strict mode must fail closed without Kuscia")
	}
	ab := &autonomyBase{base{e.svc.d}}
	msg, body, _ := parseRequest(vr.RequestMsg)
	if ab.verify(context.Background(), msg, body) == nil {
		t.Error("strict request verification must fail closed without Kuscia")
	}
}

func signLeaf(t *testing.T, p *testPKI, usage x509.KeyUsage, notBefore, notAfter time.Time) string {
	t.Helper()
	blk, _ := pem.Decode([]byte(p.caPEM))
	ca, _ := x509.ParseCertificate(blk.Bytes)
	caKeyPEM, _ := unb64(p.caKeyB64)
	kb, _ := pem.Decode(caKeyPEM)
	k, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, _ := rsa.GenerateKey(rand.Reader, 1024)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "vote"},
		NotBefore: notBefore, NotAfter: notAfter, KeyUsage: usage}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &leafKey.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestVerifySignerChain(t *testing.T) {
	p := newTestPKI(t, "alice")
	other := newTestPKI(t, "bob")
	now := time.Now()
	meta := SignMeta{SignedAt: now.UnixMilli(), Nonce: "n1"}
	good := []string{p.leafPEM, p.caPEM}
	if err := verifySignerChain(good, b64(p.caPEM), meta, true); err != nil {
		t.Fatalf("valid chain: %v", err)
	}
	if verifySignerChain(good, other.caPEM, meta, true) == nil {
		t.Error("chain of another domain accepted")
	}
	if verifySignerChain([]string{other.leafPEM, p.caPEM}, p.caPEM, meta, true) == nil {
		t.Error("foreign leaf accepted")
	}
	if verifySignerChain(good, "", meta, true) == nil {
		t.Error("unknown signer accepted")
	}
	noDS := signLeaf(t, p, x509.KeyUsageKeyEncipherment, now.Add(-time.Hour), now.Add(time.Hour))
	if verifySignerChain([]string{noDS, p.caPEM}, p.caPEM, meta, true) == nil {
		t.Error("leaf without digitalSignature accepted")
	}
	// Signing time outside the leaf validity.
	old := SignMeta{SignedAt: now.Add(-48 * time.Hour).UnixMilli(), Nonce: "n2"}
	if verifySignerChain(good, p.caPEM, old, false) == nil {
		t.Error("signature outside leaf validity accepted")
	}
	future := SignMeta{SignedAt: now.Add(30 * time.Minute).UnixMilli(), Nonce: "n3"}
	if verifySignerChain(good, p.caPEM, future, true) == nil {
		t.Error("future signing time accepted")
	}
	// Legacy (untimestamped): stored re-verification ignores time, fresh
	// verification accepts an expired leaf only within the window.
	expired := signLeaf(t, p, x509.KeyUsageDigitalSignature, now.Add(-72*time.Hour), now.Add(-48*time.Hour))
	if err := verifySignerChain([]string{expired, p.caPEM}, p.caPEM, SignMeta{}, false); err != nil {
		t.Errorf("stored legacy signature rejected: %v", err)
	}
	saved := LegacySignatureWindow
	defer func() { LegacySignatureWindow = saved }()
	LegacySignatureWindow = time.Hour
	if verifySignerChain([]string{expired, p.caPEM}, p.caPEM, SignMeta{}, true) == nil {
		t.Error("fresh legacy signature with long-expired leaf accepted")
	}
}

func TestNonceCache(t *testing.T) {
	c := newNonceCache(2)
	first := c.check("a", []byte("m1"))
	second := c.check("a", []byte("m1"))
	if !first || !second {
		t.Fatal("same nonce+message must be accepted repeatedly")
	}
	if c.check("a", []byte("m2")) {
		t.Fatal("nonce reuse with another message accepted")
	}
	c.check("b", []byte("x"))
	c.check("c", []byte("y")) // evicts "a"
	if !c.check("a", []byte("m2")) {
		t.Fatal("evicted nonce should be forgotten")
	}
}
