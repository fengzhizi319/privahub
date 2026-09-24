# Privahub Production Hardening Guide

Checklist for running Privahub outside development. Every key below is a
`config/privahub.yaml` key; each can also be set as an environment variable
`PRIVAHUB_<KEY_WITH_DOTS_AS_UNDERSCORES>` (e.g. `auth.jwt_secret` →
`PRIVAHUB_AUTH_JWT_SECRET`). Every scalar / list key is bound to its env var
even when it is absent from the config file; maps are the exception
(`crypto.keys` → `crypto.keys_file` or `PRIVAHUB_CRYPTO_KEY_<VERSION>`).

Privahub is **fail-closed**: without `PRIVAHUB_PROFILE=dev|test|local` it
refuses to start when a production requirement below is not met. Never set a
dev/test profile in production — it silently relaxes most of these checks.

Reference deployments: `deployments/k8s/` (kustomize) and
`deployments/docker/docker-compose.yml`.

## 1. Profile and process

- [ ] `PRIVAHUB_PROFILE` unset (or a non-dev profile such as `edge`/`p2p`).
- [ ] Run the official image: non-root uid 65532, read-only root FS,
      `capabilities: drop [ALL]`, `allowPrivilegeEscalation: false`,
      seccomp `RuntimeDefault` (see `deployments/k8s/deployment.yaml`).
- [ ] Writable paths only on volumes: `/app/data`, `/app/audit`,
      `/app/config/{certs,kuscia,sync}`, `/tmp`.
- [ ] Namespace labelled `pod-security.kubernetes.io/enforce: restricted`.
- [ ] `observability.log_format: json`, `log_level: info` (debug may log request metadata).

## 2. Secrets (inject from a secret store, never in git / image / ConfigMap)

| Key | Requirement | Generate |
|---|---|---|
| `auth.jwt_secret` | ≥16 chars, not the sample value | `openssl rand -hex 32` |
| `auth.jwt_previous_secrets` | only during rotation; remove after `access_token_expiry` | — |
| `crypto.keys_file` (0400 Secret mount) or `PRIVAHUB_CRYPTO_KEY_<VERSION>`, + `crypto.active_version` (legacy: `crypto.aes_key` = v1) | at-rest envelope key(s), not the sample; see [key rotation §2](runbooks/key-rotation.md#2-at-rest-encryption-keys) | `openssl rand -hex 32` |
| `server.inner_auth.token` / `token_file` | ≥16 chars; mandatory in production | `openssl rand -hex 32` |
| `observability.metrics_token` | for Prometheus Bearer scraping | `openssl rand -hex 32` |
| `audit.anchor.token` | bearer for `audit.anchor.url` | provider-issued |
| `kuscia.token`, `kuscia.cert_file/key_file` | KusciaAPI client credentials | from Kuscia |
| `auth.admin_password` | optional; if empty a random one-time password is written to `auth.initial_password_file` (0600) and must be changed at first login | — |

> Every scalar config key can be set as `PRIVAHUB_<KEY>` (dots → `_`), even
> when it is absent from the config file (e.g. `PRIVAHUB_AUTH_ADMIN_PASSWORD`,
> `PRIVAHUB_SERVER_NODE_ID`). Maps cannot: `crypto.keys` uses
> `crypto.keys_file` / `PRIVAHUB_CRYPTO_KEY_<VERSION>` instead. Prefer the
> generated one-time admin password over `auth.admin_password`.

- [ ] File-based secrets mounted read-only with mode 0400 (no startup warning
      `crypto.keys_file … has permissions`).
- [ ] `auth.initial_password_file` deleted after first login (done automatically).

## 3. Transport security

- [ ] Main port: `server.tls.enabled: true` with `cert_file`/`key_file`
      (`min_version: "1.2"` or `"1.3"`; `stack: tls|tlcp|both` for 国密 TLCP),
      **or** `server.behind_tls_proxy: true` **and** `server.trusted_proxies`
      listing only the proxy addresses/CIDRs.
- [ ] `server.trusted_platform` only if the CDN strips that header from clients.
- [ ] `auth.cookie_secure` left default (Secure when TLS/proxy) or `true`.
- [ ] `kuscia.protocol: tls` or `mtls` (and every `kuscia.nodes[].protocol`),
      `kuscia.ca_file` set, `kuscia.insecure_skip_verify: false`.
- [ ] EDGE (`server.mode: lite|edge`): `kuscia.gateway` is `https://…` (plain
      http only for a loopback gateway). **Set `PRIVAHUB_KUSCIA_GATEWAY`
      explicitly**: the `${KUSCIA_GW_ADDRESS:…}` literal in
      `config/privahub-edge.yaml` is not shell-expanded by the config loader.

## 4. Inner port (9001)

- [ ] `server.inner_bind_address: 127.0.0.1` unless the Kuscia gateway runs in
      another container/pod.
- [ ] `server.inner_auth`: token **and** `allowed_cidrs` narrowed to the
      Kuscia gateway addresses; prefer mTLS
      (`inner_auth.tls.{cert_file,key_file,client_ca_file}`, `gateway_names`)
      so `kuscia-origin-source` is bound to the client certificate.
- [ ] Never publish 9001 on a public interface / Ingress / LoadBalancer
      (compose binds `127.0.0.1`; k8s NetworkPolicy allows only the Kuscia namespace).

## 5. HTTP surface

- [ ] `observability.metrics_access: restricted` (default outside dev) with
      `metrics_allowed_cidrs` or `metrics_token`; or `auth`/`disabled`.
- [ ] `server.api_docs.enabled: false` (default outside dev).
- [ ] `server.cors_allowed_origins` empty or exact origins (no `*`).
- [ ] Limits: `server.max_body_size` (32 MiB default), `server.upload_max_size`
      (2 GiB default; `-1` = unlimited — do not), `server.read_header_timeout`,
      `server.idle_timeout`, `server.max_header_bytes`, `server.rate_limit.*`.
- [ ] Login protection: `auth.max_attempts` / `auth.lock_minutes` lock the
      (account, client IP) pair; `auth.account_max_attempts` (default
      4 × `max_attempts`) locks the whole account for every IP; keep
      `auth.password_kdf_iterations` at the default 1,000,000 SM3 iterations
      (values below 200,000 are raised). Client IPs are only taken from
      `X-Forwarded-For` when the peer is in `server.trusted_proxies`
      (default: none trusted).
- [ ] First start after upgrading from the Java/legacy hashes wraps every
      stored password hash in the KDF (≈0.26 s per account, parallelised) —
      plan the start-up window for large user tables and do not kill the pod
      during it (`startupProbe` allows 150 s).
- [ ] Clients handle the password-policy error codes: 202011605 password
      change required (`must_change_password` sessions may only change the
      password), 202011606 password too weak, 202011607 password reuse,
      202011608 CSRF check failed (cookie sessions need the double-submit
      CSRF header on unsafe methods).
- [ ] Sessions: `auth.access_token_expiry` (default 2h — keep it short), `auth.session_idle_timeout` (≤ 30m for admin consoles).

## 6. Data plane

- [ ] `data.ip_block.enable: true` (SSRF protection; the default list blocks
      loopback, RFC1918 and link-local incl. 169.254.169.254) — add internal
      ranges of your network; never disable it in production.
- [ ] Kubernetes egress default-deny, with explicit allow rules per approved
      datasource endpoint (`deployments/k8s/networkpolicy.yaml`).
- [ ] `PRIVAHUB_DATA_DIR` / `data.dir` on a dedicated volume (0750).
- [ ] `database`: SQLite file on the data volume, or MySQL over TLS with a
      least-privilege account (no `SUPER`, no `FILE`).
- [ ] `data.tee.cert_dir` / `config/certs` on a 0700 volume, not in the image.

## 7. Audit

- [ ] `audit.spool_dir` / `audit.archive_dir` on their own volume (`/app/audit`).
- [ ] `audit.retention_days: 0` (keep forever) or ≥ 1095.
- [ ] `audit.anchor.file` on WORM storage or another host, and/or
      `audit.anchor.url` + `audit.anchor.token`; `audit.anchor.interval` ≤ 1h.
- [ ] Grant audit-trail read access (`/api/v1alpha1/audit/*`) only through
      the dedicated `AUDITOR` role (or ADMIN); keep auditors separate from
      operators who can change configuration.
- [ ] Schedule `audit-verify -config … -anchor-file … -archive-dir …`
      (shipped in the image) and alert on exit code 2.
- [ ] Alert on audit spool drops (metrics) and on spool growth.

## 8. Operations

- [ ] Image pinned by digest; weekly rebuild to pick up base-image fixes.
- [ ] Backups of the data volume encrypted; restore tested; keys backed up
      separately from data.
- [ ] Runbooks rehearsed: [credential leak](runbooks/credential-leak.md),
      [key rotation](runbooks/key-rotation.md),
      [incident response](runbooks/incident-response.md),
      [break-glass admin reset](runbooks/break-glass-admin-reset.md).

## Development gates

| Command | What it runs |
|---|---|
| `make check` | meta-gate, gofmt, vet, golangci-lint (gosec), SQLi guard, log-redaction guard, image-pin gate, k8s/compose gate, OpenAPI drift, `go test -race`, govulncheck, gitleaks |
| `make sec` | the security subset (gates + gitleaks + govulncheck) |
| `make lint` / `make vuln` / `make secrets` / `make fuzz` / `make hadolint` / `make sbom` | individual tools (pinned versions, run via `go run` if not installed) |
| `pre-commit` | gitleaks (staged), gofmt, golangci-lint `--fast-only` on new code, SQLi/redaction/manifest gates |

Install pre-commit once per clone:

```bash
pipx install pre-commit            # or: python3 -m pip install --user pre-commit
make pre-commit-install
pre-commit run --all-files         # optional first full run
```

Gates are fail-closed: a missing tool/dependency fails the gate
(`scripts/lib/preflight.sh`), and `scripts/check_gate_failclosed.sh` rejects
gate scripts, Make recipes or CI steps that could pass silently.
