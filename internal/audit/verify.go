package audit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// Verification failure kinds.
const (
	BreakSegment     = "segment_broken"     // archived segment chain inconsistent
	BreakArchive     = "archive_broken"     // archive file missing/modified
	BreakGap         = "seq_gap"            // row(s) deleted in the middle
	BreakPrevHash    = "prev_hash_mismatch" // row not linked to its predecessor
	BreakHash        = "hash_mismatch"      // row content modified
	BreakTail        = "tail_mismatch"      // rows missing at the end / tail tampered
	BreakAnchor      = "anchor_mismatch"    // external anchor disagrees with the chain
	BreakAnchorTrunc = "anchor_truncation"  // chain shorter than an external anchor
)

// Anchor is one externally stored chain tail (see audit.anchor.*).
type Anchor struct {
	Time      string `json:"time"`
	Seq       int64  `json:"seq"`
	Hash      string `json:"hash"`
	Node      string `json:"node,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
}

// VerifyOptions tunes Verify.
type VerifyOptions struct {
	// Anchors are external tail records to check against the chain.
	Anchors []Anchor
	// ArchiveDir, when set, verifies each archived segment file (content
	// hash + internal chain). RequireArchives reports a missing file as
	// broken; otherwise missing files are only counted.
	ArchiveDir      string
	RequireArchives bool
	BatchSize       int
}

// VerifyResult is the outcome of a chain verification.
type VerifyResult struct {
	Valid            bool   `json:"valid"`
	Algorithm        string `json:"algorithm"`
	Checked          int64  `json:"checked"`
	FirstSeq         int64  `json:"firstSeq"`
	LastSeq          int64  `json:"lastSeq"`
	TailSeq          int64  `json:"tailSeq"`
	TailHash         string `json:"tailHash"`
	Segments         int    `json:"segments"`
	ArchivedThrough  int64  `json:"archivedThrough"`
	ArchivesVerified int    `json:"archivesVerified"`
	ArchivesMissing  int    `json:"archivesMissing"`
	AnchorsChecked   int    `json:"anchorsChecked"`
	BrokenSeq        int64  `json:"brokenSeq,omitempty"`
	BrokenID         int64  `json:"brokenId,omitempty"`
	BreakKind        string `json:"breakKind,omitempty"`
	Reason           string `json:"reason,omitempty"`
	VerifiedAt       string `json:"verifiedAt"`
}

func (r *VerifyResult) fail(kind string, seq, id int64, format string, a ...any) *VerifyResult {
	r.Valid = false
	r.BreakKind = kind
	r.BrokenSeq = seq
	r.BrokenID = id
	r.Reason = fmt.Sprintf(format, a...)
	return r
}

// Verify re-computes the whole chain (archived segment boundaries, every
// remaining row, the tail row and optional external anchors) and reports the
// first broken link. Rows appended after the tail snapshot are ignored.
func (s *Store) Verify(ctx context.Context, opt VerifyOptions) (*VerifyResult, error) {
	if opt.BatchSize <= 0 {
		opt.BatchSize = 1000
	}
	res := &VerifyResult{Valid: true, Algorithm: HashAlgorithm, VerifiedAt: time.Now().UTC().Format(time.RFC3339)}

	tail, err := s.Tail(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		tail = model.AuditChainTailDO{Seq: 0, Hash: GenesisHash}
	} else if err != nil {
		return nil, err
	}
	res.TailSeq, res.TailHash = tail.Seq, tail.Hash

	// 1. Archived segments.
	var segs []model.AuditSegmentDO
	if err := s.db.WithContext(ctx).Order("seg_seq ASC").Find(&segs).Error; err != nil {
		return nil, err
	}
	res.Segments = len(segs)
	expSeq, expPrev := int64(1), GenesisHash
	prevSeg := GenesisHash
	segBoundary := map[int64]string{} // lastSeq -> lastHash
	for i := range segs {
		sg := &segs[i]
		switch {
		case sg.SegSeq != int64(i+1):
			return res.fail(BreakSegment, sg.FirstSeq, 0, "segment #%d has seg_seq %d (segment deleted or reordered)", i+1, sg.SegSeq), nil
		case !hashEqual(sg.SegHash, ComputeSegmentHash(sg)):
			return res.fail(BreakSegment, sg.FirstSeq, 0, "segment %d hash mismatch (segment record modified)", sg.SegSeq), nil
		case sg.PrevSegHash != prevSeg:
			return res.fail(BreakSegment, sg.FirstSeq, 0, "segment %d not linked to previous segment", sg.SegSeq), nil
		case sg.FirstSeq != expSeq || sg.FirstPrevHash != expPrev:
			return res.fail(BreakSegment, sg.FirstSeq, 0, "segment %d boundary mismatch: expected first seq %d", sg.SegSeq, expSeq), nil
		case sg.LastSeq < sg.FirstSeq || sg.RecordCount != sg.LastSeq-sg.FirstSeq+1:
			return res.fail(BreakSegment, sg.FirstSeq, 0, "segment %d has inconsistent range/count", sg.SegSeq), nil
		}
		if opt.ArchiveDir != "" {
			if err := VerifyArchiveFile(opt.ArchiveDir, sg); err != nil {
				if errors.Is(err, errArchiveMissing) && !opt.RequireArchives {
					res.ArchivesMissing++
				} else {
					return res.fail(BreakArchive, sg.FirstSeq, 0, "segment %d archive: %v", sg.SegSeq, err), nil
				}
			} else {
				res.ArchivesVerified++
			}
		}
		prevSeg = sg.SegHash
		expSeq, expPrev = sg.LastSeq+1, sg.LastHash
		segBoundary[sg.LastSeq] = sg.LastHash
	}
	res.ArchivedThrough = expSeq - 1

	// 2. Live rows, keyset-paged by seq up to the tail snapshot.
	anchors := append([]Anchor(nil), opt.Anchors...)
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].Seq < anchors[j].Seq })
	wantAnchor := map[int64][]string{}
	for _, a := range anchors {
		if a.Seq > tail.Seq {
			res.AnchorsChecked++
			return res.fail(BreakAnchorTrunc, a.Seq, 0, "external anchor at seq %d (%s) is beyond the chain tail %d: records were truncated", a.Seq, a.Time, tail.Seq), nil
		}
		if a.Seq <= res.ArchivedThrough {
			if h, ok := segBoundary[a.Seq]; ok {
				res.AnchorsChecked++
				if !hashEqual(h, a.Hash) {
					return res.fail(BreakAnchor, a.Seq, 0, "external anchor at seq %d disagrees with archived segment boundary", a.Seq), nil
				}
			}
			continue // inside an archived segment: covered by archive verification
		}
		if a.Seq == 0 {
			continue
		}
		wantAnchor[a.Seq] = append(wantAnchor[a.Seq], a.Hash)
	}

	cursor := res.ArchivedThrough
	for {
		var rows []model.AuditLogDO
		if err := s.db.WithContext(ctx).Where("seq > ? AND seq <= ?", cursor, tail.Seq).
			Order("seq ASC").Limit(opt.BatchSize).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			r := &rows[i]
			if res.Checked == 0 {
				res.FirstSeq = r.Seq
			}
			if r.Seq != expSeq {
				return res.fail(BreakGap, expSeq, r.ID, "record seq %d missing (next present seq %d): record(s) deleted", expSeq, r.Seq), nil
			}
			if !hashEqual(r.PrevHash, expPrev) {
				return res.fail(BreakPrevHash, r.Seq, r.ID, "record seq %d prev_hash does not match the previous record", r.Seq), nil
			}
			if !hashEqual(r.Hash, ComputeHash(r)) {
				return res.fail(BreakHash, r.Seq, r.ID, "record seq %d content does not match its hash (modified)", r.Seq), nil
			}
			if hs, ok := wantAnchor[r.Seq]; ok {
				for _, h := range hs {
					res.AnchorsChecked++
					if !hashEqual(h, r.Hash) {
						return res.fail(BreakAnchor, r.Seq, r.ID, "external anchor at seq %d disagrees with the chain (chain rewritten)", r.Seq), nil
					}
				}
			}
			res.Checked++
			res.LastSeq = r.Seq
			expSeq, expPrev = r.Seq+1, r.Hash
		}
		cursor = rows[len(rows)-1].Seq
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if expSeq-1 != tail.Seq || !hashEqual(expPrev, tail.Hash) {
		return res.fail(BreakTail, expSeq, 0, "chain ends at seq %d but the tail records seq %d: trailing records deleted or tail tampered", expSeq-1, tail.Seq), nil
	}
	return res, nil
}
