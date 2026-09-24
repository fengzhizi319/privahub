#!/usr/bin/env bash
# =============================================================================
# check_fuzz_smoke.sh — run every Go fuzz target briefly (smoke, not campaign).
#
# Discovers `func FuzzXxx(f *testing.F)` in *_test.go and runs each one with
#   go test -run '^$' -fuzz '^FuzzXxx$' -fuzztime $FUZZTIME <pkg>
# (go test accepts only one -fuzz target per package invocation).
# Seed corpora always run as part of plain `go test`; this adds mutation.
#
# FUZZTIME: default 30s (CI). Use FUZZTIME=5s for a quick local pass.
# Having zero fuzz targets is a FAILURE: security-sensitive parsers (auth
# middleware, request decoding) are expected to carry at least one.
# =============================================================================
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib/preflight.sh
source scripts/lib/preflight.sh
require_cmd go "https://go.dev/dl/" || exit 1

FUZZTIME="${FUZZTIME:-30s}"
targets=$(grep -rEl --include='*_test.go' '^func Fuzz[A-Za-z0-9_]*\(f \*testing\.F\)' cmd internal pkg 2>/dev/null || true)
require_nonempty "fuzz targets (func FuzzXxx(f *testing.F))" "$targets" || exit 1

echo "Fuzz smoke (fuzztime=$FUZZTIME)"
fail=0
count=0
for file in $targets; do
	pkg="./$(dirname "$file")"
	while IFS= read -r fn; do
		count=$((count + 1))
		echo "  -> $pkg $fn"
		if ! CGO_ENABLED=1 go test -run '^$' -fuzz "^${fn}\$" -fuzztime "$FUZZTIME" "$pkg"; then
			echo "  [FAIL] $pkg $fn (crashers are saved under $pkg/testdata/fuzz/$fn)" >&2
			fail=1
		fi
	done < <(grep -oE '^func Fuzz[A-Za-z0-9_]*' "$file" | sed 's/^func //')
done
[ "$fail" -eq 0 ] || exit 1
echo "  [OK]   $count fuzz target(s) ran for $FUZZTIME each without crashers"
