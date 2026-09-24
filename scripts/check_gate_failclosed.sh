#!/usr/bin/env bash
# =============================================================================
# check_gate_failclosed.sh — meta-gate: gates must not pass silently.
# (Ported from PrivShield scripts/check_gate_failclosed.sh and extended.)
#
# The most dangerous gate failure is not "red", it is "could not run, so it
# was green". Typical ways that happens:
#   * `command -v tool || exit 0`, `[SKIP] ... exit 0` when a dependency is
#     missing, `except ImportError: sys.exit(0)`;
#   * CI steps with `continue-on-error: true` or `|| true` after a scanner;
#   * a Make recipe line prefixed with `-` (errors ignored);
#   * a gate script that exists but is wired into neither `make check` nor CI.
#
# This script enforces:
#   1. every scripts/check_*.sh sources scripts/lib/preflight.sh and contains
#      no soft-degradation pattern;
#   2. every scripts/check_*.sh is invoked by the Makefile (make check/sec);
#   3. every gate invoked by the Makefile is also run by CI (directly or via
#      a make target that runs it);
#   4. CI workflows contain no `continue-on-error: true` and no `|| true` /
#      `|| exit 0` on security tool / gate invocations;
#   5. Makefile gate recipes do not ignore errors (`-cmd`, `|| true`).
# =============================================================================
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib/preflight.sh
source scripts/lib/preflight.sh
require_cmd python3 "apt-get install -y python3" || exit 1

echo "Gate fail-closed meta check"
python3 - <<'PY'
import glob, pathlib, re, sys

errs = []
mk = pathlib.Path("Makefile").read_text(encoding="utf-8")
wf_files = sorted(glob.glob(".github/workflows/*.y*ml"))
if not wf_files:
    errs.append(".github/workflows: no CI workflow found")
wf = {f: pathlib.Path(f).read_text(encoding="utf-8") for f in wf_files}
ci_text = "\n".join(wf.values())

gates = sorted(glob.glob("scripts/check_*.sh"))
if not gates:
    print("  [FAIL] no scripts/check_*.sh gates found"); sys.exit(1)

SOFT = [
    (re.compile(r'(command -v|which|type)\s+\S+[^\n]*\|\|\s*exit\s+0'), "`command -v x || exit 0`"),
    (re.compile(r'if\s+!\s+(?:command -v|which)\s+\S+[^\n]*\n(?:[^\n]*\n){0,6}?\s*exit\s+0\s*\n', re.M), "command -v failure branch exits 0"),
    (re.compile(r'except\s+ImportError[^\n]*:\n(?:[ \t]+[^\n]*\n){0,4}?[ \t]+sys\.exit\(0\)'), "except ImportError -> sys.exit(0)"),
    (re.compile(r'\[SKIP\][^\n]*\n(?:[^\n]*\n){0,2}?\s*exit\s+0'), "[SKIP] followed by exit 0"),
]
for g in gates:
    src = pathlib.Path(g).read_text(encoding="utf-8")
    if g != "scripts/check_gate_failclosed.sh":
        for rx, what in SOFT:
            if rx.search(src):
                errs.append(f"{g}: soft degradation ({what})")
    if "scripts/lib/preflight.sh" not in src:
        errs.append(f"{g}: does not source scripts/lib/preflight.sh (dependency checks must hard-fail)")
    if g not in mk:
        errs.append(f"{g}: not invoked by the Makefile (orphan gate never runs in make check)")

# gates run by CI: directly, or through a make target whose recipe runs them
def make_targets_running(gate):
    out = []
    for m in re.finditer(r'^([A-Za-z0-9_.-]+):[^\n=]*\n((?:\t[^\n]*\n)+)', mk, re.M):
        if gate in m.group(2):
            out.append(m.group(1))
    return out

def ci_runs_target(t, depth=0):
    if re.search(r'\bmake\b[^\n]*\b' + re.escape(t) + r'\b', ci_text):
        return True
    if depth > 3:
        return False
    # target reached through a parent aggregate target (e.g. check: ... sqli-check ...)
    for m in re.finditer(r'^([A-Za-z0-9_.-]+):([^\n=]*)$', mk, re.M):
        if re.search(r'(^|\s)' + re.escape(t) + r'(\s|$)', m.group(2)) and ci_runs_target(m.group(1), depth + 1):
            return True
    return False

for g in gates:
    if g in ci_text:
        continue
    if not any(ci_runs_target(t) for t in make_targets_running(g)):
        errs.append(f"{g}: not run by CI (neither directly nor via a make target)")

TOOLS = r'(golangci-lint|govulncheck|gitleaks|trivy|syft|hadolint|actionlint|pnpm audit|scripts/check_|make )'
for f, text in wf.items():
    for n, line in enumerate(text.splitlines(), 1):
        if line.lstrip().startswith('#'):
            continue
        if re.search(r'continue-on-error:\s*true', line):
            errs.append(f"{f}:{n}: continue-on-error: true (gate failures would be ignored)")
        if re.search(TOOLS, line) and re.search(r'\|\|\s*(true|exit 0|:)\b', line):
            errs.append(f"{f}:{n}: '|| true' on a gate/scanner invocation")
for n, line in enumerate(mk.splitlines(), 1):
    if line.startswith("\t-") and re.search(TOOLS + r'|go (test|vet|run)', line):
        errs.append(f"Makefile:{n}: recipe ignores errors with '-' prefix")
    if line.startswith("\t") and re.search(TOOLS, line) and re.search(r'\|\|\s*(true|exit 0)', line):
        errs.append(f"Makefile:{n}: '|| true' on a gate invocation")

if errs:
    print(f"  [FAIL] {len(errs)} fail-open pattern(s):")
    for e in errs: print(f"      {e}")
    sys.exit(1)
print(f"  [OK]   {len(gates)} gate script(s): all hard-fail on missing deps, wired into make and CI; no fail-open CI steps")
PY
