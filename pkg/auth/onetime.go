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
	"crypto/rand"
	"errors"
	"math/big"
)

// oneTimeAlphabet omits look-alike characters (I O l 0 1).
const oneTimeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#%^&*"

// MinOneTimePasswordLength is the shortest password GenerateOneTimePassword
// accepts (16 chars of a 64-symbol alphabet = 96 bits).
const MinOneTimePasswordLength = 16

// GenerateOneTimePassword returns a random bootstrap / break-glass password
// of n characters (crypto/rand). Such passwords are only valid until the
// user replaces them (must_change_password).
func GenerateOneTimePassword(n int) (string, error) {
	if n < MinOneTimePasswordLength {
		return "", errors.New("one-time password too short")
	}
	b := make([]byte, n)
	max := big.NewInt(int64(len(oneTimeAlphabet)))
	for i := range b {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = oneTimeAlphabet[v.Int64()]
	}
	return string(b), nil
}
