#!/usr/bin/env python3
# Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
"""Run real HTTP/browser flows on a disposable Privahub instance.

No development database or running backend is reused. Only --kuscia enables
external calls; that stage uses uniquely named project/graph/job resources and
cleans up through the existing PSI harness. Logs and a JSON report are retained.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
WORKSPACE = ROOT.parent


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def stop(process: subprocess.Popen) -> None:
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)


class Suite:
    def __init__(self, directory: Path):
        self.directory = directory
        self.results = []
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        self.token = ''
        self.base = ''

    def stage(self, name, command, *, cwd=ROOT, env=None, timeout=600):
        start = time.monotonic()
        with (self.directory / f'{name}.log').open('w') as log:
            # Start a process group so a failed/time-limited build cannot leave
            # a compiler, Vite worker or browser running after the runner exits.
            process = subprocess.Popen(command, cwd=cwd, env=env, stdout=log,
                                       stderr=subprocess.STDOUT, start_new_session=True)
            try:
                code = process.wait(timeout=timeout)
            finally:
                stop(process)
        self.results.append({'stage': name, 'passed': code == 0,
                             'seconds': round(time.monotonic() - start, 2)})
        if code:
            raise RuntimeError(f'{name} failed; see {self.directory / (name + ".log")}')
        print(f'PASS {name}', flush=True)

    def api(self, path, body=None, *, token=True, expect=0):
        data = json.dumps(body or {}).encode()
        req = urllib.request.Request(self.base + path, data=data,
                                     headers={'Content-Type': 'application/json'})
        if token:
            req.add_header('Authorization', 'Bearer ' + self.token)
        try:
            with self.http.open(req, timeout=30) as response:
                payload = json.load(response)
        except urllib.error.HTTPError as error:
            payload = json.load(error)
        code = payload.get('status', {}).get('code')
        if expect == 'error':
            if code is None or code == 0:
                raise AssertionError(f'{path}: expected a business/permission error, got {code}')
        elif code != expect:
            # Do not write request bodies or credentials into reports.
            raise AssertionError(f'{path}: expected {expect}, got {code}: {payload.get("status")}')
        return payload.get('data')

    def flows(self, password):
        api = self.api
        api('/api/v1alpha1/project/list', token=False, expect='error')
        auth = api('/api/login', {'name': 'admin', 'passwordHash': hashlib.sha256(password.encode()).hexdigest()}, token=False)
        self.token = auth.get('access_token') or auth.get('accessToken') or auth.get('token')
        if not self.token:
            raise AssertionError('login did not create a Bearer session')
        api('/api/v1alpha1/user/get')
        project = api('/api/v1alpha1/project/create', {'name': 'Migration verification', 'description': 'isolated flow', 'computeMode': 'MPC'})
        pid = project['projectId']
        # No seeded ids: all subsequent operations use ids returned by this run.
        for node in ['alice', 'bob']:
            api('/api/v1alpha1/project/node/add', {'projectId': pid, 'nodeId': node})
        api('/api/v1alpha1/project/update', {'projectId': pid, 'name': 'Migration verified', 'description': 'updated'})
        detail = api('/api/v1alpha1/project/get', {'projectId': pid})
        assert detail['projectName'] == 'Migration verified' and len(detail['nodes']) == 2
        graph = api('/api/v1alpha1/graph/create', {'projectId': pid, 'name': 'Flow graph'})
        gid = graph['graphId']
        node_id = gid + '-node-1'
        node = {'graphNodeId': node_id, 'codeName': 'read_data/datatable', 'label': 'Read data',
                'x': 80, 'y': 100, 'inputs': [], 'outputs': [node_id + '-output-0'],
                'nodeDef': {'domain': 'read_data', 'name': 'datatable', 'version': '0.0.1'}}
        api('/api/v1alpha1/graph/update', {'projectId': pid, 'graphId': gid, 'nodes': [node], 'edges': []})
        graph_detail = api('/api/v1alpha1/graph/detail', {'projectId': pid, 'graphId': gid})
        assert graph_detail['nodes'][0]['outputs'] == node['outputs']
        assert isinstance(graph_detail['nodes'][0]['nodeDef'], dict)
        api('/api/v1alpha1/graph/meta/update', {'projectId': pid, 'graphId': gid, 'name': 'Renamed graph'})
        api('/api/v1alpha1/project/delete', {'projectId': pid}, expect='error')
        api('/api/v1alpha1/project/job/list', {'projectId': pid, 'pageNum': 1, 'pageSize': 10})
        assert api('/api/v1alpha1/scheduled/graph/once/success', {'projectId': pid, 'graphId': gid}) is False
        sid = api('/api/v1alpha1/scheduled/id', {'projectId': pid, 'graphId': gid})
        assert isinstance(sid, str) and sid
        assert api('/api/v1alpha1/scheduled/page', {'projectId': pid})['total'] == 0
        assert api('/api/v1alpha1/model/page', {'projectId': pid})['total'] == 0
        api('/api/v1alpha1/model/serving/create', {'projectId': pid, 'modelId': 'unknown', 'partyConfigs': []}, expect='error')
        pending = api('/api/v1alpha1/message/pending', {'ownerId': 'kuscia-system'})
        assert isinstance(pending, int)
        api('/api/v1alpha1/message/list', {'ownerId': 'kuscia-system', 'page': 1, 'size': 10, 'isInitiator': False})
        api('/api/v1alpha1/approval/create', {'initiatorId': 'alice', 'voteType': 'INVALID'}, expect='error')
        components = api('/api/v1alpha1/component/list')
        assert isinstance(components, dict) and components
        batch = api('/api/v1alpha1/component/batch', [{'domain': 'read_data', 'name': 'datatable'}])
        assert isinstance(batch, list) and batch[0]['name'] == 'datatable'
        api('/api/v1alpha1/component/i18n')
        api('/api/v1alpha1/version/list')
        # CENTER has no institution record; reject foreign institution access.
        api('/api/v1alpha1/inst/get', {'instId': 'alice'}, expect='error')
        api('/api/v1alpha1/node/list')
        api('/api/v1alpha1/nodeRoute/page', {'page': 1, 'size': 10})
        # HTTP feature tables use real DB persistence without an external SQL/
        # OSS server. The local URL is stored, never fetched by this scenario.
        table = api('/api/v1alpha1/datatable/create', {'ownerId': 'alice', 'nodeIds': ['alice'],
            'datatableName': 'Migration features', 'datasourceType': 'HTTP',
            'relativeUri': 'https://example.com/features', 'columns': [{'colName': 'score', 'colType': 'float'}]})
        assert table['failedCreatedNodes'] == {}
        listed = api('/api/v1alpha1/datatable/list', {'ownerId': 'alice', 'types': ['HTTP'], 'pageSize': 100, 'pageNumber': 1})
        table_id = next(item['datatableVO']['datatableId'] for item in listed['datatableNodeVOList']
                        if item['datatableVO']['datatableName'] == 'Migration features')
        table_req = {'nodeId': 'alice', 'datatableId': table_id, 'datasourceType': 'HTTP'}
        assert api('/api/v1alpha1/datatable/get', table_req)['datatableVO']['schema'][0]['colName'] == 'score'
        relation = {'projectId': pid, 'nodeId': 'alice', 'datatableId': table_id, 'type': 'HTTP',
                    'configs': [{'colName': 'score', 'isLabelKey': True}]}
        api('/api/v1alpha1/project/datatable/add', relation)
        assert api('/api/v1alpha1/project/datatable/get', relation)['configs'][0]['isLabelKey'] is True
        api('/api/v1alpha1/datatable/delete', table_req, expect='error')
        api('/api/v1alpha1/project/datatable/delete', relation)
        api('/api/v1alpha1/datatable/delete', table_req)
        api('/api/v1alpha1/graph/delete', {'projectId': pid, 'graphId': gid})
        api('/api/v1alpha1/project/delete', {'projectId': pid})
        api('/api/v1alpha1/project/get', {'projectId': pid}, expect='error')
        self.authorization_flow()
        api('/api/logout')
        api('/api/v1alpha1/project/list', expect='error')
        self.results.append({'stage': 'http-business-flows', 'passed': True})
        print('PASS http-business-flows', flush=True)

    def authorization_flow(self):
        """Real allow→revoke→deny flow; no DB mutation or mocked identity."""
        api = self.api
        admin_token = self.token
        username = 'authz-' + secrets.token_hex(5)
        initial = 'Initial-' + secrets.token_hex(12)
        chosen = 'Chosen-' + secrets.token_hex(12)
        digest = lambda value: hashlib.sha256(value.encode()).hexdigest()
        api('/api/v1alpha1/user/create', {'name': username, 'passwordHash': digest(initial),
            'ownerType': 'CENTER', 'ownerId': 'kuscia-system', 'roleCodes': ['ADMIN']})
        try:
            initial_session = api('/api/login', {'name': username, 'passwordHash': digest(initial)}, token=False)
            self.token = initial_session['token']
            api('/api/v1alpha1/user/updatePwd', {'oldPasswordHash': digest(initial),
                'newPasswordHash': digest(chosen), 'confirmPasswordHash': digest(chosen)})
            session = api('/api/login', {'name': username, 'passwordHash': digest(chosen)}, token=False)
            self.token = session['token']
            api('/api/v1alpha1/user/list')  # proves the initial ADMIN grant worked
            self.token = admin_token
            api('/api/v1alpha1/user/update', {'name': username, 'roleCodes': []})
            self.token = session['token']
            api('/api/v1alpha1/user/get', expect='error')
            api('/api/user/refresh', {'refresh_token': session['refresh_token']}, token=False, expect='error')
            fresh = api('/api/login', {'name': username, 'passwordHash': digest(chosen)}, token=False)
            self.token = fresh['token']
            api('/api/v1alpha1/user/get')  # identity remains valid after re-login
            api('/api/v1alpha1/user/list', expect='error')  # ADMIN was removed
        finally:
            self.token = admin_token
            api('/api/v1alpha1/user/delete', {'name': username})
        self.results.append({'stage': 'authorization-revocation-flow', 'passed': True})
        print('PASS authorization-revocation-flow', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--browser', action='store_true', help='run desktop/mobile Chromium tests against this instance')
    parser.add_argument('--kuscia', action='store_true', help='also run a real two-node PSI; requires the local Docker Kuscia stack')
    parser.add_argument('--output', type=Path, help='new directory for reports/logs (must not exist)')
    args = parser.parse_args()
    directory = args.output.resolve() if args.output else Path(tempfile.mkdtemp(prefix='privahub-migration-'))
    if args.output:
        directory.mkdir(mode=0o700, parents=True, exist_ok=False)
    suite = Suite(directory)
    processes = []
    logs = []
    try:
        binary = directory / 'privahub'
        env = {k: v for k, v in os.environ.items() if not k.startswith('PRIVAHUB_')}
        env['CGO_ENABLED'] = '1'
        suite.stage('build-backend', ['go', 'build', '-o', str(binary), './cmd/server'], env=env)
        runtime = directory / 'runtime'
        (runtime / 'config').mkdir(parents=True)
        for name in ['components', 'i18n', 'components.json', 'graph.yaml']:
            (runtime / 'config' / name).symlink_to(ROOT / 'config' / name)
        port, inner, web_port = free_port(), free_port(), free_port()
        while len({port, inner, web_port}) != 3:
            inner, web_port = free_port(), free_port()
        password = 'Migration-' + secrets.token_hex(12)
        configuration = {
            'server': {'http_port': port, 'inner_port': inner, 'mode': 'master', 'node_id': 'kuscia-system',
                       'cors_allowed_origins': [f'http://127.0.0.1:{web_port}'],
                       'rate_limit': {'login_burst': 100, 'login_per_minute': 1000, 'global_burst': 1000, 'global_rps': 1000,
                                      'user_burst': 100, 'user_permits': 1000}},
            'database': {'driver': 'sqlite', 'dsn': str(runtime / 'test.db') + '?_busy_timeout=5000&_journal_mode=WAL'},
            'auth': {'jwt_secret': secrets.token_hex(32), 'password_kdf_iterations': 200000,
                     'initial_password_file': str(runtime / 'initial-password')},
            'crypto': {'aes_key': secrets.token_hex(16)},
            'kuscia': {'api_address': '127.0.0.1', 'api_port': int(os.environ.get('MIGRATION_KUSCIA_GRPC_PORT', '28083')) if args.kuscia else 1,
                       'http_port': int(os.environ.get('MIGRATION_KUSCIA_HTTP_PORT', '28082')) if args.kuscia else 1, 'protocol': 'notls',
                       'gateway': os.environ.get('MIGRATION_KUSCIA_GATEWAY', '127.0.0.1:28080') if args.kuscia else '127.0.0.1:1',
                       'pool_file': str(runtime / 'nodes.json')},
            'data': {'dir': str(runtime / 'uploads')},
            'observability': {'log_level': 'warn', 'log_format': 'json'},
        }
        if args.kuscia:
            configuration['kuscia']['nodes'] = [
                {'domain_id': domain, 'mode': 'lite', 'host': '127.0.0.1',
                 'port': int(os.environ.get(f'MIGRATION_{domain.upper()}_GRPC_PORT', str(grpc))),
                 'http_port': int(os.environ.get(f'MIGRATION_{domain.upper()}_HTTP_PORT', str(http))),
                 'protocol': 'notls'}
                for domain, grpc, http in [('alice', 28183, 28182), ('bob', 28283, 28282)]]
        config = runtime / 'config' / 'privahub.yaml'
        config.write_text(json.dumps(configuration))  # JSON is valid YAML.
        env.update(PRIVAHUB_PROFILE='test', PRIVAHUB_AUTH_ADMIN_PASSWORD=password)
        backend_log = (directory / 'backend.log').open('w')
        logs.append(backend_log)
        backend = subprocess.Popen([str(binary), '-config', str(config)], cwd=runtime, env=env,
                                   stdout=backend_log, stderr=subprocess.STDOUT, start_new_session=True)
        processes.append(backend)
        suite.base = f'http://127.0.0.1:{port}'
        deadline = time.monotonic() + 60
        while True:
            try:
                with suite.http.open(suite.base + '/api/v1alpha1/healthz', timeout=1) as response:
                    if response.status == 200:
                        break
            except (OSError, urllib.error.URLError):
                if backend.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError('isolated backend did not start; see backend.log')
                time.sleep(0.2)
        suite.flows(password)
        if args.browser:
            web_env = {**env, 'API_PROXY_URL': suite.base}
            web_log = (directory / 'vite.log').open('w')
            logs.append(web_log)
            vite = subprocess.Popen(['corepack', 'pnpm', 'exec', 'vite', '--port', str(web_port), '--strictPort'],
                cwd=ROOT / 'web/apps/secretpad', env=web_env, stdout=web_log,
                stderr=subprocess.STDOUT, start_new_session=True)
            processes.append(vite)
            web_url = f'http://127.0.0.1:{web_port}'
            deadline = time.monotonic() + 60
            while True:
                try:
                    with suite.http.open(web_url, timeout=1):
                        break
                except (OSError, urllib.error.URLError):
                    if vite.poll() is not None or time.monotonic() >= deadline:
                        raise RuntimeError('isolated Vite server did not start; see vite.log')
                    time.sleep(0.2)
            browser_env = {**web_env, 'E2E_BASE_URL': web_url, 'E2E_USER': 'admin', 'E2E_PASSWORD': password,
                           'E2E_ARTIFACT_DIR': str(directory / 'screenshots')}
            suite.stage('browser-business-flows', ['corepack', 'pnpm', 'exec', 'playwright', 'test',
                         '--workers=1', '--output', str(directory / 'playwright')],
                        cwd=ROOT / 'web/apps/secretpad', env=browser_env)
        if args.kuscia:
            python = os.environ.get('MIGRATION_PYTHON', sys.executable)
            psi_env = {**env, 'CLWORK_ADMIN_PASSWORD': password}
            suite.stage('real-kuscia-psi', [python, str(WORKSPACE / 'scripts/lib/psi_smoke.py'),
                         '--base', suite.base], env=psi_env, timeout=720)
        return 0
    except Exception as exc:
        suite.results.append({'stage': 'runner', 'passed': False, 'error': str(exc)})
        print(f'FAIL {exc}', file=sys.stderr)
        return 1
    finally:
        for process in reversed(processes):
            stop(process)
        for log in logs:
            log.close()
        # The report never contains the random password, cookies or bearer token.
        (directory / 'report.json').write_text(json.dumps({'results': suite.results,
            'realKusciaRequested': args.kuscia, 'browserRequested': args.browser}, indent=2) + '\n')
        print(f'Report: {directory / "report.json"}', flush=True)


if __name__ == '__main__':
    raise SystemExit(main())
