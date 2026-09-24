# Runbook: Key Rotation

Routine cadence (and immediately after a suspected leak):

| Key | Routine | Downtime |
|---|---|---|
| `auth.jwt_secret` | 90 days | none (grace via `jwt_previous_secrets`) |
| `crypto.keys` (at-rest) | 12 months | none (versioned envelope) |
| `server.inner_auth.token` / inner mTLS certs | 90 days / cert lifetime | coordinated with the Kuscia gateway |
| `observability.metrics_token`, `audit.anchor.token` | 90 days | none |
| TLS server certificate (`server.tls.*`) | before expiry (automate) | restart |
| Kuscia client certs / node certs | per Kuscia policy | coordinated with peers |

All changes go through the secret store → Secret → rolling restart. Record
each rotation (who / when / key id, never the value) in the change log.

## 1. JWT signing secret

Tokens carry a `kid`; old secrets listed in `auth.jwt_previous_secrets` still
verify, new tokens are signed with `auth.jwt_secret`.

Routine (no forced logout):

1. `NEW=$(openssl rand -hex 32)`
2. Set `auth.jwt_previous_secrets: [<current secret>]` and `auth.jwt_secret: $NEW`
   (both from the secret store). Restart.
3. After `auth.access_token_expiry` (+ margin) has elapsed, remove the old
   value from `jwt_previous_secrets`. Restart.

Emergency (leak): set only the new `auth.jwt_secret`, **empty**
`jwt_previous_secrets`, restart → all sessions are invalid immediately.

## 2. At-rest encryption keys

Secret columns (node/inst tokens, datasource credentials, private keys) are
stored as SM4-GCM envelopes `enc:<version>:…` with a per-record HKDF-SM3 key
(`pkg/crypto`). `crypto.keys` maps versions to keys (≥ 32 hex chars or ≥ 16
raw bytes); `crypto.active_version` encrypts new data; every listed version
still decrypts. The legacy single `crypto.aes_key` is version `v1`.
**Losing every configured version makes the encrypted columns unreadable.**

Ways to inject the key map (merged in this order, later wins):

| Source | Example | Notes |
|---|---|---|
| `crypto.keys` in the config file | `crypto: {keys: {v1: …}}` | only if the config file itself is a Secret |
| `crypto.keys_file` / `PRIVAHUB_CRYPTO_KEYS_FILE` | `/run/secrets/privahub/crypto-keys.yaml` | YAML or JSON map `version: key`; mount from a Kubernetes **Secret** (`defaultMode: 0400`). A file readable by group/other logs a startup warning (expected 0600/0400). |
| `PRIVAHUB_CRYPTO_KEY_<VERSION>` | `PRIVAHUB_CRYPTO_KEY_V2=<hex>` | one env var per version; `<VERSION>` is lower-cased (`V2` → `v2`), allowed chars `A-Z 0-9 . _ -` |
| `crypto.aes_key` / `PRIVAHUB_CRYPTO_AES_KEY` | legacy | registered as `v1` only when no other source defines `v1` |

`crypto.active_version` / `PRIVAHUB_CRYPTO_ACTIVE_VERSION` selects the key for
new ciphertexts (required when several versions are configured and none is
`v1`).

Keys file example (`crypto-keys.yaml`, JSON works too):

```yaml
v1: 3f1c…   # old — keep until re-encryption is complete
v2: 9ab0…   # new
```

1. Generate: `openssl rand -hex 32`.
2. Add it as the next version and activate it — either in the keys file
   (`v2: ${NEW}` next to `v1`) or as env vars:

   ```bash
   PRIVAHUB_CRYPTO_KEY_V1=${OLD}      # keep until re-encryption is complete
   PRIVAHUB_CRYPTO_KEY_V2=${NEW}
   PRIVAHUB_CRYPTO_ACTIVE_VERSION=v2
   ```
3. Restart. The startup migration (`encrypt_at_rest`) re-encrypts secret
   columns whose ciphertext is not under the active version; check the log
   for `at-rest:` errors (a missing old version is logged per row).
4. Verify no rows remain on the old version (log shows 0 migrated on the next
   restart), take a backup, then remove `v1` and restart.
5. Leak case: in addition rotate every credential that was stored encrypted
   (datasource passwords, AK/SK) — re-encryption does not help if the
   ciphertext + old key were both exposed.

Back up key material separately from database backups; a backup is useless
without the key versions it was written with.

## 3. Inner-port token and certificates

1. Generate a new token; configure it on **both** sides: Privahub
   (`server.inner_auth.token_file`) and the Kuscia gateway / peer that sends
   `X-Privahub-Inner-Token`.
2. There is a single active token: schedule the change, update the gateway
   first only if it can send both, otherwise update both in the same window
   (inner calls fail with 401 in between; Kuscia retries).
3. mTLS: issue the new client cert from the same CA (SAN = node id matching
   `kuscia-origin-source`), deploy, then revoke the old one; rotating the CA
   requires listing both CAs in `client_ca_file` during the overlap.

## 4. Metrics / anchor tokens

Update the token in the secret store and in the consumer (Prometheus scrape
config / anchor service) — the consumer first if it supports two tokens.

## 5. After every rotation

- `make check`-equivalent smoke: login, list projects, create a test job.
- Confirm the old value no longer works (e.g. an old JWT → 401).
- Audit trail contains the restart / config reload events.
