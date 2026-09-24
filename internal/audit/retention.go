package audit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/sm3"
	"gorm.io/gorm"
)

// MinProductionRetentionDays is the minimum retention (3 years) accepted in
// production when audit.retention_days is set (0 = keep forever).
const MinProductionRetentionDays = 1095

// maxSegmentRows bounds one archive segment.
const maxSegmentRows = 100000

var errArchiveMissing = errors.New("archive file missing")

// segmentHeader / segmentFooter frame an archive file. Rows keep their own
// seq/prev_hash/hash, so the file is an independently verifiable piece of the
// chain; its SM3 is stored in audit_segment.file_sm3.
type segmentHeader struct {
	Type          string `json:"type"`
	Version       int    `json:"version"`
	Algorithm     string `json:"algorithm"`
	FirstSeq      int64  `json:"firstSeq"`
	LastSeq       int64  `json:"lastSeq"`
	FirstPrevHash string `json:"firstPrevHash"`
	Count         int64  `json:"count"`
}

type segmentFooter struct {
	Type     string `json:"type"`
	LastHash string `json:"lastHash"`
	Count    int64  `json:"count"`
}

func segmentFileName(first, last int64) string {
	return fmt.Sprintf("audit-segment-%012d-%012d.jsonl", first, last)
}

// ArchiveExpired exports and deletes records older than retentionDays, as
// contiguous chain segments. The deletion happens in the same transaction as
// the insertion of the segment boundary record, so verification of the
// remaining chain keeps working. A broken chain is never archived (that would
// destroy the evidence). retentionDays <= 0 is a no-op.
func (s *Store) ArchiveExpired(ctx context.Context, archiveDir string, retentionDays int, now time.Time) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	if archiveDir == "" {
		return 0, errors.New("audit: archive_dir is required for retention")
	}
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return 0, err
	}
	cutoff := now.AddDate(0, 0, -retentionDays).UnixMicro()
	var total int64
	for {
		n, err := s.archiveOne(ctx, archiveDir, cutoff)
		total += n
		if err != nil || n == 0 {
			return total, err
		}
	}
}

func (s *Store) archiveOne(ctx context.Context, dir string, cutoff int64) (int64, error) {
	last, err := s.LastSegment(ctx)
	if err != nil {
		return 0, err
	}
	start, prev, segSeq, prevSeg := int64(1), GenesisHash, int64(1), GenesisHash
	if last != nil {
		start, prev, segSeq, prevSeg = last.LastSeq+1, last.LastHash, last.SegSeq+1, last.SegHash
	}
	tail, err := s.Tail(ctx)
	if err != nil {
		return 0, err
	}
	// Contiguous prefix of expired rows: everything below the first row
	// that is still within the retention window.
	end := tail.Seq
	var firstKeep struct{ Seq *int64 }
	if err := s.db.WithContext(ctx).Model(&model.AuditLogDO{}).Select("MIN(seq) AS seq").
		Where("seq >= ? AND ts_micro >= ?", start, cutoff).Scan(&firstKeep).Error; err != nil {
		return 0, err
	}
	if firstKeep.Seq != nil {
		end = *firstKeep.Seq - 1
	}
	if end-start+1 > maxSegmentRows {
		end = start + maxSegmentRows - 1
	}
	if end < start {
		return 0, nil
	}
	var rows []model.AuditLogDO
	if err := s.db.WithContext(ctx).Where("seq BETWEEN ? AND ?", start, end).Order("seq ASC").Find(&rows).Error; err != nil {
		return 0, err
	}
	// Refuse to archive a broken chain.
	exp, expPrev := start, prev
	for i := range rows {
		r := &rows[i]
		if r.Seq != exp || r.PrevHash != expPrev || !hashEqual(r.Hash, ComputeHash(r)) {
			return 0, fmt.Errorf("audit: chain broken at seq %d, refusing to archive", exp)
		}
		exp, expPrev = r.Seq+1, r.Hash
	}
	if exp-1 != end {
		return 0, fmt.Errorf("audit: chain broken at seq %d, refusing to archive", exp)
	}

	// Export.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	count := int64(len(rows))
	_ = enc.Encode(segmentHeader{Type: "segment_header", Version: 1, Algorithm: HashAlgorithm,
		FirstSeq: start, LastSeq: end, FirstPrevHash: prev, Count: count})
	for i := range rows {
		if err := enc.Encode(&rows[i]); err != nil {
			return 0, err
		}
	}
	_ = enc.Encode(segmentFooter{Type: "segment_footer", LastHash: expPrev, Count: count})
	name := segmentFileName(start, end)
	if err := writeFileDurable(filepath.Join(dir, name), buf.Bytes()); err != nil {
		return 0, err
	}
	seg := model.AuditSegmentDO{
		SegSeq: segSeq, FirstSeq: start, LastSeq: end, RecordCount: count,
		FirstPrevHash: prev, LastHash: expPrev, FileName: name, FileSM3: sm3.SumHex(buf.Bytes()),
		ArchiveMicro: time.Now().UTC().UnixMicro(), PrevSegHash: prevSeg,
	}
	seg.SegHash = ComputeSegmentHash(&seg)

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&seg).Error; err != nil {
			return err
		}
		res := tx.Where("seq BETWEEN ? AND ?", start, end).Delete(&model.AuditLogDO{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != count {
			return fmt.Errorf("audit: archived %d rows but deleted %d", count, res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	metricArchived.Add(float64(count))
	return count, nil
}

func writeFileDurable(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
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
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	syncDir(filepath.Dir(path))
	return nil
}

// VerifyArchiveFile checks an archived segment file against its segment
// record: file SM3, header/footer, and the row chain inside.
func VerifyArchiveFile(dir string, seg *model.AuditSegmentDO) error {
	data, err := os.ReadFile(filepath.Join(dir, filepath.Base(seg.FileName)))
	if errors.Is(err, os.ErrNotExist) {
		return errArchiveMissing
	}
	if err != nil {
		return err
	}
	if !hashEqual(sm3.SumHex(data), seg.FileSM3) {
		return errors.New("file content hash mismatch")
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	if !sc.Scan() {
		return errors.New("empty archive")
	}
	var h segmentHeader
	if err := json.Unmarshal(sc.Bytes(), &h); err != nil || h.Type != "segment_header" {
		return errors.New("bad header")
	}
	if h.FirstSeq != seg.FirstSeq || h.LastSeq != seg.LastSeq || h.FirstPrevHash != seg.FirstPrevHash {
		return errors.New("header does not match segment record")
	}
	exp, prev := h.FirstSeq, h.FirstPrevHash
	var n int64
	for sc.Scan() {
		line := sc.Bytes()
		if bytes.Contains(line, []byte(`"type":"segment_footer"`)) {
			var f segmentFooter
			if err := json.Unmarshal(line, &f); err != nil {
				return errors.New("bad footer")
			}
			if f.Count != n || n != seg.RecordCount || !hashEqual(f.LastHash, prev) || !hashEqual(prev, seg.LastHash) {
				return errors.New("footer does not match chain")
			}
			return nil
		}
		var r model.AuditLogDO
		if err := json.Unmarshal(line, &r); err != nil {
			return fmt.Errorf("bad row after seq %d", exp-1)
		}
		if r.Seq != exp || r.PrevHash != prev || !hashEqual(r.Hash, ComputeHash(&r)) {
			return fmt.Errorf("chain broken at seq %d", exp)
		}
		exp, prev = r.Seq+1, r.Hash
		n++
	}
	return errors.New("missing footer")
}
