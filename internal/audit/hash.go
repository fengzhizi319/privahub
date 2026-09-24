package audit

import (
	"crypto/subtle"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/sm3"
)

// HashAlgorithm labels the only hash convention produced and accepted:
// unkeyed SM3 over a length-prefixed canonical pre-image. There is no
// fallback to another convention (a verifier that accepts "any of" several
// conventions opens a downgrade path; see PrivShield no_hmac_design.md).
//
// The chain is unkeyed on purpose: anyone (e.g. a regulator) can re-compute
// it without a secret. Consequently the chain alone only proves internal
// consistency; resistance against an attacker with DB write access comes from
// external anchoring (audit.anchor.*), which stores the tail hash outside the
// attacker's reach.
const HashAlgorithm = "SM3:privahub-audit:v1"

// GenesisHash is the prev_hash of the first chain record.
var GenesisHash = strings.Repeat("0", 64)

// Column size limits (must match model.AuditLogDO) — values are truncated
// before hashing so what is stored is exactly what was hashed.
const (
	maxActor        = 128
	maxActorType    = 16
	maxOwnerID      = 64
	maxIP           = 64
	maxAction       = 255
	maxResourceType = 64
	maxResourceID   = 255
	maxResult       = 16
	maxReason       = 512
	maxTraceID      = 128
	maxExtraKey     = 64
	maxExtraValue   = 512
	maxExtraEntries = 32
)

// truncate cuts s to at most n bytes on a UTF-8 rune boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// canonicalJoin encodes each field as "<byte length>:<content>" so no field
// content can move across a field boundary (PrivShield v2 lesson).
func canonicalJoin(fields ...string) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteByte(':')
		b.WriteString(f)
	}
	return b.String()
}

// canonicalExtra renders Extra deterministically (sorted keys, JSON).
func canonicalExtra(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > maxExtraEntries {
		keys = keys[:maxExtraEntries]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[truncate(k, maxExtraKey)] = truncate(m[k], maxExtraValue)
	}
	// encoding/json sorts map keys, so the output is canonical.
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// TruncateTime normalizes an event time: UTC, microsecond precision (MySQL
// DATETIME(6)/PG precision; hashing sub-microsecond digits breaks
// verification after a DB round trip — PrivShield bug lesson).
func TruncateTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// ComputeHash returns the chain hash of a stored row (all content fields plus
// Seq and PrevHash). The row's own Hash/ID are not part of the pre-image.
func ComputeHash(r *model.AuditLogDO) string {
	payload := canonicalJoin(
		HashAlgorithm,
		strconv.FormatInt(r.Seq, 10),
		r.EventID,
		strconv.FormatInt(r.TsMicro, 10),
		r.Actor, r.ActorType, r.OwnerID, r.IP,
		r.Action, r.ResourceType, r.ResourceID,
		r.Result, r.Reason, r.TraceID, r.Extra,
		r.PrevHash,
	)
	return sm3.SumHex([]byte(payload))
}

// ComputeSegmentHash is the hash of an archived segment record.
func ComputeSegmentHash(s *model.AuditSegmentDO) string {
	payload := canonicalJoin(
		"SM3:privahub-audit-segment:v1",
		strconv.FormatInt(s.SegSeq, 10),
		strconv.FormatInt(s.FirstSeq, 10),
		strconv.FormatInt(s.LastSeq, 10),
		strconv.FormatInt(s.RecordCount, 10),
		s.FirstPrevHash, s.LastHash, s.FileName, s.FileSM3,
		strconv.FormatInt(s.ArchiveMicro, 10),
		s.PrevSegHash,
	)
	return sm3.SumHex([]byte(payload))
}

func hashEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// rowFromEvent builds the (unchained) row content of an event; Seq,
// PrevHash and Hash are filled by the chain writer.
func rowFromEvent(id string, e Event) model.AuditLogDO {
	return model.AuditLogDO{
		EventID:      truncate(id, 64),
		TsMicro:      TruncateTime(e.Time).UnixMicro(),
		Actor:        truncate(e.Actor, maxActor),
		ActorType:    truncate(e.ActorType, maxActorType),
		OwnerID:      truncate(e.OwnerID, maxOwnerID),
		IP:           truncate(e.IP, maxIP),
		Action:       truncate(e.Action, maxAction),
		ResourceType: truncate(e.ResourceType, maxResourceType),
		ResourceID:   truncate(e.ResourceID, maxResourceID),
		Result:       truncate(e.Result, maxResult),
		Reason:       truncate(e.Reason, maxReason),
		TraceID:      truncate(e.TraceID, maxTraceID),
		Extra:        canonicalExtra(e.Extra),
	}
}
