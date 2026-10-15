package dao

import (
	"net/url"
	"regexp"
	"strings"
)

// The sqlite dialector (gorm.io/driver/sqlite) uses github.com/mattn/go-sqlite3,
// whose DSN options are "_busy_timeout=5000", "_journal_mode=WAL", ... The
// documented/configured DSN uses the modernc.org/sqlite form
// "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", which mattn silently
// ignores: the database stayed in rollback-journal (DELETE) mode and a
// deferred read->write transaction upgrade failed immediately with
// "database is locked" (e.g. Kuscia job watch events racing the poller).

// mattnPragmas maps PRAGMA names to the equivalent mattn DSN option.
var mattnPragmas = map[string]string{
	"busy_timeout":             "_busy_timeout",
	"journal_mode":             "_journal_mode",
	"synchronous":              "_synchronous",
	"foreign_keys":             "_foreign_keys",
	"locking_mode":             "_locking_mode",
	"auto_vacuum":              "_auto_vacuum",
	"cache_size":               "_cache_size",
	"case_sensitive_like":      "_case_sensitive_like",
	"defer_foreign_keys":       "_defer_foreign_keys",
	"ignore_check_constraints": "_ignore_check_constraints",
	"query_only":               "_query_only",
	"recursive_triggers":       "_recursive_triggers",
	"secure_delete":            "_secure_delete",
}

var pragmaRe = regexp.MustCompile(`^\s*([A-Za-z_]+)\s*\(\s*([^)]*?)\s*\)\s*$`)

// NormalizeSQLiteDSN rewrites "_pragma=name(value)" options into mattn
// options (explicit mattn options win) and defaults "_txlock=immediate", so
// every transaction takes the write lock up front and waits on busy_timeout
// instead of failing a lock upgrade. Unknown pragmas are returned so the
// caller can report them.
func NormalizeSQLiteDSN(dsn string) (string, []string) {
	path, rawQuery, _ := strings.Cut(dsn, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return dsn, nil
	}
	var unknown []string
	for _, p := range q["_pragma"] {
		m := pragmaRe.FindStringSubmatch(p)
		if m == nil {
			unknown = append(unknown, p)
			continue
		}
		opt, ok := mattnPragmas[strings.ToLower(m[1])]
		if !ok {
			unknown = append(unknown, p)
			continue
		}
		if q.Get(opt) == "" {
			q.Set(opt, m[2])
		}
	}
	q.Del("_pragma")
	if q.Get("_txlock") == "" {
		q.Set("_txlock", "immediate")
	}
	return path + "?" + q.Encode(), unknown
}
