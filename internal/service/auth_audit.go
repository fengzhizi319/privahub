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
	"context"

	"github.com/fengzhizi319/privahub/internal/audit"
)

// Additional auth audit actions (see audit.Action* for the common ones).
const (
	auditActionRefreshReuse   = "auth.refresh.reuse"
	auditActionSessionRevoked = "auth.session.revoked"
)

// clientIPKey carries the client IP of the request through the service layer
// (set by handlers with WithClientIP) so password changes can be audited.
type clientIPKey struct{}

// WithClientIP returns ctx annotated with the caller's IP for auth auditing.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

func clientIPFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// recordAuthEvent emits a security audit event for an authentication action.
// It never includes secrets (hashes, tokens): only user, IP, result and a
// short machine-readable reason.
func recordAuthEvent(ctx context.Context, action, user, ip, result, reason string) {
	if ip == "" {
		ip = clientIPFrom(ctx)
	}
	audit.Record(ctx, audit.Event{
		Actor:        user,
		ActorType:    audit.ActorUser,
		IP:           ip,
		Action:       action,
		ResourceType: "user",
		ResourceID:   user,
		Result:       result,
		Reason:       reason,
	})
}
