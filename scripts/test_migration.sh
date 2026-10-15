#!/usr/bin/env bash
# Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
# Systematic migration gate. No existing database is changed; every stage fails
# closed and logs its output. --kuscia explicitly enables external computation.
set -euo pipefail
TASK_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_BROWSER=1
RUN_KUSCIA=0
LEGACY_SOURCE=''
while (($#)); do
  case "$1" in
    --no-browser) RUN_BROWSER=0 ;;
    --kuscia) RUN_KUSCIA=1 ;;
    --legacy-source) LEGACY_SOURCE="${2:?--legacy-source requires a path}"; shift ;;
    -h|--help)
      echo 'Usage: bash scripts/test_migration.sh [--no-browser] [--kuscia] [--legacy-source PATH]'
      echo 'Runs Go tests/race/vet, frontend typecheck/lint/tests/build, isolated HTTP + browser flows.'
      echo 'Real PSI: MIGRATION_PYTHON=/path/to/sf310/bin/python ... --kuscia'
      exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done
cd "$TASK_ROOT"
if [[ -n "$LEGACY_SOURCE" ]]; then
  python3 scripts/migration_inventory.py --source "$LEGACY_SOURCE" --check
fi
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./internal/service/... ./internal/controller/http/...
CGO_ENABLED=1 go vet ./...
cd "$TASK_ROOT/web"
corepack pnpm typecheck
corepack pnpm lint
corepack pnpm test
corepack pnpm build
cd "$TASK_ROOT"
SYSTEM_ARGS=()
if ((RUN_BROWSER)); then SYSTEM_ARGS+=(--browser); fi
if ((RUN_KUSCIA)); then SYSTEM_ARGS+=(--kuscia); fi
python3 scripts/run_system_e2e.py "${SYSTEM_ARGS[@]}"
