package audit

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Options configures the audit Service (mapped from config.AuditConfig).
type Options struct {
	QueueSize      int           // in-memory queue capacity (default 4096)
	BatchSize      int           // rows per DB transaction (default 128)
	SpoolDir       string        // local fsync'd spool for DB failures ("" disables)
	SpoolMaxBytes  int64         // spool cap (default 256MiB); beyond => dropped
	ReplayInterval time.Duration // spool replay period (default 30s)

	RetentionDays     int           // 0 = keep forever
	ArchiveDir        string        // segment export directory
	RetentionInterval time.Duration // archival run period (default 24h)

	AnchorFile     string
	AnchorURL      string
	AnchorToken    string
	AnchorInterval time.Duration // default 1h
	AnchorTimeout  time.Duration

	NodeID string
}

func (o *Options) normalize() {
	if o.QueueSize <= 0 {
		o.QueueSize = 4096
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 128
	}
	if o.SpoolMaxBytes <= 0 {
		o.SpoolMaxBytes = 256 << 20
	}
	if o.ReplayInterval <= 0 {
		o.ReplayInterval = 30 * time.Second
	}
	if o.RetentionInterval <= 0 {
		o.RetentionInterval = 24 * time.Hour
	}
	if o.AnchorInterval <= 0 {
		o.AnchorInterval = time.Hour
	}
}

// Service is the asynchronous, durable audit Recorder: a bounded queue feeds
// a single writer goroutine (one chain writer per process); DB failures fall
// back to a local spool that is replayed later.
type Service struct {
	store *Store
	log   *zap.Logger
	opt   Options
	spool *spool

	ch       chan model.AuditLogDO
	flushReq chan chan struct{}
	stopCh   chan struct{}
	done     chan struct{}
	bgWG     sync.WaitGroup
	started  atomic.Bool
	closed   atomic.Bool

	anchorMu       sync.Mutex
	lastAnchorSeq  int64
	lastAnchorHash string
	httpClient     *http.Client
}

// New creates the service (ensures the chain tail and spool directory).
func New(db *gorm.DB, log *zap.Logger, opt Options) (*Service, error) {
	if db == nil {
		return nil, errors.New("audit: nil db")
	}
	if log == nil {
		log = zap.NewNop()
	}
	opt.normalize()
	store := NewStore(db)
	if err := store.EnsureTail(context.Background()); err != nil {
		return nil, err
	}
	sp, err := newSpool(opt.SpoolDir, opt.SpoolMaxBytes)
	if err != nil {
		return nil, err
	}
	return &Service{
		store: store, log: log, opt: opt, spool: sp,
		ch:         make(chan model.AuditLogDO, opt.QueueSize),
		flushReq:   make(chan chan struct{}),
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Init creates and starts the service and installs it as the global
// recorder.
func Init(db *gorm.DB, log *zap.Logger, opt Options) (*Service, error) {
	s, err := New(db, log, opt)
	if err != nil {
		return nil, err
	}
	s.Start()
	SetRecorder(s)
	return s, nil
}

// Store exposes the chain store (query / verify).
func (s *Service) Store() *Store {
	if s == nil {
		return nil
	}
	return s.store
}

// Options returns the effective options.
func (s *Service) Options() Options { return s.opt }

// Start launches the writer (which first replays the spool), anchoring and
// retention goroutines.
func (s *Service) Start() {
	if s == nil || !s.started.CompareAndSwap(false, true) {
		return
	}
	go s.run()
	if s.opt.AnchorFile != "" || s.opt.AnchorURL != "" {
		s.bgWG.Add(1)
		go s.every(s.opt.AnchorInterval, func(ctx context.Context) {
			if err := s.AnchorNow(ctx); err != nil {
				s.log.Warn("audit anchoring failed", zap.Error(err))
			}
		})
	}
	if s.opt.RetentionDays > 0 {
		s.bgWG.Add(1)
		go s.every(s.opt.RetentionInterval, func(ctx context.Context) {
			n, err := s.store.ArchiveExpired(ctx, s.opt.ArchiveDir, s.opt.RetentionDays, time.Now())
			if err != nil {
				s.log.Error("audit retention archival failed", zap.Error(err))
			} else if n > 0 {
				s.log.Info("audit records archived", zap.Int64("records", n))
			}
		})
	}
}

func (s *Service) every(d time.Duration, fn func(context.Context)) {
	defer s.bgWG.Done()
	t := time.NewTicker(d)
	defer t.Stop()
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		fn(ctx)
	}
	run() // once at startup
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			run()
		}
	}
}

// Record enqueues an event without blocking. When the queue is full (or the
// service is closed) the event is written to the local spool; only when the
// spool is unavailable/full is it dropped (counted in
// privahub_audit_events_total{outcome="dropped"}).
func (s *Service) Record(_ context.Context, e Event) {
	if s == nil {
		return
	}
	row := rowFromEvent(uuid.NewString(), e)
	if !s.closed.Load() {
		select {
		case s.ch <- row:
			metricQueueDepth.Set(float64(len(s.ch)))
			return
		default:
		}
	}
	s.spoolOrDrop([]model.AuditLogDO{row}, "queue full")
}

func (s *Service) spoolOrDrop(rows []model.AuditLogDO, why string) {
	if err := s.spool.append(rows); err != nil {
		metricEvents.WithLabelValues("dropped").Add(float64(len(rows)))
		s.log.Error("AUDIT EVENTS DROPPED", zap.Int("count", len(rows)), zap.String("cause", why), zap.Error(err))
		return
	}
	metricEvents.WithLabelValues("spooled").Add(float64(len(rows)))
}

func (s *Service) run() {
	defer close(s.done)
	s.replay()
	t := time.NewTicker(s.opt.ReplayInterval)
	defer t.Stop()
	for {
		select {
		case r := <-s.ch:
			s.persist(s.drain([]model.AuditLogDO{r}, s.opt.BatchSize))
		case <-t.C:
			s.replay()
		case ack := <-s.flushReq:
			s.drainAll()
			s.replay()
			close(ack)
		case <-s.stopCh:
			s.drainAll()
			return
		}
	}
}

func (s *Service) drain(batch []model.AuditLogDO, max int) []model.AuditLogDO {
	for len(batch) < max {
		select {
		case r := <-s.ch:
			batch = append(batch, r)
		default:
			return batch
		}
	}
	return batch
}

func (s *Service) drainAll() {
	for {
		b := s.drain(nil, s.opt.BatchSize)
		if len(b) == 0 {
			return
		}
		s.persist(b)
	}
}

func (s *Service) persist(batch []model.AuditLogDO) {
	metricQueueDepth.Set(float64(len(s.ch)))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := s.store.Append(ctx, batch); err != nil {
		s.log.Warn("audit DB write failed, spooling", zap.Int("count", len(batch)), zap.Error(err))
		s.spoolOrDrop(batch, "db write failed")
		return
	}
	metricEvents.WithLabelValues("persisted").Add(float64(len(batch)))
}

// replay moves spooled events into the chain (idempotent by event id). A
// file is removed only after all of its rows were committed.
func (s *Service) replay() {
	if s.spool == nil {
		return
	}
	files, err := s.spool.rotate()
	if err != nil {
		s.log.Warn("audit spool rotate failed", zap.Error(err))
		return
	}
	for _, f := range files {
		rows, bad, err := readSpoolFile(f)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("audit spool read failed", zap.String("file", f), zap.Error(err))
			return
		}
		if bad > 0 {
			s.log.Warn("audit spool: skipped corrupt lines", zap.String("file", f), zap.Int("lines", bad))
		}
		for i := 0; i < len(rows); i += s.opt.BatchSize {
			end := min(i+s.opt.BatchSize, len(rows))
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			n, err := s.store.Append(ctx, rows[i:end])
			cancel()
			if err != nil {
				s.log.Warn("audit spool replay deferred (DB unavailable)", zap.Error(err))
				return
			}
			metricEvents.WithLabelValues("replayed").Add(float64(n))
		}
		s.spool.remove(f)
	}
}

// Flush waits until queued (and spooled, if the DB is reachable) events are
// persisted. Used by tests and shutdown.
func (s *Service) Flush(ctx context.Context) error {
	if s == nil || !s.started.Load() || s.closed.Load() {
		return nil
	}
	ack := make(chan struct{})
	select {
	case s.flushReq <- ack:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return nil
	}
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops accepting queued events (later ones go to the spool), drains
// the queue into the database and stops background loops.
func (s *Service) Close(ctx context.Context) error {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(s.stopCh)
	if !s.started.Load() {
		return nil
	}
	finished := make(chan struct{})
	go func() {
		<-s.done
		s.bgWG.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
		return ctx.Err()
	}
	// Events that raced with Close go to the spool (replayed on next start).
	if late := s.drain(nil, s.opt.QueueSize); len(late) > 0 {
		s.spoolOrDrop(late, "recorded during shutdown")
	}
	if s.opt.AnchorFile != "" || s.opt.AnchorURL != "" {
		actx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.AnchorNow(actx)
	}
	return nil
}
