#!/usr/bin/env bash
# =============================================================================
# check_base_image_pins.sh — container base images must be reproducible.
#
# Rules for every Dockerfile in the repo (tracked or new):
#   1. Every FROM (or ARG *_IMAGE= default used by FROM) must reference an
#      image pinned by digest (@sha256:...). A tag can be re-pointed upstream
#      at any time (supply-chain risk) and makes Trivy results irreproducible.
#   2. No ':latest' and no untagged images.
#   3. The golang builder must not freeze a distro minor version
#      (golang:1.25-alpine3.21 stops receiving Go patch releases once that
#      Alpine version is dropped — ported from PrivShield).
#   Stage references (FROM builder / FROM ${ARG}) are resolved via ARG defaults.
#
# To refresh a digest:  docker buildx imagetools inspect <image:tag>
# =============================================================================
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib/preflight.sh
source scripts/lib/preflight.sh
require_cmd python3 "apt-get install -y python3" || exit 1
require_cmd git "apt-get install -y git" || exit 1

mapfile -t files < <(git ls-files --cached --others --exclude-standard -- '*Dockerfile*' ':!:web/**' | sort -u)
require_nonempty "Dockerfiles" "${files[*]}" || exit 1

echo "Base image pin gate"
python3 - "${files[@]}" <<'PY'
import re, sys
bad = 0
for f in sys.argv[1:]:
    try:
        text = open(f, encoding="utf-8").read()
    except FileNotFoundError:
        continue
    args, stages = {}, set()
    for n, line in enumerate(text.splitlines(), 1):
        s = line.strip()
        m = re.match(r'ARG\s+(\w+)=(\S+)', s)
        if m:
            args[m.group(1)] = m.group(2).strip('"')
            continue
        m = re.match(r'FROM\s+(?:--platform=\S+\s+)?(\S+)(?:\s+AS\s+(\S+))?', s, re.I)
        if not m:
            continue
        ref, alias = m.group(1), m.group(2)
        if alias:
            stages.add(alias.lower())
        if ref.lower() in stages or ref == "scratch":
            continue
        am = re.fullmatch(r'\$\{?(\w+)\}?', ref)
        if am:
            ref = args.get(am.group(1), "")
            if not ref:
                print(f"  [FAIL] {f}:{n}: FROM uses ARG {am.group(1)} without a default"); bad += 1; continue
        problems = []
        if '@sha256:' not in ref:
            problems.append("not pinned by digest (@sha256:...)")
        name = ref.split('@')[0]
        tag = name.rsplit(':', 1)[1] if ':' in name.split('/')[-1] else ''
        if tag in ('', 'latest'):
            problems.append("missing tag or ':latest'")
        if re.match(r'(docker\.io/library/)?golang:[^@]*-(alpine|bookworm|bullseye|trixie)[0-9]', ref):
            problems.append("golang builder pins a distro minor version (freezes Go patches)")
        for p in problems:
            print(f"  [FAIL] {f}:{n}: {ref}: {p}"); bad += 1
if bad:
    print("\n  refresh digests with: docker buildx imagetools inspect <image:tag>")
    sys.exit(1)
print(f"  [OK]   all FROM images pinned by digest ({len(sys.argv)-1} Dockerfile(s))")
PY
