# Privahub Threat Model (STRIDE)

Status: living document — update it in the same PR as any change to a trust
boundary (new listener, new outbound integration, new credential type).
Last reviewed: 2026-09. Companion documents:
[hardening-guide.md](hardening-guide.md) · [runbooks/](runbooks/) · [../../SECURITY.md](../../SECURITY.md).

## 1. System and trust boundaries

```text
 Browser (SPA, privaconsole)                                    Other institutions
   │  TB1: Internet / office network → TLS proxy or server.tls  (their Privahub + Kuscia)
   ▼                                                                   ▲
 ┌──────────────────────── Privahub process ─────────────────────┐    │ TB5: cross-domain
 │ main API :8080  (JWT/cookie session, RBAC, rate limit, audit) │    │ (Kuscia gateway, mTLS)
 │ inner API :9001 (node-to-node; inner_auth token/CIDR/mTLS)    │◄───┤
 │ /metrics (metrics_access)   /api/v1alpha1/docs (api_docs)     │    │
 │ EDGE forwarder / P2P sync producers & consumers               │────┘
 │ datasource connectors (OSS/S3, ODPS, MySQL, HTTP) ──TB6──► external endpoints (SSRF surface)
 │ TEE capsule-manager client ──TB7──► TEE domain
 │ audit trail (hash chain, spool, anchor) ──TB8──► WORM file / anchor URL
 └──────────┬───────────────────────────┬────────────────────────┘
            │ TB3: DB (SQLite file / MySQL) │ TB4: KusciaAPI gRPC/HTTP (tls/mtls + token)
            ▼                               ▼
        data volume                    Kuscia master/lite  ──► SecretFlow task pods
```

| ID | Boundary | Crossed by | Primary controls |
|---|---|---|---|
| TB1 | user ↔ main API | SPA, API clients | TLS (`server.tls` or TLS proxy + `trusted_proxies`), JWT + session idle timeout, Secure/HttpOnly cookies, CSRF-safe design, RBAC, login throttling/lockout, CSP |
| TB2 | Kuscia gateway ↔ inner API | node callbacks, edge/P2P sync | `server.inner_auth` (token ≥16 chars, CIDR allowlist, mTLS with SAN = `kuscia-origin-source`), loopback bind by default, NetworkPolicy |
| TB3 | process ↔ database | GORM | parameterised queries (CI SQLi gate), at-rest SM4-GCM/HKDF-SM3 envelope encryption for secret columns (`crypto.keys`, versioned), 0600/0700 file perms, dedicated volume |
| TB4 | process ↔ KusciaAPI | job/domaindata/route management | `kuscia.protocol=tls|mtls` (required in production), CA pinning (`ca_file`), token |
| TB5 | institution ↔ institution | P2P votes, project/job sync | Kuscia gateway mTLS, vote signatures, ownership checks on synced rows |
| TB6 | process ↔ datasource endpoints | user-supplied endpoints | SSRF guard (`data.ip_block`, guarded dialer), NetworkPolicy egress default-deny |
| TB7 | process ↔ TEE | capsule-manager | node certificates in `data.tee.cert_dir`, SM3 integrity |
| TB8 | audit ↔ external anchor | tail hash export | anchor file on WORM / anchor URL with bearer token, `audit-verify` |

Assets: user credentials & sessions; JWT signing secret(s); at-rest keys
(`crypto.keys`); node private keys/certificates (`config/certs`); datasource
credentials (AK/SK, DB passwords); the inner token; project/job metadata and
result references; the audit trail (evidence).

Actors: anonymous Internet user; authenticated low-privilege user; node admin
of another institution (semi-trusted peer); malicious co-located process on
the host/pod network; compromised upstream dependency / CI; insider operator.

## 2. STRIDE by component

### 2.1 SPA (privaconsole)

| | Threat | Mitigation | Residual / owner |
|---|---|---|---|
| S | Session theft via XSS | strict CSP (`script-src 'self'`, no inline), React escaping, HttpOnly cookie session | 3rd-party script supply chain → `pnpm audit`, lockfile, CI |
| T | Malicious dependency in bundle | `--frozen-lockfile`, `pnpm audit --prod`, gitleaks, SBOM | no npm provenance verification yet |
| R | — | actions are audited server-side | — |
| I | Tokens in localStorage, source maps leaking code | cookie session; CI fails if `.map` files ship | legacy bearer-token flows |
| D | — | — | — |
| E | UI-only authorisation | every permission re-checked by backend RBAC / data-resource middleware | — |

### 2.2 Main API (:8080)

| | Threat | Mitigation |
|---|---|---|
| S | credential stuffing / brute force | per-IP login rate limit, account lockout (`auth.max_attempts`, `lock_minutes`), KDF-hashed passwords (`auth.password_kdf_iterations`), weak-password list |
| S | forged JWT | HMAC secret ≥16 chars refused if sample; rotation via `auth.jwt_previous_secrets`; password-version claim invalidates old tokens |
| T | parameter confusion / mass assignment | strict JSON key normalisation, bind-consistency guard (fuzzed: `FuzzGuardBindingConsistency`) |
| T | SQL injection | GORM placeholders; `scripts/check_sql_injection_guard.sh` gate |
| R | user denies an action | hash-chained audit trail for auth + write operations |
| I | verbose errors, API docs, metrics | `api_docs.enabled=false` in prod, `metrics_access=restricted`, log redaction gate |
| D | slowloris, huge bodies, upload abuse | `read_header_timeout`, `max_header_bytes`, `max_body_size`, `upload_max_size` (2 GiB default), global rate limit |
| E | horizontal access to other nodes' projects | RBAC + data-resource ownership middleware; AUTONOMY whitelist is explicit |

### 2.3 Inner port (:9001)

The inner API trusts `kuscia-origin-source` to identify the calling node.

| | Threat | Mitigation |
|---|---|---|
| S | any process that reaches :9001 forges `kuscia-origin-source` | `inner_auth` mandatory in production even on loopback; token compared in constant time; mTLS binds origin to certificate SAN |
| T | replay/modify sync payloads | TLS/mTLS; ownership checks on synced rows (`sync/ownership.go`) |
| I | port exposed to the Internet | default bind `127.0.0.1`; compose binds `127.0.0.1`; k8s `privahub-inner` ClusterIP + NetworkPolicy (Kuscia namespace only); never on an Ingress |
| E | inner routes reachable on the main port | `inner_port_paths` served only by the inner router |

### 2.4 Kuscia (KusciaAPI + gateway)

| | Threat | Mitigation | Residual |
|---|---|---|---|
| S | MITM of KusciaAPI | `kuscia.protocol=tls|mtls` enforced in prod, `insecure_skip_verify` refused | dev profile uses `notls` |
| T | tampered job definitions | only Privahub holds the KusciaAPI client cert/token | Kuscia container runs `privileged` (upstream requirement) → dedicated host |
| E | container escape from task pods | upstream Kuscia/containerd isolation | out of Privahub scope |

### 2.5 EDGE / P2P sync

| | Threat | Mitigation |
|---|---|---|
| S | a peer impersonates another node | Kuscia gateway mTLS + `kuscia-origin-source` bound to cert SAN; vote signatures |
| T | peer writes rows it does not own | ownership validation on consumed records |
| I | EDGE forwards user requests over plain HTTP | production requires `https://` `kuscia.gateway` unless loopback |
| D | sync queue exhaustion | `sync.max_retries`, bounded queues, `edge_idle_timeout` |

### 2.6 Datasources (SSRF)

User-supplied OSS/S3/ODPS/HTTP/MySQL endpoints are fetched server-side.

| | Threat | Mitigation |
|---|---|---|
| I/E | SSRF to cloud metadata (169.254.169.254), KusciaAPI, inner port, cluster services | `data.ip_block` (enabled outside dev) enforced at **dial time** (post-DNS, defeats rebinding) via the guarded dialer; k8s egress default-deny |
| I | datasource credentials exposed | at-rest encryption of secret columns, `json:"-"` on secret fields, log redaction gate |
| T | path traversal in uploads / local datasource paths | `inDir` containment under `PRIVAHUB_DATA_DIR`; gosec G304 exclusions limited to operator-configured paths |

### 2.7 TEE

| | Threat | Mitigation |
|---|---|---|
| S | fake capsule manager | TLS with node certs from `data.tee.cert_dir`; `sync.tee` init config |
| T | tampered TEE task config / results | SM3 digests over task payloads |
| I | node private key disclosure | cert dir on a dedicated 0700 volume, never baked into the image |

### 2.8 Audit trail

| | Threat | Mitigation |
|---|---|---|
| T | DB admin deletes/edits audit rows | SM3 hash chain + `audit_chain_tail`; periodic external anchor (`audit.anchor.file` on WORM / `audit.anchor.url`); `audit-verify` detects the first broken link |
| R | audit gaps when DB is down | fsync'd spool (`audit.spool_dir`, capped by `spool_max_bytes`, drops are counted in metrics) |
| I | secrets in audit records / logs | redaction in the audit middleware; `check_log_redaction.sh` gate |
| D | retention deletes evidence early | `retention_days` 0 or ≥ 1095 in production; archival to hash-chained segments before deletion |

### 2.9 Build & supply chain

| Threat | Mitigation |
|---|---|
| compromised GitHub Action (mutable tag) | all actions pinned by commit SHA; actionlint |
| vulnerable Go/npm dependency | `govulncheck` (reachable → fail), `pnpm audit --prod`, weekly scheduled CI |
| vulnerable base image | digest-pinned bases, Trivy (fixable HIGH/CRITICAL → fail), SBOMs as artifacts |
| committed secret | gitleaks over full history (CI) + pre-commit hook |
| gate silently skipped | `scripts/check_gate_failclosed.sh` meta-gate |

## 3. Known gaps / follow-ups

- `goimports` grouping not yet enforced by lint (formatting-only follow-up).
- EDGE `config/privahub-edge.yaml` contains a `${KUSCIA_GW_ADDRESS:…}` literal
  that is never expanded; production must set `PRIVAHUB_KUSCIA_GATEWAY`.
- Kuscia requires `privileged` containers upstream.
