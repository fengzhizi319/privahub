#!/usr/bin/env bash
# =============================================================================
# check_log_redaction.sh — credentials must never reach the logs (fail-closed).
# Ported from PrivShield scripts/check_log_redaction.sh, adapted to zap.
#
# WHY: logs travel much further than the database (stdout collectors, SIEM,
# support bundles, developer terminals). A single
#     log.Info("login", zap.String("password", req.Password))
# compiles, passes every test and looks like "extra context" in review.
#
# HOW: for every logger call (log./logger./s.log./zap.L()/zap.S() ... Info/Warn/
# Error/Debug/Fatal/Panic/DPanic/With/Infof/Infow...) and every zap field
# constructor (zap.String/Any/Reflect/Stringer/ByteString/Binary/Strings), the
# full (multi-line) argument list is collected, ALL string literals are blanked
# out, and only the remaining VALUE expressions are matched. So the key name
# "token" or the message text "invalid token" never trigger; `tok`,
# `req.Password`, `cfg.Auth.JWTSecret`, `c.GetHeader("Authorization")` do.
#
# Exemption: append `// log-redaction: ok <reason>` to the offending line.
# =============================================================================
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1
# shellcheck source=scripts/lib/preflight.sh
source scripts/lib/preflight.sh
require_cmd python3 "apt-get install -y python3" || exit 1

echo "Log redaction guard"

python3 - <<'PY'
import pathlib, re, sys

ROOTS = ["cmd", "internal", "pkg"]
CALL = re.compile(
    r'(?:\b(?:log|logger|lg|l|s\.log|s\.logger|h\.log|h\.logger|c\.log|d\.log|e\.log|a\.log|zap\.L\(\)|zap\.S\(\))'
    r'\.(?:Info|Warn|Error|Debug|Fatal|Panic|DPanic|With)\w*'
    r'|\bzap\.(?:String|Strings|Any|Reflect|Stringer|ByteString|Binary)'
    r'|\blog\.(?:Print|Printf|Println|Fatal|Fatalf|Panicf))\(')
STRLIT = re.compile(r'"(?:[^"\\]|\\.)*"|`[^`]*`|\'(?:[^\'\\]|\\.)*\'')
# credential-bearing identifiers: whole word (case-insensitive) ...
CRED_WORD = re.compile(
    r'\b(?:token|tok|rawToken|tokenStr|tokenString|accessToken|refreshToken|bearer|'
    r'password|passwd|pwd|secret|apiKey|api_key|privateKey|privKey|priv_key|aesKey|'
    r'aes_key|jwtSecret|signingKey|credential|credentials|cookie)\b', re.I)
# ... or a camelCase suffix (case-sensitive: AdminPassword, JWTSecret, InnerToken)
CRED_SUFFIX = re.compile(r'\b\w+(?:Password|Passwd|Secret|Token|ApiKey|APIKey|PrivateKey|AESKey|AesKey|SigningKey|Credential)\b')
# a hit is harmless when only its presence/length is logged
PRESENCE = re.compile(r'^\s*(?:!=|==)|^\s*\)?\s*>\s*0')
# raw auth headers: checked on the ORIGINAL text (the header name is a literal)
AUTHHDR = re.compile(r'(?:GetHeader|Header\.Get|Header\.Values)\(\s*"(?i:authorization|cookie|x-privahub-inner-token|token|x-auth-token|proxy-authorization)"\s*\)'
                     r'|Request\.Header\b(?!\.Get)|\.Cookie\(')

def call_text(src, start):
    depth, i, in_str = 1, start, None
    while i < len(src) and depth:
        ch = src[i]
        if in_str:
            if ch == '\\' and in_str != '`': i += 2; continue
            if ch == in_str: in_str = None
        elif ch in '"`\'': in_str = ch
        elif ch == '(': depth += 1
        elif ch == ')': depth -= 1
        i += 1
    return src[start:i-1]

files = [p for r in ROOTS for p in pathlib.Path(r).rglob("*.go") if not p.name.endswith("_test.go")]
if not files:
    print("  [FAIL] no Go source files found - gate would pass vacuously"); sys.exit(1)

bad, calls = [], 0
for p in files:
    src = p.read_text(encoding="utf-8")
    lines = src.splitlines()
    for m in CALL.finditer(src):
        calls += 1
        body = call_text(src, m.end())
        first = src.count('\n', 0, m.start())
        last = first + body.count('\n')
        if any('log-redaction: ok' in lines[k] for k in range(first, min(last + 1, len(lines)))):
            continue
        vals = STRLIT.sub('""', body)
        # drop bare "err" and generic identifiers that end with Token-ish words but are types/ids
        hit = None
        for cm in list(CRED_WORD.finditer(vals)) + list(CRED_SUFFIX.finditer(vals)):
            w = cm.group(0)
            if re.search(r'(ID|Id|Name|File|Path|Hash|Len|Type|Expiry|TTL|Count|Prefix)$', w):
                continue
            after = vals[cm.end():]
            if after.lstrip().startswith('('):          # function/method call, e.g. NewToken(...)
                continue
            if PRESENCE.match(after) or vals[:cm.start()].rstrip().endswith('len('):
                continue
            hit = w; break
        if hit:
            bad.append((p, first + 1, f"credential-like value '{hit}'", lines[first].strip()))
        elif AUTHHDR.search(body):
            bad.append((p, first + 1, "raw Authorization/Cookie header", lines[first].strip()))

seen, uniq = set(), []
for b in bad:
    if (b[0], b[1]) not in seen:
        seen.add((b[0], b[1])); uniq.append(b)
bad = uniq
if bad:
    print(f"  [FAIL] {len(bad)} log statement(s) may write credentials:")
    for p, n, why, text in bad:
        print(f"      {p}:{n}  [{why}]\n        {text[:150]}")
    sys.exit(1)
if calls == 0:
    print("  [FAIL] no logger calls found - gate would pass vacuously"); sys.exit(1)
print(f"  [OK]   {calls} logger calls / zap fields in {len(files)} files: no credential values or raw auth headers")
PY
rc=$?
if [ "$rc" -ne 0 ]; then
	cat >&2 <<'MSG'

Log redaction guard FAILED.
  credentials -> log an identifier (user name, key version, token id) or a
                 short hash prefix, never the value.
  headers     -> never log Authorization / Cookie / the whole Header map.
  exemption   -> `// log-redaction: ok <reason>` on the line (reviewed in diff).
MSG
	exit 1
fi
echo "Log redaction guard passed."
