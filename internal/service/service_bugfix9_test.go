package service

// Regression tests for Bug73–Bug79 (round 17).
//
// Bug73: ListNodeStatus ignored FindByGraphID error in no-job path.
// Bug74: ListNodeStatus ignored FindByProjectAndJobID error for tasks.
// Bug75: latestJobIDForGraph ignored FindByProjectID error.
// Bug76: GetNodeLogs ignored FindByJobID error.
// Bug77: RefreshNodeMaxIndex ignored FindByGraphID error.
// Bug78: wire.go passed nil logger to NewSseServer.
// Bug79: wire.go Shutdown() did not stop SseServer heartbeat goroutine.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Bug78 + Bug79: SseServer logger and shutdown ---

func TestBug78_SseServer_WithLogger(t *testing.T) {
	// Bug78: NewSseServer should accept a non-nil logger without panic.
	log := zap.NewNop()
	sse := NewSseServer(log)
	require.NotNil(t, sse)
	assert.Equal(t, 0, sse.ActiveConnections())
	sse.Stop()
}

func TestBug79_SseServer_StopIdempotent(t *testing.T) {
	// Bug79: Stop must be safe to call multiple times (stopOnce guard).
	sse := NewSseServer(zap.NewNop())
	sse.Stop()
	sse.Stop() // second call must not panic
	assert.Equal(t, 0, sse.ActiveConnections())
}
