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

package service

import (
	"sync"
	"time"
)

// loginThrottle is an in-memory failure counter with lockout, used for the
// (account, client IP) pairs and for unknown accounts. Existing accounts are
// additionally counted in user_accounts (failed_attempts/locked_invalid_time)
// against the higher account-wide threshold.
type loginThrottle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
}

type throttleEntry struct {
	count       int
	windowEnd   time.Time // counting window (= lock duration since last failure)
	lockedUntil time.Time
}

const maxThrottleEntries = 100_000

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{entries: make(map[string]*throttleEntry)}
}

func throttleKey(name, ip string) string { return "p\x00" + name + "\x00" + ip }
func accountKey(name string) string      { return "a\x00" + name }

// fail records a failure and reports whether this failure (newly) locked key.
func (t *loginThrottle) fail(key string, now time.Time, max int, lock time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[key]
	if e == nil || (now.After(e.windowEnd) && now.After(e.lockedUntil)) {
		if len(t.entries) >= maxThrottleEntries {
			t.gcLocked(now)
		}
		e = &throttleEntry{}
		t.entries[key] = e
	}
	if now.Before(e.lockedUntil) {
		return false
	}
	e.count++
	e.windowEnd = now.Add(lock)
	if e.count >= max {
		e.lockedUntil = now.Add(lock)
		e.count = 0
		return true
	}
	return false
}

// lockedUntil returns the lock expiry of key when it is currently locked.
func (t *loginThrottle) lockedUntil(key string, now time.Time) (time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[key]
	if e == nil || !now.Before(e.lockedUntil) {
		return time.Time{}, false
	}
	return e.lockedUntil, true
}

func (t *loginThrottle) reset(key string) {
	t.mu.Lock()
	delete(t.entries, key)
	t.mu.Unlock()
}

// gcLocked drops expired entries; if the table is still full (attack), the
// oldest-expiring half is dropped to bound memory.
func (t *loginThrottle) gcLocked(now time.Time) {
	for k, e := range t.entries {
		if now.After(e.windowEnd) && now.After(e.lockedUntil) {
			delete(t.entries, k)
		}
	}
	if len(t.entries) < maxThrottleEntries {
		return
	}
	i := 0
	for k := range t.entries {
		if i%2 == 0 {
			delete(t.entries, k)
		}
		i++
	}
}
