package audit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/sqlutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// errTailMoved reports a lost compare-and-set on the chain tail (another
// writer advanced it); the append is retried.
var errTailMoved = errors.New("audit: chain tail moved concurrently")

const tailRowID = 1

// Store is the database side of the audit hash chain.
type Store struct {
	db *gorm.DB
}

// NewStore wraps db.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// DB returns the underlying database.
func (s *Store) DB() *gorm.DB { return s.db }

// lockTail adds SELECT ... FOR UPDATE on dialects that support row locks
// (MySQL/Postgres). SQLite serializes writers at the database level; the
// compare-and-set on the tail row + unique seq arbitrate the rest.
func (s *Store) lockTail(tx *gorm.DB) *gorm.DB {
	switch tx.Dialector.Name() {
	case "mysql", "postgres":
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx
}

// EnsureTail creates the chain tail row if it does not exist yet (derived
// from the last row / last archived segment, or genesis).
func (s *Store) EnsureTail(ctx context.Context) error {
	var n int64
	if err := s.db.WithContext(ctx).Model(&model.AuditChainTailDO{}).Where("id = ?", tailRowID).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tail := model.AuditChainTailDO{ID: tailRowID, Seq: 0, Hash: GenesisHash, UpdateMicro: time.Now().UnixMicro()}
	var last model.AuditLogDO
	err := s.db.WithContext(ctx).Order("seq DESC").Take(&last).Error
	switch {
	case err == nil:
		tail.Seq, tail.Hash = last.Seq, last.Hash
	case errors.Is(err, gorm.ErrRecordNotFound):
		if seg, err := s.LastSegment(ctx); err != nil {
			return err
		} else if seg != nil {
			tail.Seq, tail.Hash = seg.LastSeq, seg.LastHash
		}
	default:
		return err
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&tail).Error
}

// Tail returns the current chain tail.
func (s *Store) Tail(ctx context.Context) (model.AuditChainTailDO, error) {
	var t model.AuditChainTailDO
	err := s.db.WithContext(ctx).Where("id = ?", tailRowID).Take(&t).Error
	return t, err
}

// LastSegment returns the newest archived segment or nil.
func (s *Store) LastSegment(ctx context.Context) (*model.AuditSegmentDO, error) {
	var seg model.AuditSegmentDO
	err := s.db.WithContext(ctx).Order("seg_seq DESC").Take(&seg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &seg, nil
}

// Append chains and inserts rows (EventID and content set; Seq/PrevHash/Hash
// are assigned here). Rows whose EventID already exists are skipped, which
// makes spool replay idempotent. It returns the number of inserted rows.
//
// Concurrency: the tail row is locked (MySQL) and advanced with a
// compare-and-set in the same transaction; seq is unique. A lost race rolls
// back and is retried, so the chain never forks.
func (s *Store) Append(ctx context.Context, rows []model.AuditLogDO) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	var inserted int
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		inserted, err = s.appendOnce(ctx, rows)
		if err == nil {
			return inserted, nil
		}
		if ctx.Err() != nil {
			return 0, err
		}
		backoff := min(time.Duration(5<<attempt)*time.Millisecond, 500*time.Millisecond)
		// Jitter only de-synchronises retrying writers; it is not security-relevant.
		time.Sleep(backoff/2 + time.Duration(rand.Int64N(int64(backoff/2)+1))) //nolint:gosec // G404: retry jitter, not a secret
	}
	return 0, err
}

// AppendEvents synchronously appends events to the chain, bypassing the
// asynchronous queue and spool. Offline tools (cmd/privahub-admin) use it
// with a Store built on their own transaction so that the audited change and
// its audit record commit — or roll back — together.
func (s *Store) AppendEvents(ctx context.Context, events ...Event) (int, error) {
	rows := make([]model.AuditLogDO, 0, len(events))
	for _, e := range events {
		if e.Time.IsZero() {
			e.Time = time.Now()
		}
		if e.ActorType == "" {
			e.ActorType = ActorSystem
		}
		if e.Result == "" {
			e.Result = ResultSuccess
		}
		rows = append(rows, rowFromEvent(uuid.NewString(), e))
	}
	return s.Append(ctx, rows)
}

func (s *Store) appendOnce(ctx context.Context, in []model.AuditLogDO) (int, error) {
	inserted := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "sqlite" {
			// Take the SQLite write lock first (like BEGIN IMMEDIATE) so
			// concurrent processes wait on busy_timeout instead of failing
			// a read->write upgrade on a stale WAL snapshot.
			if err := tx.Exec("UPDATE audit_chain_tail SET update_micro = update_micro WHERE id = ?", tailRowID).Error; err != nil {
				return err
			}
		}
		var tail model.AuditChainTailDO
		if err := s.lockTail(tx).Where("id = ?", tailRowID).Take(&tail).Error; err != nil {
			return fmt.Errorf("read audit chain tail: %w", err)
		}
		ids := make([]string, 0, len(in))
		for i := range in {
			ids = append(ids, in[i].EventID)
		}
		var existing []string
		if err := tx.Model(&model.AuditLogDO{}).Where("event_id IN ?", ids).Pluck("event_id", &existing).Error; err != nil {
			return err
		}
		seen := make(map[string]bool, len(existing)+len(in))
		for _, id := range existing {
			seen[id] = true
		}
		out := make([]model.AuditLogDO, 0, len(in))
		seq, prev := tail.Seq, tail.Hash
		for _, r := range in {
			if seen[r.EventID] {
				continue
			}
			seen[r.EventID] = true
			r.ID = 0
			seq++
			r.Seq = seq
			r.PrevHash = prev
			r.Hash = ComputeHash(&r)
			prev = r.Hash
			out = append(out, r)
		}
		if len(out) == 0 {
			return nil
		}
		if err := tx.CreateInBatches(out, 100).Error; err != nil {
			return err
		}
		res := tx.Model(&model.AuditChainTailDO{}).
			Where("id = ? AND seq = ? AND hash = ?", tailRowID, tail.Seq, tail.Hash).
			Updates(map[string]any{"seq": seq, "hash": prev, "update_micro": time.Now().UnixMicro()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errTailMoved
		}
		inserted = len(out)
		return nil
	})
	return inserted, err
}

// Query filters the audit list.
type Query struct {
	StartMicro   int64  // inclusive, 0 = unbounded
	EndMicro     int64  // exclusive, 0 = unbounded
	Actor        string // exact
	Action       string // substring
	Result       string // exact
	ResourceType string // exact
	ResourceID   string // exact
	IP           string // exact
	TraceID      string // exact
	Page         int    // 1-based
	Size         int    // default 20, max 200
}

// List returns one page of records (newest first) and the total count.
func (s *Store) List(ctx context.Context, q Query) ([]model.AuditLogDO, int64, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size <= 0 {
		q.Size = 20
	}
	if q.Size > 200 {
		q.Size = 200
	}
	db := s.db.WithContext(ctx).Model(&model.AuditLogDO{})
	if q.StartMicro > 0 {
		db = db.Where("ts_micro >= ?", q.StartMicro)
	}
	if q.EndMicro > 0 {
		db = db.Where("ts_micro < ?", q.EndMicro)
	}
	for col, v := range map[string]string{
		"actor": q.Actor, "result": q.Result, "resource_type": q.ResourceType,
		"resource_id": q.ResourceID, "ip": q.IP, "trace_id": q.TraceID,
	} {
		if v = strings.TrimSpace(v); v != "" {
			db = db.Where(col+" = ?", v)
		}
	}
	if a := strings.TrimSpace(q.Action); a != "" {
		db = db.Where(`action LIKE ? ESCAPE '!'`, sqlutil.Contains(a))
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.AuditLogDO
	err := db.Order("seq DESC").Offset((q.Page - 1) * q.Size).Limit(q.Size).Find(&rows).Error
	return rows, total, err
}
