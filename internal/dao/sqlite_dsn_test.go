package dao

import (
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/pkg/config"
)

func TestNormalizeSQLiteDSN(t *testing.T) {
	cases := []struct {
		in      string
		want    map[string]string
		unknown int
	}{
		{"privahub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
			map[string]string{"_busy_timeout": "5000", "_journal_mode": "WAL", "_txlock": "immediate"}, 0},
		{"a.db", map[string]string{"_txlock": "immediate"}, 0},
		// explicit mattn options win over the translated pragma
		{"a.db?_busy_timeout=100&_pragma=busy_timeout(5000)&_txlock=deferred",
			map[string]string{"_busy_timeout": "100", "_txlock": "deferred"}, 0},
		{"a.db?_pragma=foreign_keys(1)&_pragma=mmap_size(1)",
			map[string]string{"_foreign_keys": "1", "_txlock": "immediate"}, 1},
	}
	for _, tc := range cases {
		out, unknown := NormalizeSQLiteDSN(tc.in)
		path, raw, _ := strings.Cut(out, "?")
		if path != strings.SplitN(tc.in, "?", 2)[0] {
			t.Errorf("%s: path changed to %q", tc.in, path)
		}
		q, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		if q.Has("_pragma") {
			t.Errorf("%s: _pragma left in %q", tc.in, out)
		}
		for k, v := range tc.want {
			if q.Get(k) != v {
				t.Errorf("%s: %s = %q want %q (%s)", tc.in, k, q.Get(k), v, out)
			}
		}
		if len(unknown) != tc.unknown {
			t.Errorf("%s: unknown = %v", tc.in, unknown)
		}
	}
}

// Regression: the configured "_pragma=" DSN was ignored by mattn/go-sqlite3
// (journal_mode stayed DELETE) and concurrent read->write transactions (job
// watch events vs. the poller) failed with "database is locked".
func TestNewDB_SQLitePragmasAppliedAndConcurrentWritesWait(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "t.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := NewDB(&config.DatabaseConfig{Driver: "sqlite", DSN: dsn}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil || !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q (%v), want wal", mode, err)
	}
	var timeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&timeout).Error; err != nil || timeout != 5000 {
		t.Fatalf("busy_timeout = %d (%v)", timeout, err)
	}

	type counter struct {
		ID int
		N  int
	}
	if err := db.AutoMigrate(&counter{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&counter{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	const workers, rounds = 8, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				// read, then write in the same transaction (lock upgrade)
				err := db.Transaction(func(tx *gorm.DB) error {
					var c counter
					if err := tx.First(&c, 1).Error; err != nil {
						return err
					}
					return tx.Model(&counter{}).Where("id = ?", 1).Update("n", c.N+1).Error
				})
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent transaction failed: %v", err)
	}
	var c counter
	if err := db.First(&c, 1).Error; err != nil {
		t.Fatal(err)
	}
	if c.N != workers*rounds {
		t.Fatalf("lost updates: n = %d want %d", c.N, workers*rounds)
	}
}
