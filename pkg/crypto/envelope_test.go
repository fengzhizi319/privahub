package crypto

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKeyring(t *testing.T) *Keyring {
	t.Helper()
	kr, err := NewKeyring(map[string]string{
		"v1": "12345678901234567890123456789012",
		"v2": strings.Repeat("ab", 32),
	}, "v2")
	require.NoError(t, err)
	return kr
}

func TestEnvelopeRoundTrip(t *testing.T) {
	kr := testKeyring(t)
	ct, err := kr.Encrypt("node-token-secret")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ct, "enc:v2:"))
	assert.NotContains(t, ct, "node-token-secret")
	pt, err := kr.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, "node-token-secret", pt)

	ct2, _ := kr.Encrypt("node-token-secret")
	assert.NotEqual(t, ct, ct2, "random salt/nonce per record")

	empty, err := kr.Encrypt("")
	require.NoError(t, err)
	assert.Equal(t, "", empty)
}

func TestEnvelopeRotationAndVersions(t *testing.T) {
	old, err := NewKeyring(map[string]string{"v1": "12345678901234567890123456789012"}, "v1")
	require.NoError(t, err)
	ct, err := old.Encrypt("secret")
	require.NoError(t, err)

	kr := testKeyring(t)
	pt, err := kr.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, "secret", pt)
	assert.True(t, kr.NeedsRotation(ct))
	assert.True(t, kr.NeedsRotation("plain"))
	ct2, _ := kr.Encrypt("secret")
	assert.False(t, kr.NeedsRotation(ct2))

	only2, err := NewKeyring(map[string]string{"v2": strings.Repeat("ab", 32)}, "v2")
	require.NoError(t, err)
	_, err = only2.Decrypt(ct)
	assert.ErrorIs(t, err, ErrKeyVersionNotFound)
}

func TestEnvelopeTamper(t *testing.T) {
	kr := testKeyring(t)
	ct, _ := kr.Encrypt("secret")
	// Rewriting the version header breaks the AAD.
	forged := strings.Replace(ct, "enc:v2:", "enc:v1:", 1)
	_, err := kr.Decrypt(forged)
	assert.Error(t, err)

	raw, _ := base64.StdEncoding.DecodeString(ct[len("enc:v2:"):])
	raw[len(raw)-1] ^= 1
	_, err = kr.Decrypt("enc:v2:" + base64.StdEncoding.EncodeToString(raw))
	assert.ErrorIs(t, err, ErrDecrypt)

	_, err = kr.Decrypt("enc:v2:AAAA")
	assert.ErrorIs(t, err, ErrInvalidEnvelope)
}

func TestKeyringValidation(t *testing.T) {
	_, err := NewKeyring(map[string]string{"v1": "short"}, "v1")
	assert.Error(t, err)
	_, err = NewKeyring(map[string]string{"v1": "12345678901234567890123456789012"}, "v9")
	assert.Error(t, err)
	_, err = NewKeyring(map[string]string{"bad:ver": "12345678901234567890123456789012"}, "bad:ver")
	assert.Error(t, err)
	b, err := ParseKeyMaterial(strings.Repeat("0f", 16))
	require.NoError(t, err)
	assert.Len(t, b, 16)
}

func TestDefaultSealOpen(t *testing.T) {
	SetDefault(nil)
	v, err := Seal("x-plain")
	require.NoError(t, err)
	assert.Equal(t, "x-plain", v, "no keyring: passthrough")

	SetDefault(testKeyring(t))
	defer SetDefault(nil)
	v, err = Seal("x-plain")
	require.NoError(t, err)
	assert.True(t, IsEncrypted(v))
	again, _ := Seal(v)
	assert.Equal(t, v, again, "idempotent")
	pt, err := Open(v)
	require.NoError(t, err)
	assert.Equal(t, "x-plain", pt)
	pt, err = Open("legacy-plain")
	require.NoError(t, err)
	assert.Equal(t, "legacy-plain", pt)

	assert.True(t, ConstantTimeEqual("abc", "abc"))
	assert.False(t, ConstantTimeEqual("abc", "abd"))
	assert.False(t, ConstantTimeEqual("abc", "abcd"))
}
