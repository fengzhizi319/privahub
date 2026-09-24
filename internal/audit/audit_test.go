package audit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "audit.db")
	}
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AuditLogDO{}, &model.AuditChainTailDO{}, &model.AuditSegmentDO{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func newSvc(t *testing.T, db *gorm.DB, opt Options) *Service {
	t.Helper()
	if opt.SpoolDir == "" {
		opt.SpoolDir = filepath.Join(t.TempDir(), "spool")
	}
	s, err := New(db, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	return s
}

func flush(t *testing.T, s *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
}

func verify(t *testing.T, st *Store, opt VerifyOptions) *VerifyResult {
	t.Helper()
	r, err := st.Verify(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ev(i int) Event {
	return Event{
		// nanosecond precision on purpose: must be truncated before hashing
		Time:  time.Unix(1700000000, int64(i)*1000+789).Add(time.Duration(i) * time.Second),
		Actor: "alice", ActorType: ActorUser, IP: "10.0.0.1", Action: "auth.login",
		ResourceType: "user", ResourceID: fmt.Sprint(i), Result: ResultSuccess,
		Extra: map[string]string{"k": "v", "a": "b|c"},
	}
}

func TestChainCorrectness(t *testing.T) {
	db := openDB(t, "")
	s := newSvc(t, db, Options{})
	for i := 0; i < 250; i++ {
		s.Record(context.Background(), ev(i))
	}
	flush(t, s)
	r := verify(t, s.Store(), VerifyOptions{BatchSize: 37})
	if !r.Valid || r.Checked != 250 || r.LastSeq != 250 || r.TailSeq != 250 {
		t.Fatalf("unexpected verify result: %+v", r)
	}
	var first model.AuditLogDO
	db.Where("seq = 1").Take(&first)
	if first.PrevHash != GenesisHash {
		t.Fatalf("first prev hash %q", first.PrevHash)
	}
	if first.TsMicro%1000 == 789 || first.TsMicro != TruncateTime(ev(0).Time).UnixMicro() {
		t.Fatalf("timestamp not truncated to micros: %d", first.TsMicro)
	}
}

func TestCanonicalEncodingNoBoundaryAmbiguity(t *testing.T) {
	a := model.AuditLogDO{Seq: 1, Actor: "alice", ActorType: "user|x", PrevHash: GenesisHash}
	b := model.AuditLogDO{Seq: 1, Actor: "alice|user", ActorType: "x", PrevHash: GenesisHash}
	if ComputeHash(&a) == ComputeHash(&b) {
		t.Fatal("field boundary ambiguity")
	}
}

func TestTamperDetection(t *testing.T) {
	cases := []struct {
		name   string
		tamper func(db *gorm.DB)
		kind   string
		seq    int64
	}{
		{"modify row", func(db *gorm.DB) {
			db.Model(&model.AuditLogDO{}).Where("seq = ?", 7).Update("result", "failure")
		}, BreakHash, 7},
		{"modify time", func(db *gorm.DB) {
			db.Model(&model.AuditLogDO{}).Where("seq = ?", 3).Update("ts_micro", 1)
		}, BreakHash, 3},
		{"delete middle", func(db *gorm.DB) { db.Where("seq = ?", 5).Delete(&model.AuditLogDO{}) }, BreakGap, 5},
		{"delete tail", func(db *gorm.DB) { db.Where("seq >= ?", 9).Delete(&model.AuditLogDO{}) }, BreakTail, 9},
		{"relink row", func(db *gorm.DB) {
			var r model.AuditLogDO
			db.Where("seq = ?", 4).Take(&r)
			r.Reason = "forged"
			r.PrevHash = GenesisHash
			r.Hash = ComputeHash(&r)
			db.Save(&r)
		}, BreakPrevHash, 4},
		{"recompute rest of chain", func(db *gorm.DB) {
			var rows []model.AuditLogDO
			db.Order("seq").Find(&rows)
			prev := GenesisHash
			for i := range rows {
				if rows[i].Seq == 6 {
					rows[i].Reason = "forged"
				}
				rows[i].PrevHash = prev
				rows[i].Hash = ComputeHash(&rows[i])
				prev = rows[i].Hash
				db.Save(&rows[i])
			}
			db.Model(&model.AuditChainTailDO{}).Where("id = 1").Update("hash", prev)
		}, "", 0}, // undetectable without anchors (see TestAnchorDetectsRewrite)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openDB(t, "")
			s := newSvc(t, db, Options{})
			for i := 0; i < 10; i++ {
				s.Record(context.Background(), ev(i))
			}
			flush(t, s)
			if r := verify(t, s.Store(), VerifyOptions{}); !r.Valid {
				t.Fatalf("pre-tamper invalid: %+v", r)
			}
			tc.tamper(db)
			r := verify(t, s.Store(), VerifyOptions{})
			if tc.kind == "" {
				if !r.Valid {
					t.Fatalf("expected internally consistent chain: %+v", r)
				}
				return
			}
			if r.Valid || r.BreakKind != tc.kind || r.BrokenSeq != tc.seq {
				t.Fatalf("want %s at %d, got %+v", tc.kind, tc.seq, r)
			}
		})
	}
}

func TestAnchorDetectsRewrite(t *testing.T) {
	db := openDB(t, "")
	anchorFile := filepath.Join(t.TempDir(), "anchors", "audit.anchor")
	s := newSvc(t, db, Options{AnchorFile: anchorFile, AnchorInterval: time.Hour})
	for i := 0; i < 10; i++ {
		s.Record(context.Background(), ev(i))
	}
	flush(t, s)
	if err := s.AnchorNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	anchors, err := ReadAnchorFile(anchorFile)
	if err != nil || len(anchors) == 0 || anchors[len(anchors)-1].Seq != 10 {
		t.Fatalf("anchors %v %v", anchors, err)
	}
	if r := verify(t, s.Store(), VerifyOptions{Anchors: anchors}); !r.Valid || r.AnchorsChecked < 1 {
		t.Fatalf("%+v", r)
	}
	// Full-chain rewrite (attacker with DB access): consistent, but the
	// anchor disagrees.
	var rows []model.AuditLogDO
	db.Order("seq").Find(&rows)
	prev := GenesisHash
	for i := range rows {
		if rows[i].Seq == 2 {
			rows[i].Actor = "mallory"
		}
		rows[i].PrevHash = prev
		rows[i].Hash = ComputeHash(&rows[i])
		prev = rows[i].Hash
		db.Save(&rows[i])
	}
	db.Model(&model.AuditChainTailDO{}).Where("id = 1").Update("hash", prev)
	if r := verify(t, s.Store(), VerifyOptions{Anchors: anchors}); r.Valid || r.BreakKind != BreakAnchor || r.BrokenSeq != 10 {
		t.Fatalf("rewrite not detected: %+v", r)
	}
	// Truncation below an anchor.
	db.Where("seq >= 8").Delete(&model.AuditLogDO{})
	db.Model(&model.AuditChainTailDO{}).Where("id = 1").Updates(map[string]any{"seq": 7})
	if r := verify(t, s.Store(), VerifyOptions{Anchors: anchors}); r.Valid || r.BreakKind != BreakAnchorTrunc {
		t.Fatalf("truncation not detected: %+v", r)
	}
}

func TestConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	// Two "processes": independent DB handles and services on one database.
	db1, db2 := openDB(t, path), openDB(t, path)
	s1 := newSvc(t, db1, Options{BatchSize: 7})
	s2 := newSvc(t, db2, Options{BatchSize: 5})
	var wg sync.WaitGroup
	const perG, gs = 40, 8
	for g := 0; g < gs; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			svc := s1
			if g%2 == 1 {
				svc = s2
			}
			for i := 0; i < perG; i++ {
				svc.Record(context.Background(), ev(g*1000+i))
			}
		}(g)
	}
	// Direct store appends racing with the services.
	st := NewStore(openDB(t, path))
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			row := rowFromEvent(fmt.Sprintf("direct-%d", i), ev(i))
			if _, err := st.Append(context.Background(), []model.AuditLogDO{row}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	flush(t, s1)
	flush(t, s2)
	var n int64
	db1.Model(&model.AuditLogDO{}).Count(&n)
	if n != perG*gs+20 {
		t.Fatalf("rows %d, want %d", n, perG*gs+20)
	}
	if r := verify(t, st, VerifyOptions{}); !r.Valid || r.Checked != n {
		t.Fatalf("%+v", r)
	}
}

func TestSpoolReplay(t *testing.T) {
	db := openDB(t, "")
	spoolDir := filepath.Join(t.TempDir(), "spool")
	s := newSvc(t, db, Options{SpoolDir: spoolDir})
	// Simulate a DB outage.
	if err := db.Migrator().DropTable(&model.AuditLogDO{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		s.Record(context.Background(), ev(i))
	}
	flush(t, s) // write fails -> spooled; replay fails too
	files, _ := filepath.Glob(filepath.Join(spoolDir, "audit-spool*"))
	if len(files) == 0 {
		t.Fatal("expected spool files")
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Recorded after shutdown -> spool too.
	s.Record(context.Background(), ev(99))

	// DB back; a torn line (crash mid-write) must be skipped.
	if err := db.AutoMigrate(&model.AuditLogDO{}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(spoolDir, spoolActive), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	_, _ = f.WriteString(`{"eventId":"torn","act`)
	_ = f.Close()

	// "Restart": a new service replays the spool on startup.
	s2 := newSvc(t, db, Options{SpoolDir: spoolDir})
	flush(t, s2)
	var n int64
	db.Model(&model.AuditLogDO{}).Count(&n)
	if n != 21 {
		t.Fatalf("replayed %d rows, want 21", n)
	}
	if left, _ := filepath.Glob(filepath.Join(spoolDir, spoolReplayPrefix+"*")); len(left) != 0 {
		t.Fatalf("spool not cleaned: %v", left)
	}
	if r := verify(t, s2.Store(), VerifyOptions{}); !r.Valid || r.Checked != 21 {
		t.Fatalf("%+v", r)
	}
	// Idempotent: replaying the same events again inserts nothing.
	var rows []model.AuditLogDO
	db.Find(&rows)
	if err := s2.spool.append(rows); err != nil {
		t.Fatal(err)
	}
	flush(t, s2)
	db.Model(&model.AuditLogDO{}).Count(&n)
	if n != 21 {
		t.Fatalf("duplicate replay: %d rows", n)
	}
}

func TestQueueFullSpoolsAndBoundedSpoolDrops(t *testing.T) {
	db := openDB(t, "")
	spoolDir := filepath.Join(t.TempDir(), "spool")
	s, err := New(db, nil, Options{QueueSize: 2, SpoolDir: spoolDir, SpoolMaxBytes: 2000})
	if err != nil {
		t.Fatal(err)
	}
	// not started: queue fills up after 2 events, then spool, then drop.
	for i := 0; i < 20; i++ {
		s.Record(context.Background(), ev(i))
	}
	if len(s.ch) != 2 {
		t.Fatalf("queue len %d", len(s.ch))
	}
	rows, _, _ := readSpoolFile(filepath.Join(spoolDir, spoolActive))
	if len(rows) == 0 || len(rows) >= 18 {
		t.Fatalf("spooled %d rows; expected bounded spool", len(rows))
	}
	s.Start()
	flush(t, s)
	_ = s.Close(context.Background())
	var n int64
	db.Model(&model.AuditLogDO{}).Count(&n)
	if n != int64(2+len(rows)) {
		t.Fatalf("persisted %d, want %d", n, 2+len(rows))
	}
}

func TestRetentionArchivalBoundary(t *testing.T) {
	db := openDB(t, "")
	archive := filepath.Join(t.TempDir(), "archive")
	s := newSvc(t, db, Options{})
	now := time.Now()
	for i := 0; i < 30; i++ {
		e := ev(i)
		e.Time = now.AddDate(0, 0, -1200).Add(time.Duration(i) * time.Minute) // expired
		if i >= 20 {
			e.Time = now.Add(-time.Duration(30-i) * time.Minute) // recent
		}
		s.Record(context.Background(), e)
	}
	// An old-looking event after a recent one must stop the prefix.
	late := ev(100)
	late.Time = now.AddDate(0, 0, -2000)
	s.Record(context.Background(), late)
	flush(t, s)

	n, err := s.Store().ArchiveExpired(context.Background(), archive, 1095, now)
	if err != nil || n != 20 {
		t.Fatalf("archived %d, %v", n, err)
	}
	var left int64
	db.Model(&model.AuditLogDO{}).Count(&left)
	if left != 11 {
		t.Fatalf("remaining rows %d", left)
	}
	r := verify(t, s.Store(), VerifyOptions{ArchiveDir: archive, RequireArchives: true})
	if !r.Valid || r.Segments != 1 || r.ArchivedThrough != 20 || r.FirstSeq != 21 || r.ArchivesVerified != 1 {
		t.Fatalf("%+v", r)
	}
	// Idempotent / nothing more to archive.
	if n, err := s.Store().ArchiveExpired(context.Background(), archive, 1095, now); err != nil || n != 0 {
		t.Fatalf("second run %d %v", n, err)
	}
	// Appends continue on top of the boundary.
	for i := 0; i < 5; i++ {
		s.Record(context.Background(), ev(200+i))
	}
	flush(t, s)
	if r := verify(t, s.Store(), VerifyOptions{ArchiveDir: archive}); !r.Valid || r.Checked != 16 {
		t.Fatalf("%+v", r)
	}
	// Deleting the first live row after the boundary is detected.
	db.Where("seq = 21").Delete(&model.AuditLogDO{})
	if r := verify(t, s.Store(), VerifyOptions{}); r.Valid || r.BreakKind != BreakGap || r.BrokenSeq != 21 {
		t.Fatalf("%+v", r)
	}
}

func TestRetentionArchiveTamperAndSegmentTamper(t *testing.T) {
	db := openDB(t, "")
	archive := filepath.Join(t.TempDir(), "archive")
	s := newSvc(t, db, Options{})
	now := time.Now()
	for i := 0; i < 5; i++ {
		e := ev(i)
		e.Time = now.AddDate(-4, 0, 0)
		s.Record(context.Background(), e)
	}
	flush(t, s)
	if n, err := s.Store().ArchiveExpired(context.Background(), archive, 1095, now); err != nil || n != 5 {
		t.Fatalf("%d %v", n, err)
	}
	// All rows archived: tail must still match the segment boundary.
	if r := verify(t, s.Store(), VerifyOptions{ArchiveDir: archive}); !r.Valid || r.Checked != 0 {
		t.Fatalf("%+v", r)
	}
	files, _ := filepath.Glob(filepath.Join(archive, "audit-segment-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	data, _ := os.ReadFile(files[0])
	_ = os.WriteFile(files[0], append(data, ' '), 0o600)
	if r := verify(t, s.Store(), VerifyOptions{ArchiveDir: archive}); r.Valid || r.BreakKind != BreakArchive {
		t.Fatalf("archive tamper not detected: %+v", r)
	}
	_ = os.WriteFile(files[0], data, 0o600)
	db.Model(&model.AuditSegmentDO{}).Where("seg_seq = 1").Update("last_seq", 4)
	if r := verify(t, s.Store(), VerifyOptions{}); r.Valid || r.BreakKind != BreakSegment {
		t.Fatalf("segment tamper not detected: %+v", r)
	}
}

func TestRecordNilSafe(t *testing.T) {
	SetRecorder(nil)
	Record(context.Background(), Event{Action: "x"}) // no recorder: no-op
	var s *Service
	s.Record(context.Background(), Event{})
	SetRecorder(panicRecorder{})
	Record(nil, Event{Action: "x"}) //nolint:staticcheck // nil ctx must be tolerated
	SetRecorder(nil)
}

type panicRecorder struct{}

func (panicRecorder) Record(context.Context, Event) { panic("boom") }

func TestListFilters(t *testing.T) {
	db := openDB(t, "")
	s := newSvc(t, db, Options{})
	for i := 0; i < 10; i++ {
		e := ev(i)
		if i%2 == 0 {
			e.Result = ResultDenied
			e.Action = "authz.denied"
		}
		s.Record(context.Background(), e)
	}
	flush(t, s)
	rows, total, err := s.Store().List(context.Background(), Query{Result: ResultDenied, Action: "authz", Size: 3})
	if err != nil || total != 5 || len(rows) != 3 || rows[0].Seq < rows[1].Seq {
		t.Fatalf("%d %d %v", total, len(rows), err)
	}
	_, total, _ = s.Store().List(context.Background(), Query{Action: "%"})
	if total != 0 {
		t.Fatalf("LIKE wildcard not escaped: %d", total)
	}
}

func TestAnchorHTTP(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.Header.Get("Authorization")+" "+string(b))
		mu.Unlock()
	}))
	defer srv.Close()
	db := openDB(t, "")
	s := newSvc(t, db, Options{AnchorURL: srv.URL, AnchorToken: "t0k", AnchorInterval: time.Hour})
	s.Record(context.Background(), ev(1))
	flush(t, s)
	if err := s.AnchorNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	last := got[len(got)-1]
	if !strings.HasPrefix(last, "Bearer t0k ") || !strings.Contains(last, `"seq":1`) {
		t.Fatalf("anchor request %q", last)
	}
}
