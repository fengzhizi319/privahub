// Package innerauth carries the shared inner-port token
// (server.inner_auth.token) on this platform's node-to-node HTTP calls.
package innerauth

import (
	"net/http"
	"sync/atomic"
)

// TokenHeader carries the shared inner-port token.
const TokenHeader = "X-Privahub-Inner-Token"

var outgoing atomic.Value // string

// SetOutgoingToken sets the token added to outgoing inner calls ("" = none).
func SetOutgoingToken(token string) { outgoing.Store(token) }

// OutgoingToken returns the configured outgoing token.
func OutgoingToken() string {
	t, _ := outgoing.Load().(string)
	return t
}

// Apply adds the token header to an outgoing inner request when configured
// (replacing any value copied from an incoming request).
func Apply(req *http.Request) {
	if req == nil {
		return
	}
	req.Header.Del(TokenHeader)
	if t := OutgoingToken(); t != "" {
		req.Header.Set(TokenHeader, t)
	}
}
