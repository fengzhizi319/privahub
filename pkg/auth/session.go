// Copyright 2026 The Privahub Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"regexp"

	"github.com/fengzhizi319/privahub/pkg/sm3"
)

// TokenDigest returns the value stored in user_tokens.token for a bearer
// token: lowercase hex SM3("privahub-session:" ‖ token). Only the digest is
// persisted, so a database leak does not yield usable session tokens.
func TokenDigest(token string) string {
	return sm3.SumHex([]byte("privahub-session:" + token))
}

// RefreshJTIDigest returns the stored form of a refresh token JTI.
func RefreshJTIDigest(jti string) string {
	if jti == "" {
		return ""
	}
	return sm3.SumHex([]byte("privahub-refresh:" + jti))
}

var tokenDigestRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// IsTokenDigest reports whether a stored user_tokens.token value is already a
// digest (legacy rows hold the raw JWT / Java UUID token).
func IsTokenDigest(s string) bool { return tokenDigestRe.MatchString(s) }
