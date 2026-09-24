package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

const (
	spoolActive       = "audit-spool.jsonl"
	spoolReplayPrefix = "audit-spool.replay-"
)

var errSpoolFull = errors.New("audit spool full")

// spool is a local append-only, fsync'd JSON-lines file holding events that
// could not be written to the database. It is replayed (idempotently, by
// event id) into the chain on startup and periodically.
type spool struct {
	mu       sync.Mutex
	dir      string
	maxBytes int64
}

func newSpool(dir string, maxBytes int64) (*spool, error) {
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	sp := &spool{dir: dir, maxBytes: maxBytes}
	metricSpoolBytes.Set(float64(sp.size()))
	return sp, nil
}

func (sp *spool) size() int64 {
	var total int64
	entries, _ := os.ReadDir(sp.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "audit-spool") {
			if info, err := e.Info(); err == nil {
				total += info.Size()
			}
		}
	}
	return total
}

// append durably writes rows (fsync) to the active spool file.
func (sp *spool) append(rows []model.AuditLogDO) error {
	if sp == nil {
		return errors.New("audit spool disabled")
	}
	var buf []byte
	for i := range rows {
		b, err := json.Marshal(&rows[i])
		if err != nil {
			return err
		}
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	cur := sp.size()
	if sp.maxBytes > 0 && cur+int64(len(buf)) > sp.maxBytes {
		return errSpoolFull
	}
	f, err := os.OpenFile(filepath.Join(sp.dir, spoolActive), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	metricSpoolBytes.Set(float64(cur + int64(len(buf))))
	return nil
}

// rotate moves the active file aside so appends continue in a fresh file
// while the rotated ones are replayed; it returns all pending replay files
// in order.
func (sp *spool) rotate() ([]string, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	active := filepath.Join(sp.dir, spoolActive)
	if info, err := os.Stat(active); err == nil && info.Size() > 0 {
		name := fmt.Sprintf("%s%020d.jsonl", spoolReplayPrefix, time.Now().UnixNano())
		if err := os.Rename(active, filepath.Join(sp.dir, name)); err != nil {
			return nil, err
		}
		syncDir(sp.dir)
	}
	entries, err := os.ReadDir(sp.dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), spoolReplayPrefix) {
			files = append(files, filepath.Join(sp.dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// readSpoolFile decodes a spool file; torn/corrupt lines (crash during a
// write) are skipped and counted.
func readSpoolFile(path string) ([]model.AuditLogDO, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var rows []model.AuditLogDO
	bad := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var r model.AuditLogDO
		if err := json.Unmarshal(line, &r); err != nil || r.EventID == "" {
			bad++
			continue
		}
		r.ID, r.Seq, r.PrevHash, r.Hash = 0, 0, "", ""
		rows = append(rows, r)
	}
	return rows, bad, sc.Err()
}

func (sp *spool) remove(path string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	_ = os.Remove(path)
	syncDir(sp.dir)
	metricSpoolBytes.Set(float64(sp.size()))
}

// syncDir fsyncs a directory so renames/creates are durable.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
