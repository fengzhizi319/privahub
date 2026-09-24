#!/usr/bin/env bash
# =============================================================================
# preflight.sh — hard-fail dependency checks for scripts/check_*.sh gates.
#
# A gate that silently passes because a tool is missing is worse than no gate:
# it makes "make check is green" mean "the checks ran" when they did not.
# Every gate therefore sources this file and calls require_* for each external
# dependency; a missing dependency is a FAILURE with an actionable fix.
#
# There is deliberately no "skip" switch. (Ported from PrivShield.)
#
#   source "$(dirname "${BASH_SOURCE[0]}")/lib/preflight.sh"
#   require_cmd python3 "install python3 (apt-get install -y python3)"
# =============================================================================

# require_cmd <command> <install hint>
require_cmd() {
	local cmd="$1" hint="${2:-}"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "[preflight] required command '$cmd' not found - this gate cannot run." >&2
		[ -n "$hint" ] && echo "            install: $hint" >&2
		echo "            A gate that cannot run must fail, not pass silently." >&2
		return 1
	fi
	return 0
}

# require_python_module <import name> <pip package>
require_python_module() {
	local module="$1" package="$2"
	require_cmd python3 "apt-get install -y python3" || return 1
	if ! python3 -c "import ${module}" >/dev/null 2>&1; then
		echo "[preflight] python3 module '${module}' missing - install: python3 -m pip install ${package}" >&2
		return 1
	fi
	return 0
}

# require_nonempty <description> <value>
# Guards against "no targets found => nothing to check => pass".
require_nonempty() {
	local what="$1" value="$2"
	if [ -z "$value" ]; then
		echo "[preflight] no ${what} found - a gate with no targets would pass vacuously." >&2
		return 1
	fi
	return 0
}
