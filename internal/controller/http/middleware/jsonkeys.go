package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"
)

// Parameter-confusion hardening.
//
// encoding/json (used by every gin ShouldBindJSON) matches an object key to a
// struct field exactly OR case-insensitively (Unicode simple folding), and the
// LAST matching key wins. CaseKeysRequest additionally adds camelCase /
// snake_case twins. A body such as {"projectId":"mine","projectid":"victim"}
// or {"projectId":"mine","project_id":"victim"} therefore let an authorization
// guard read one id while the handler bound another. To make "what the guard
// checked" and "what the handler uses" provably identical, every JSON object
// key is reduced to a canonical name (folded like encoding/json, underscores
// removed) and objects holding two keys with the same canonical name but
// different values — literal duplicates included — are rejected before any
// guard or handler runs.

// ErrDuplicateJSONKey reports an object with two keys of the same canonical name.
var ErrDuplicateJSONKey = errors.New("duplicate or aliased JSON object key")

// ErrInvalidJSON reports a body that is not exactly one JSON value.
var ErrInvalidJSON = errors.New("invalid JSON body")

// foldRune returns the smallest rune of r's simple-fold orbit (identical to
// encoding/json's foldRune, so canonical names agree with its key matching).
func foldRune(r rune) rune {
	for {
		r2 := unicode.SimpleFold(r)
		if r2 <= r {
			return r2
		}
		r = r2
	}
}

// CanonicalJSONKey folds k the way encoding/json matches keys case-
// insensitively and drops underscores, so "projectId", "project_id",
// "PROJECTID" and "Project_Id" share one canonical name.
func CanonicalJSONKey(k string) string {
	var b []byte
	for i := 0; i < len(k); {
		c := k[i]
		if c < utf8.RuneSelf {
			i++
			if c == '_' {
				continue
			}
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			b = append(b, c)
			continue
		}
		r, n := utf8.DecodeRuneInString(k[i:])
		i += n
		b = utf8.AppendRune(b, foldRune(r))
	}
	return string(b)
}

type keyFrame struct {
	obj       bool
	expectKey bool
	keys      map[string]keySpan // canonical -> first occurrence
	// pending key whose value is being read
	pendingKey   string
	pendingCanon string
	pendingStart int64
}

type keySpan struct {
	raw   string
	value []byte // raw value bytes (including the leading ':' / spaces)
}

// sameJSON reports whether two raw JSON values (possibly preceded by ':' and
// whitespace) are identical after compaction.
func sameJSON(a, b []byte) bool {
	trim := func(v []byte) []byte { return bytes.TrimLeft(v, " \t\r\n:") }
	var ca, cb bytes.Buffer
	if json.Compact(&ca, trim(a)) != nil || json.Compact(&cb, trim(b)) != nil {
		return false
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

// CheckJSONKeys scans body token by token (map decoding would hide literal
// duplicates) and returns ErrDuplicateJSONKey when any object, at any depth,
// contains two keys with the same CanonicalJSONKey whose values differ, or
// ErrInvalidJSON when the body is not exactly one well-formed JSON value.
//
// Aliases carrying the identical value (e.g. {"nodeId":"a","node_id":"a"},
// typically an object echoed back from a CaseKeysResponse-rewritten
// response) are accepted: every binding sees the same value, so they cannot
// confuse a guard and a handler.
func CheckJSONKeys(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var stack []*keyFrame
	var dupErr error
	valueDone := func() {
		n := len(stack)
		if n == 0 || !stack[n-1].obj {
			return
		}
		top := stack[n-1]
		top.expectKey = true
		end := dec.InputOffset()
		if top.pendingStart < 0 || end > int64(len(body)) {
			return
		}
		val := body[top.pendingStart:end]
		if prev, dup := top.keys[top.pendingCanon]; dup {
			if dupErr == nil && !sameJSON(prev.value, val) {
				dupErr = fmt.Errorf("%w: %q and %q", ErrDuplicateJSONKey, prev.raw, top.pendingKey)
			}
		} else {
			top.keys[top.pendingCanon] = keySpan{raw: top.pendingKey, value: val}
		}
		top.pendingStart = -1
	}
	started := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			if !started || len(stack) != 0 {
				return ErrInvalidJSON
			}
			return dupErr
		}
		if err != nil {
			return ErrInvalidJSON
		}
		if started && len(stack) == 0 {
			return ErrInvalidJSON // trailing data after the first value
		}
		started = true
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, &keyFrame{obj: true, expectKey: true, keys: map[string]keySpan{}, pendingStart: -1})
			case '[':
				stack = append(stack, &keyFrame{pendingStart: -1})
			default: // '}' or ']'
				stack = stack[:len(stack)-1]
				valueDone()
			}
		case string:
			if n := len(stack); n > 0 && stack[n-1].obj && stack[n-1].expectKey {
				top := stack[n-1]
				top.pendingKey, top.pendingCanon = t, CanonicalJSONKey(t)
				top.pendingStart = dec.InputOffset()
				top.expectKey = false
				continue
			}
			valueDone()
		default:
			valueDone()
		}
	}
}

// looksLikeJSON reports whether body (trimmed) starts like a JSON object/array.
func looksLikeJSON(body []byte) bool {
	t := bytes.TrimSpace(body)
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

// canonicalFieldValues returns the values of every top-level key of obj whose
// canonical name equals CanonicalJSONKey(field).
func canonicalFieldValues(obj map[string]json.RawMessage, field string) []json.RawMessage {
	want := CanonicalJSONKey(field)
	var out []json.RawMessage
	for k, v := range obj {
		if CanonicalJSONKey(k) == want {
			out = append(out, v)
		}
	}
	return out
}
