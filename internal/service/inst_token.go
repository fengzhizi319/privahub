package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Inst tokens (P2P node registration, Java TokenUtil) are HS256 JWTs with
// audience = nodeId. Java signs them with secret = instId, which is not a
// secret at all (inst ids are exchanged between institutions). Go signs with
// a key derived from the server secret (auth.jwt_secret) and marks the token
// with the "kv" claim; tokens issued with the legacy instId key before this
// process started are still accepted until they expire (30 minutes).

const (
	instTokenKeyVersion = 2
	instTokenKeyLabel   = "privahub/inst-token/v2/"
)

var instTokenKeys = struct {
	sync.RWMutex
	secret       []byte
	legacyCutoff time.Time
}{legacyCutoff: time.Now()}

// SetInstTokenSecret installs the server secret inst tokens are signed with
// (wired from auth.jwt_secret). Legacy instId-signed tokens issued before the
// call keep verifying until they expire.
func SetInstTokenSecret(secret string) {
	instTokenKeys.Lock()
	defer instTokenKeys.Unlock()
	if secret == "" {
		instTokenKeys.secret = nil
		return
	}
	instTokenKeys.secret = []byte(secret)
	instTokenKeys.legacyCutoff = time.Now()
}

// instTokenKey derives the per-institution signing key; nil when no server
// secret is configured.
func instTokenKey(instID string) []byte {
	instTokenKeys.RLock()
	secret := instTokenKeys.secret
	instTokenKeys.RUnlock()
	if len(secret) == 0 {
		return nil
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(instTokenKeyLabel + instID))
	return m.Sum(nil)
}

// SignInstToken mirrors Java TokenUtil.sign (audience=nodeId, timestamp /
// signNonce claims, 30 minute expiry), signed with the server-secret derived
// key (legacy instId key only when no server secret is configured).
func SignInstToken(instID, nodeID string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"aud":       []string{nodeID},
		"timestamp": now.UnixMilli(),
		"signNonce": uuid.NewString(),
		"exp":       now.Add(instTokenExpire).Unix(),
	}
	key := instTokenKey(instID)
	if key == nil {
		key = []byte(instID)
	} else {
		claims["kv"] = instTokenKeyVersion
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

// VerifyInstToken verifies an inst token for (instID, nodeID): HS256 only,
// audience and expiry required. Server-secret tokens ("kv") verify with the
// derived key; legacy instId-signed tokens are accepted only when issued
// before the server secret was installed (backward compatibility until they
// expire) or when no server secret is configured.
func VerifyInstToken(token, instID, nodeID string) bool {
	keyFor := func(legacy bool) jwt.Keyfunc {
		return func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok || t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected alg %v", t.Header["alg"])
			}
			if legacy {
				return []byte(instID), nil
			}
			key := instTokenKey(instID)
			if key == nil {
				return nil, fmt.Errorf("no inst token secret")
			}
			return key, nil
		}
	}
	opts := []jwt.ParserOption{jwt.WithAudience(nodeID), jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})}
	if parsed, err := jwt.Parse(token, keyFor(false), opts...); err == nil && parsed.Valid {
		return true
	}
	// Legacy (instId-keyed) token.
	parsed, err := jwt.Parse(token, keyFor(true), opts...)
	if err != nil || !parsed.Valid {
		return false
	}
	claims, _ := parsed.Claims.(jwt.MapClaims)
	if _, marked := claims["kv"]; marked {
		return false // a server-secret token must never verify with the instId key
	}
	instTokenKeys.RLock()
	hasSecret, cutoff := len(instTokenKeys.secret) > 0, instTokenKeys.legacyCutoff
	instTokenKeys.RUnlock()
	if !hasSecret {
		return true
	}
	ts, ok := claims["timestamp"].(float64)
	exp, _ := claims.GetExpirationTime()
	if !ok || exp == nil {
		return false
	}
	issued := time.UnixMilli(int64(ts))
	// Issued by the previous (instId-keyed) version, with the Java lifetime.
	return issued.Before(cutoff) && exp.Time.Sub(issued) <= instTokenExpire+time.Minute
}
