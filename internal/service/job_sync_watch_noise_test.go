package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// flakyWatcher fails the first `fail` calls, then delivers one event and
// blocks until the context ends.
type flakyWatcher struct {
	mu    sync.Mutex
	fail  int
	calls int
}

func (w *flakyWatcher) WatchJobs(ctx context.Context, _ int64, fn func(*kuscia.WatchJobEvent) error) error {
	w.mu.Lock()
	w.calls++
	n := w.calls
	w.mu.Unlock()
	if n <= w.fail {
		return &kuscia.APIError{Path: kuscia.WatchJobPath, HTTPStatus: http.StatusBadGateway}
	}
	_ = fn(&kuscia.WatchJobEvent{Type: kuscia.WatchEventModified}) // no object: ignored by HandleWatchEvent
	<-ctx.Done()
	return nil
}

func (w *flakyWatcher) Calls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func newNoiseSync(t *testing.T) (*JobStatusSyncService, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	s := NewJobStatusSyncService(nil, nil, zap.New(core))
	s.backoffMin, s.backoffMax = time.Millisecond, 4*time.Millisecond
	s.readyEvery = 5 * time.Millisecond
	return s, logs
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func countLogs(logs *observer.ObservedLogs, lvl zapcore.Level, prefix string) int {
	n := 0
	for _, e := range logs.All() {
		if e.Level == lvl && len(e.Message) >= len(prefix) && e.Message[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}

// Repeated watch failures log one WARN, then DEBUG until the stream recovers (INFO).
func TestJobSyncWatch_FailureLogsDeduplicated(t *testing.T) {
	s, logs := newNoiseSync(t)
	w := &flakyWatcher{fail: 5}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.watchNode(ctx, "tee", w); close(done) }()
	waitUntil(t, "recovery", func() bool { return countLogs(logs, zapcore.InfoLevel, "kuscia job watch recovered") == 1 })
	cancel()
	<-done
	if got := countLogs(logs, zapcore.WarnLevel, "kuscia job watch failed"); got != 1 {
		t.Fatalf("want exactly 1 WARN, got %d", got)
	}
	if got := countLogs(logs, zapcore.DebugLevel, "kuscia job watch still failing"); got != 4 {
		t.Fatalf("want 4 DEBUG repeats, got %d", got)
	}
	if w.Calls() != 6 {
		t.Fatalf("calls = %d", w.Calls())
	}
}

// A pool node rejected by the readiness gate is never connected to and the
// pause is logged once; the watch starts when the node becomes ready.
func TestJobSyncWatch_NodeNotReadyIsNotWatched(t *testing.T) {
	s, logs := newNoiseSync(t)
	var ready atomic.Bool
	var checks atomic.Int32
	s.SetWatchNodeReady(func(_ context.Context, domainID string) bool {
		if domainID != "tee" {
			t.Errorf("unexpected domain %q", domainID)
		}
		checks.Add(1)
		return ready.Load()
	})
	w := &flakyWatcher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.watchNode(ctx, "tee", w); close(done) }()
	waitUntil(t, "several readiness checks", func() bool { return checks.Load() >= 5 })
	if w.Calls() != 0 {
		t.Fatalf("watch connected to a node that is not ready (%d calls)", w.Calls())
	}
	if got := countLogs(logs, zapcore.InfoLevel, "kuscia job watch paused"); got != 1 {
		t.Fatalf("pause should be logged once, got %d", got)
	}
	if got := len(logs.FilterLevelExact(zapcore.WarnLevel).All()); got != 0 {
		t.Fatalf("no WARN expected while paused, got %d", got)
	}
	ready.Store(true)
	waitUntil(t, "watch after ready", func() bool { return w.Calls() == 1 })
	if got := countLogs(logs, zapcore.InfoLevel, "kuscia job watch resumed"); got != 1 {
		t.Fatalf("resume should be logged once, got %d", got)
	}
	cancel()
	<-done
}

// The default ("") watch is never gated.
func TestJobSyncWatch_DefaultWatchIgnoresReadinessGate(t *testing.T) {
	s, _ := newNoiseSync(t)
	s.SetWatchNodeReady(func(context.Context, string) bool { return false })
	w := &flakyWatcher{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.watchNode(ctx, "", w); close(done) }()
	waitUntil(t, "default watch", func() bool { return w.Calls() == 1 })
	cancel()
	<-done
}

type fakeDomainQuerier struct {
	resp *kuscia.QueryDomainResponse
	err  error
}

func (f fakeDomainQuerier) QueryDomain(context.Context, string) (*kuscia.QueryDomainResponse, error) {
	return f.resp, f.err
}

func TestKusciaDomainReady(t *testing.T) {
	readyResp := &kuscia.QueryDomainResponse{Data: kuscia.DomainInfo{NodeStatuses: []kuscia.NodeStatus{{Status: kuscia.DomainStatusReady}}}}
	notReady := &kuscia.QueryDomainResponse{Data: kuscia.DomainInfo{NodeStatuses: []kuscia.NodeStatus{{Status: "NotReady"}}}}
	noNodes := &kuscia.QueryDomainResponse{}
	cases := []struct {
		name string
		q    fakeDomainQuerier
		want bool
	}{
		{"ready instance", fakeDomainQuerier{resp: readyResp}, true},
		{"instance not ready", fakeDomainQuerier{resp: notReady}, false},
		{"domain without lite (tee)", fakeDomainQuerier{resp: noNodes}, false},
		{"domain missing", fakeDomainQuerier{err: &kuscia.APIError{Code: kuscia.ErrCodeDomainNotExists}}, false},
		{"master unreachable: let the watch retry", fakeDomainQuerier{err: errors.New("dial tcp: refused")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := KusciaDomainReady(tc.q, time.Second)(context.Background(), "tee"); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
