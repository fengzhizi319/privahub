# Runbook: Credential Leak

Trigger: a secret was (or may have been) exposed — gitleaks/CI finding, secret
in logs / tickets / chat, leaked `.env` / backup, compromised laptop or CI.

**Principle: rotate first, investigate second. Removing the secret from git
history is NOT remediation** — assume anything pushed was copied.

## 0. Triage (≤ 15 min)

1. Open an incident (see [incident-response.md](incident-response.md)); note
   who found it, where, and when it was first exposed (`git log -S`, CI logs).
2. Classify the credential and jump to its row:

| Credential | Blast radius | Rotate via |
|---|---|---|
| `auth.jwt_secret` | anyone can mint admin sessions | [key-rotation.md §1](key-rotation.md#1-jwt-signing-secret) — **emergency mode** (no grace period) |
| `crypto.aes_key` / `crypto.keys` | decrypts datasource credentials / secret columns at rest (with DB access) | [key-rotation.md §2](key-rotation.md#2-at-rest-encryption-keys) + rotate every credential stored in the DB |
| `server.inner_auth.token` | forge node-to-node calls if the inner port is reachable | [key-rotation.md §3](key-rotation.md#3-inner-port-token-and-certificates) |
| node / Kuscia private keys (`config/certs`, `kuscia.key_file`) | impersonate this node to peers | re-issue certs via Kuscia, notify peers to distrust the old cert |
| `observability.metrics_token`, `audit.anchor.token` | metrics disclosure / forged anchors | regenerate, update secret, restart |
| user / admin password | account takeover | force reset (`POST /api/v1alpha1/user/reset-password`), sessions are revoked automatically |
| datasource credentials (OSS AK/SK, DB passwords) | data exfiltration at the source | rotate at the provider, update the datasource in Privahub |
| CI / registry / GitHub tokens | supply chain | revoke in the provider, audit recent workflow runs and pushed images |

## 1. Contain

- Revoke/rotate the credential (table above). Deploy the new value from the
  secret store; restart Privahub pods (`kubectl -n privahub rollout restart deploy/privahub`).
- JWT secret leak: rotate **without** `auth.jwt_previous_secrets` so every
  existing session is invalidated immediately.
- If exposure is public (public repo, paste site): treat as actively exploited.

## 2. Investigate

- Audit trail: logins, admin actions, datasource/node changes since first
  exposure (`POST /api/v1alpha1/audit/list`, `GET /api/v1alpha1/audit/verify`, or the DB); verify integrity first with
  `audit-verify` (see incident-response.md §3).
- Access logs: requests with valid tokens from unknown IPs; inner-port calls
  from unexpected peers.
- Kuscia: jobs / domaindata / routes created in the window.

## 3. Clean up

- Remove the secret from the source (file, log, ticket). If it was committed,
  rewriting history is optional hygiene (coordinate with all clones) — the
  rotation above is what matters.
- If gitleaks flagged a **false positive**, add a *narrow* allowlist entry to
  `.gitleaks.toml` with a justification in the same PR; never allowlist a
  real secret.
- Add a regression guard if the leak path was systematic (e.g. a new gitleaks
  rule, a log-redaction pattern in `scripts/check_log_redaction.sh`).

## 4. Close

Post-mortem within 5 business days: root cause, exposure window, affected
data, notifications (peers, customers, regulator where required by law —
e.g. PIPL/GDPR timelines), follow-up actions with owners.
