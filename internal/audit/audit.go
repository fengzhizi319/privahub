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

// Package audit provides a tamper-evident security audit trail.
//
// Callers (auth, authz, HTTP middleware, ...) use Record to emit events. Record
// is nil-safe, never panics and never blocks the request path for long: events
// are handed to a process-wide Recorder (installed via SetRecorder / Init)
// which persists them asynchronously in a hash chain.
package audit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/logger"
)

// Actor types.
const (
	ActorUser   = "user"
	ActorNode   = "node"
	ActorSystem = "system"
	// ActorAnonymous is an unauthenticated caller (e.g. a failed login).
	ActorAnonymous = "anonymous"
)

// Results.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
	ResultDenied  = "denied"
)

// Well-known actions (free-form strings are allowed as well).
const (
	ActionLogin          = "auth.login"
	ActionLogout         = "auth.logout"
	ActionLockout        = "auth.lockout"
	ActionPasswordChange = "auth.password.change"
	ActionPasswordReset  = "auth.password.reset"
	ActionAuthzDenied    = "authz.denied"
)

// Event is one audit record.
type Event struct {
	Time         time.Time         // zero => now
	Actor        string            // user id / name / node id
	ActorType    string            // user / node / system
	OwnerID      string            // owning node / tenant
	IP           string            // client IP
	Action       string            // e.g. "auth.login", "POST /api/v1alpha1/project/create"
	ResourceType string            // e.g. "project"
	ResourceID   string            // e.g. project id
	Result       string            // success / failure / denied
	Reason       string            // short reason (never secrets)
	TraceID      string            // request / trace id
	Extra        map[string]string // additional non-sensitive attributes
}

// Recorder persists audit events.
type Recorder interface {
	Record(ctx context.Context, e Event)
}

type recorderHolder struct{ r Recorder }

var global atomic.Pointer[recorderHolder]

// SetRecorder installs the process-wide recorder (nil disables recording).
func SetRecorder(r Recorder) {
	if r == nil {
		global.Store(nil)
		return
	}
	global.Store(&recorderHolder{r: r})
}

// GetRecorder returns the current recorder or nil.
func GetRecorder() Recorder {
	h := global.Load()
	if h == nil {
		return nil
	}
	return h.r
}

// Record emits an audit event. Safe to call when no recorder is installed.
func Record(ctx context.Context, e Event) {
	defer func() { _ = recover() }()
	h := global.Load()
	if h == nil || h.r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	enrich(ctx, &e)
	if e.ActorType == "" {
		if e.Actor == "" {
			e.ActorType = ActorSystem
		} else {
			e.ActorType = ActorUser
		}
	}
	if e.Result == "" {
		e.Result = ResultSuccess
	}
	h.r.Record(ctx, e)
}

// enrich fills TraceID and actor fields from the request context when the
// caller left them empty.
func enrich(ctx context.Context, e *Event) {
	if e.TraceID == "" {
		if t, ok := ctx.Value(logger.TraceIDKey).(string); ok {
			e.TraceID = t
		}
	}
	if e.Actor == "" {
		if u, ok := auth.UserFromContext(ctx); ok && u != nil {
			e.Actor = u.Name
			if e.OwnerID == "" {
				e.OwnerID = u.OwnerID
			}
			if e.ActorType == "" {
				e.ActorType = ActorUser
				if u.IsVirtualNode() {
					e.ActorType = ActorNode
				}
			}
		}
	}
}
