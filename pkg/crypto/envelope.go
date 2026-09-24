// Package crypto provides at-rest envelope encryption for secrets stored by
// Privahub (node tokens, inst tokens, datasource credentials, private keys).
//
// Algorithm (国密, aligned with PrivShield pkg/crypto):
//   - cipher: SM4 (GB/T 32907-2016) in GCM mode (AEAD, 12-byte nonce, 16-byte tag);
//   - key derivation: HKDF (RFC 5869) with SM3 (GM/T 0004-2012), a fresh
//     16-byte random salt per record, so every record uses a distinct key;
//   - envelope: "enc:<version>:" + base64(salt || nonce || ciphertext || tag);
//     the header "enc:<version>:" is the GCM additional data, so stripping or
//     rewriting the version breaks authentication.
//
// Keys are versioned (crypto.keys + crypto.active_version) for rotation:
// new values use the active version, older versions keep decrypting.
package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/emmansun/gmsm/sm3"
	"github.com/emmansun/gmsm/sm4"
	"golang.org/x/crypto/hkdf"
)

const (
	// Prefix marks an envelope-encrypted value.
	Prefix = "enc:"
	// SaltSize is the per-record HKDF salt length.
	SaltSize = 16
	// NonceSize is the SM4-GCM nonce length.
	NonceSize = 12
	// MinKeySize is the minimum key material length in bytes.
	MinKeySize = 16

	sm4KeySize = 16
	tagSize    = 16
	hkdfInfo   = "privahub at-rest SM4-GCM envelope"
)

var (
	// ErrNoKey reports that no keyring is configured.
	ErrNoKey = errors.New("crypto: no encryption key configured")
	// ErrInvalidEnvelope reports a malformed envelope.
	ErrInvalidEnvelope = errors.New("crypto: invalid envelope")
	// ErrKeyVersionNotFound reports an envelope whose key version is unknown.
	ErrKeyVersionNotFound = errors.New("crypto: key version not found")
	// ErrDecrypt reports an authentication failure (wrong key or tampering).
	ErrDecrypt = errors.New("crypto: decryption failed (wrong key or tampered data)")
)

var versionRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,31}$`)

// Keyring holds versioned key material; it is immutable and safe for
// concurrent use.
type Keyring struct {
	keys   map[string][]byte
	active string
}

// ParseKeyMaterial decodes a configured key: hex when the value is valid hex
// of at least 32 characters (e.g. `openssl rand -hex 32`), raw bytes
// otherwise. At least MinKeySize bytes are required.
func ParseKeyMaterial(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 2*MinKeySize && len(s)%2 == 0 {
		if b, err := hex.DecodeString(s); err == nil {
			return b, nil
		}
	}
	if len(s) < MinKeySize {
		return nil, fmt.Errorf("key material must be at least %d bytes (or %d hex characters)", MinKeySize, 2*MinKeySize)
	}
	return []byte(s), nil
}

// NewKeyring builds a keyring from version -> key material. active must be
// one of the versions.
func NewKeyring(material map[string]string, active string) (*Keyring, error) {
	if len(material) == 0 {
		return nil, ErrNoKey
	}
	kr := &Keyring{keys: make(map[string][]byte, len(material)), active: strings.ToLower(strings.TrimSpace(active))}
	for v, m := range material {
		v = strings.ToLower(strings.TrimSpace(v))
		if !versionRe.MatchString(v) {
			return nil, fmt.Errorf("invalid key version %q (use [a-z0-9_.-], e.g. v1)", v)
		}
		b, err := ParseKeyMaterial(m)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", v, err)
		}
		kr.keys[v] = b
	}
	if _, ok := kr.keys[kr.active]; !ok {
		return nil, fmt.Errorf("active key version %q is not configured (have %s)", active, strings.Join(kr.Versions(), ","))
	}
	return kr, nil
}

// ActiveVersion returns the version used for new ciphertexts.
func (k *Keyring) ActiveVersion() string { return k.active }

// Versions returns the configured versions, sorted.
func (k *Keyring) Versions() []string {
	out := make([]string, 0, len(k.keys))
	for v := range k.keys {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func deriveGCM(master, salt []byte) (cipher.AEAD, error) {
	key := make([]byte, sm4KeySize)
	if _, err := io.ReadFull(hkdf.New(sm3.New, master, salt, []byte(hkdfInfo)), key); err != nil {
		return nil, err
	}
	defer zero(key)
	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt seals plaintext with the active key. The empty string stays empty.
func (k *Keyring) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if k == nil {
		return "", ErrNoKey
	}
	salt := make([]byte, SaltSize+NonceSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("crypto: random: %w", err)
	}
	gcm, err := deriveGCM(k.keys[k.active], salt[:SaltSize])
	if err != nil {
		return "", err
	}
	header := Prefix + k.active + ":"
	out := gcm.Seal(salt, salt[SaltSize:], []byte(plaintext), []byte(header))
	return header + base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt opens an envelope produced by Encrypt.
func (k *Keyring) Decrypt(envelope string) (string, error) {
	if envelope == "" {
		return "", nil
	}
	version, payload, ok := splitEnvelope(envelope)
	if !ok {
		return "", ErrInvalidEnvelope
	}
	if k == nil {
		return "", ErrNoKey
	}
	master, ok := k.keys[version]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrKeyVersionNotFound, version)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) < SaltSize+NonceSize+tagSize {
		return "", ErrInvalidEnvelope
	}
	gcm, err := deriveGCM(master, data[:SaltSize])
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, data[SaltSize:SaltSize+NonceSize], data[SaltSize+NonceSize:], []byte(Prefix+version+":"))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(pt), nil
}

// NeedsRotation reports whether value is plaintext or sealed with a
// non-active key version.
func (k *Keyring) NeedsRotation(value string) bool {
	if value == "" || k == nil {
		return false
	}
	v, _, ok := splitEnvelope(value)
	return !ok || v != k.active
}

func splitEnvelope(s string) (version, payload string, ok bool) {
	if !strings.HasPrefix(s, Prefix) {
		return "", "", false
	}
	rest := s[len(Prefix):]
	i := strings.IndexByte(rest, ':')
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// IsEncrypted reports whether s looks like an envelope.
func IsEncrypted(s string) bool {
	_, _, ok := splitEnvelope(s)
	return ok
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// --- process-wide default keyring (set once at startup) ---

var defaultKeyring atomic.Pointer[Keyring]

// SetDefault installs the process-wide keyring used by the database field
// serializer (nil disables at-rest encryption: values are stored as-is).
func SetDefault(k *Keyring) { defaultKeyring.Store(k) }

// Default returns the process-wide keyring (nil when not configured).
func Default() *Keyring { return defaultKeyring.Load() }

// Seal encrypts with the default keyring; without a keyring the value is
// returned unchanged (dev/test without crypto keys). Already sealed values
// are returned unchanged.
func Seal(plaintext string) (string, error) {
	k := Default()
	if k == nil || plaintext == "" || IsEncrypted(plaintext) {
		return plaintext, nil
	}
	return k.Encrypt(plaintext)
}

// Open decrypts with the default keyring; plaintext (legacy, not yet
// migrated) values are returned unchanged.
func Open(value string) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}
	return Default().Decrypt(value)
}

// ConstantTimeEqual compares two secrets in constant time (with respect to
// their content; lengths are compared first).
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
