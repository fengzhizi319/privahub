#!/usr/bin/env bash
# =============================================================================
# check_openapi_drift.sh — the committed OpenAPI document must match the code.
#
# internal/controller/http/apidocs/openapi.json is generated from the real
# gin routers by cmd/openapi-gen. A route added/removed without regenerating
# the document means API consumers (web api-client codegen, security review of
# the exposed surface) work from a stale contract. The generator writes to a
# temp file so running this locally never touches the working tree.
#
# Fix:  CGO_ENABLED=1 go run ./cmd/openapi-gen   (then commit the result)
# =============================================================================
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib/preflight.sh
source scripts/lib/preflight.sh
require_cmd go "https://go.dev/dl/" || exit 1

SPEC=internal/controller/http/apidocs/openapi.json
[ -f "$SPEC" ] || { echo "  [FAIL] $SPEC not found" >&2; exit 1; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "OpenAPI drift gate"
if ! CGO_ENABLED=1 go run ./cmd/openapi-gen -out "$tmp/openapi.json" >"$tmp/gen.log" 2>&1; then
	echo "  [FAIL] openapi-gen failed:" >&2
	sed 's/^/      /' "$tmp/gen.log" >&2
	exit 1
fi
[ -s "$tmp/openapi.json" ] || { echo "  [FAIL] openapi-gen produced no output" >&2; exit 1; }
if ! diff -u "$SPEC" "$tmp/openapi.json" >"$tmp/diff.txt"; then
	echo "  [FAIL] $SPEC is out of date with the routers:" >&2
	head -60 "$tmp/diff.txt" | sed 's/^/      /' >&2
	echo "  fix: CGO_ENABLED=1 go run ./cmd/openapi-gen && commit $SPEC" >&2
	exit 1
fi
echo "  [OK]   $SPEC matches the generated document"
