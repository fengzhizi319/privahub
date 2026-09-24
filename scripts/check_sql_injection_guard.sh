#!/usr/bin/env bash
# =============================================================================
# check_sql_injection_guard.sh — GORM SQL-injection guard (fail-closed).
# Ported from PrivShield scripts/check_sql_injection_guard.sh, adapted to GORM.
#
# WHY: every GORM method whose first argument is raw SQL text — Raw, Exec,
# Where, Or, Not, Having, Joins, Order, Group, Select, Distinct — is an
# injection sink when that text is built at runtime. Placeholders (`?`) bind
# VALUES only; they cannot protect identifiers (column / ORDER BY names).
# Today the code base is clean: values use `?`, sort keys go through fixed
# maps (modelSortColumns, ...). Nothing enforced that — this gate does.
#
# WHAT (positive invariants, not "no concatenation ever"):
#   A. fmt.Sprintf / strings.Join / strings.Builder output must not be the SQL
#      argument of a GORM sink.
#   B. Request-derived expressions (req./request./c.Query/c.Param/c.PostForm/
#      c.GetHeader/filter./params./input.) must not appear in the SQL text
#      argument (they belong after the first comma, as bound args).
#   C. A string literal must not open a quoted SQL value and then be
#      concatenated:  Where("name = '" + v + "'")
#   D. Any other runtime-built SQL text (x + "...") in a sink must be listed
#      in scripts/sqli-allowlist.txt with a justification (e.g. the column
#      name comes from a whitelist map). New entries show up in review.
#      The same applies to a bare variable passed to a text-only sink
#      (Raw/Exec/Order/Group/Having/Joins).
#
# Only non-test .go files under cmd/ internal/ pkg/ are scanned.
# =============================================================================
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib/preflight.sh
source scripts/lib/preflight.sh
require_cmd python3 "apt-get install -y python3" || exit 1

echo "SQL injection guard (GORM)"

python3 - <<'PY'
import pathlib, re, sys

ROOTS = ["cmd", "internal", "pkg"]
ALLOW_FILE = pathlib.Path("scripts/sqli-allowlist.txt")
SINK = re.compile(r'\.(Raw|Exec|Where|Or|Not|Having|Joins|Order|Group|Select|Distinct)\(')
REQ = re.compile(r'\b(req|request|filter|params?|input|body|query)\.[A-Za-z_]|\bc\.(Query|Param|PostForm|DefaultQuery|GetHeader|GetQuery)\(')
BUILD = re.compile(r'^\s*(fmt\.Sprintf|strings\.Join|fmt\.Sprint|sb\.String|b\.String|buf\.String)\b')
QUOTED_VALUE = re.compile(r'''"[^"]*(=|<|>|LIKE|IN\s*\()\s*'"\s*\+''', re.I)
# sinks whose first argument can only be SQL text (never a struct/map condition)
TEXT_ONLY = {"Raw", "Exec", "Order", "Group", "Having", "Joins"}
STRLIT = re.compile(r'"(?:[^"\\]|\\.)*"|`[^`]*`')

def first_arg(src, start):
    """Return text of the first call argument beginning at src[start] (just after '(')."""
    depth, i, out = 0, start, []
    in_str = None
    while i < len(src):
        ch = src[i]
        if in_str:
            out.append(ch)
            if ch == '\\' and in_str == '"':
                out.append(src[i+1]); i += 2; continue
            if ch == in_str:
                in_str = None
        elif ch in '"`':
            in_str = ch; out.append(ch)
        elif ch in '([{':
            depth += 1; out.append(ch)
        elif ch in ')]}':
            if depth == 0:
                break
            depth -= 1; out.append(ch)
        elif ch == ',' and depth == 0:
            break
        else:
            out.append(ch)
        i += 1
    return ''.join(out).strip()

allow = []
if ALLOW_FILE.exists():
    for ln in ALLOW_FILE.read_text(encoding="utf-8").splitlines():
        ln = ln.strip()
        if not ln or ln.startswith('#'):
            continue
        parts = [p.strip() for p in ln.split('|')]
        if len(parts) != 3 or not parts[2]:
            print(f"  [FAIL] malformed allowlist entry (need 'path | snippet | reason'): {ln}")
            sys.exit(1)
        allow.append(parts)
used = set()

files = [p for r in ROOTS for p in pathlib.Path(r).rglob("*.go") if not p.name.endswith("_test.go")]
if not files:
    print("  [FAIL] no Go source files found - gate would pass vacuously"); sys.exit(1)

bad, sinks = [], 0
for p in files:
    src = p.read_text(encoding="utf-8")
    if 'gorm' not in src and '.Raw(' not in src and '.Exec(' not in src:
        continue
    for m in SINK.finditer(src):
        arg = first_arg(src, m.end())
        if not arg:
            continue
        sinks += 1
        line_no = src.count('\n', 0, m.start()) + 1
        line = src.splitlines()[line_no - 1]
        if 'sqli-guard: ok' in line:
            continue
        # pure string literal (possibly concatenated literals only) is safe
        stripped = STRLIT.sub('', arg).replace('+', '').strip()
        is_literal = stripped == ''
        why = None
        if BUILD.search(arg) and any(k in arg.upper() for k in ('SELECT','WHERE','ORDER','GROUP','UPDATE','DELETE','INSERT','=','LIKE',' IN')) or re.match(r'^\s*fmt\.Sprintf\b', arg):
            why = "A: SQL text built with fmt.Sprintf/strings.Join - use ? placeholders / whitelist maps"
        elif REQ.search(STRLIT.sub('""', arg)):
            why = "B: request-derived value in SQL text - pass it as a bound ? argument"
        elif QUOTED_VALUE.search(arg):
            why = "C: literal opens a quoted SQL value then concatenates - use ?"
        elif not is_literal and ('+' in arg or (m.group(1) in TEXT_ONLY and re.fullmatch(r'[A-Za-z_][\w.]*', arg))):
            hit = [a for a in allow if a[0] == str(p) and a[1] in arg]
            if hit:
                used.add(tuple(hit[0])); continue
            why = "D: runtime-built SQL text not in scripts/sqli-allowlist.txt"
        if why:
            bad.append((p, line_no, arg[:140], why))

stale = [a for a in allow if tuple(a) not in used]
if stale:
    for a in stale:
        print(f"  [FAIL] stale allowlist entry (no longer matches code): {a[0]} | {a[1]}")
if bad:
    print(f"  [FAIL] {len(bad)} suspicious SQL sink(s):")
    for p, n, a, why in bad:
        print(f"      {p}:{n}  [{why}]\n        {a}")
if bad or stale:
    sys.exit(1)
if sinks == 0:
    print("  [FAIL] no GORM SQL sinks found - gate would pass vacuously"); sys.exit(1)
print(f"  [OK]   {sinks} GORM SQL sinks in {len(files)} files: no Sprintf/request data in SQL text; {len(used)} allowlisted identifier concatenations")
PY
rc=$?
if [ "$rc" -ne 0 ]; then
	cat >&2 <<'MSG'

SQL injection guard FAILED.
  values      -> db.Where("col = ?", v)   (never inside the SQL string)
  identifiers -> map the request key through a fixed map[string]string of
                 column names, then add a justified entry to
                 scripts/sqli-allowlist.txt  (path | code snippet | reason)
  one-off     -> trailing comment `// sqli-guard: ok <reason>` on that line
MSG
	exit 1
fi
echo "SQL injection guard passed."
