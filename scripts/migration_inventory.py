#!/usr/bin/env python3
# Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
"""Extract the legacy Spring controller contract without requiring a Java build.

This inventory checks route coverage only. Business equivalence requires the flow
and service tests in test_migration.sh. Generated snapshots let CI run without the
legacy checkout, while --check detects upstream route changes during migration.
"""
import argparse
import json
from pathlib import Path
import re
import sys


def inventory(source: Path) -> list[dict[str, str]]:
    controllers = source / 'secretpad-web/src/main/java'
    if not controllers.is_dir():
        raise ValueError(f'controller directory missing: {controllers}')
    routes = []
    for path in sorted(controllers.rglob('*Controller.java')):
        # Block comments may contain annotations; strip them before extracting.
        text = re.sub(r'/\*.*?\*/', '', path.read_text(), flags=re.S)
        declaration = re.search(r'public\s+class\s+\w+', text)
        if not declaration:
            continue
        before, body = text[:declaration.start()], text[declaration.end():]
        bases = re.findall(r'@RequestMapping\(\s*(?:value\s*=\s*)?"([^"]+)"', before)
        base = bases[-1] if bases else ''
        for match in re.finditer(r'@(Post|Get|Put|Delete)Mapping\(\s*(?:value\s*=\s*)?"([^"]*)"', body):
            url = '/' + '/'.join(part.strip('/') for part in [base, match[2]] if part.strip('/'))
            routes.append({'controller': path.stem, 'method': match[1].upper(), 'path': url,
                           'listener': 'inner' if url in {'/sync', '/api/v1alpha1/data/sync',
                               '/api/v1alpha1/vote_sync/create', '/api/v1alpha1/user/node/resetPassword'} else 'main'})
    if not routes:
        raise ValueError('no controller routes extracted')
    keys = [(r['method'], r['path']) for r in routes]
    if len(keys) != len(set(keys)):
        raise ValueError('duplicate legacy routes; inspect controller profiles before updating the snapshot')
    return sorted(routes, key=lambda r: (r['controller'], r['method'], r['path']))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True, help='legacy secretpad root (read only)')
    parser.add_argument('--output', type=Path, default=Path(__file__).resolve().parents[1] /
                        'internal/controller/http/testdata/legacy_routes.json')
    parser.add_argument('--check', action='store_true', help='compare without modifying the snapshot')
    args = parser.parse_args()
    try:
        rows = inventory(args.source)
        rendered = json.dumps(rows, ensure_ascii=False, indent=2) + '\n'
        if args.check:
            if args.output.read_text() != rendered:
                print('Legacy API inventory drift: review and regenerate the snapshot.', file=sys.stderr)
                return 1
        else:
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(rendered)
        print(f'{len(rows)} legacy API routes, {len(set(r["controller"] for r in rows))} controllers')
        return 0
    except (OSError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
