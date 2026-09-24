# Runbook: Break-glass Admin Reset

Use when **no** administrator can log in (lost password, account locked,
compromised admin account). Every use is a security event:
open an incident ticket first and record who, when and why.

Requirements: two people (four-eyes: operator + approver), shell access to the
deployment, and a fresh backup of the data volume.

## Option A — another admin exists (preferred)

`POST /api/v1alpha1/user/reset-password` as the other admin, with a temporary
password handed over out of band. The new password must satisfy the policy
(errors 202011606 too weak / 202011607 reused), **the account is flagged
`must_change_password`** and all its sessions are revoked (password version
bump). At the next login the user's session is restricted to the
change-password flow (error 202011605 on every other API) until they choose
their own password. Lockouts expire after `auth.lock_minutes`.

## Option B — no working admin: `privahub-admin reset-password`

`privahub-admin` (shipped in the image at `/usr/local/bin/privahub-admin`,
built from `cmd/privahub-admin`) works directly on the database. In **one
transaction** it:

- generates a random one-time password (20 chars, crypto/rand);
- stores it with the server password KDF (`sm3kdf:v1`, work factor
  `auth.password_kdf_iterations`), keeping the old hash in the reuse history;
- sets `must_change_password` and clears login / reset lockouts;
- revokes every session of the user (`user_tokens` rows deleted, password
  version bumped, so outstanding JWTs are rejected too);
- appends a hash-chained audit event `auth.password.reset`
  (`actor_type=system`, `actor=privahub-admin`, `reason` = `--reason`,
  `extra.method=break_glass_cli`, `extra.operator`, `extra.host`).

If any step fails nothing is changed. The password is never logged or
audited; it is printed **once** to stdout, or written to a new file with mode
0600 (`--output-file`, refuses to overwrite an existing file).

| Flag | Meaning |
|---|---|
| `--user` (required) | account name (`auth.admin_name`, default `admin`) |
| `--reason` (required) | incident / ticket id and why — stored in the audit trail |
| `--operator` | person running the tool (default `$SUDO_USER` / OS user) |
| `--output-file` | write the password to this new 0600 file instead of stdout |
| `--config` | config file; database, KDF work factor and `PRIVAHUB_*` env are read like the server |
| `--driver` / `--dsn` | open the database directly instead of reading `--config` |
| `--length` | password length (default 20, minimum 16) |

Exit codes: 0 success, 1 error (nothing changed), 2 usage error.

### Procedure

1. **Backup**: copy the data volume (SQLite `privahub.db*`) or take a MySQL dump.
2. **Stop** Privahub (`kubectl -n privahub scale deploy/privahub --replicas=0`,
   or `docker compose stop privahub`). With SQLite this is required (single
   writer); with MySQL it is strongly recommended so no request races the reset.
3. **Run the tool** with the same image, config and secrets as the server
   (a one-off pod / container that mounts the data volume):

   ```bash
   # Kubernetes: one-off Job built from the Deployment's pod template
   # (same image, env, secrets and volumes; probes removed)
   kubectl -n privahub get deploy/privahub -o json | jq --arg op "$USER" '{
     apiVersion: "batch/v1", kind: "Job",
     metadata: {name: "privahub-breakglass", namespace: "privahub"},
     spec: {backoffLimit: 0, ttlSecondsAfterFinished: 600, template: (.spec.template
       | .spec.restartPolicy = "Never"
       | .spec.containers[0] |= (del(.livenessProbe, .readinessProbe, .startupProbe)
         | .command = ["privahub-admin"]
         | .args = ["reset-password", "--config", "/etc/privahub/privahub.yaml",
                    "--user", "admin", "--reason", "INC-1234: admin locked out",
                    "--operator", $op]))}}' | kubectl apply -f -
   kubectl -n privahub wait --for=condition=complete job/privahub-breakglass --timeout=120s
   kubectl -n privahub logs job/privahub-breakglass   # read once
   kubectl -n privahub delete job/privahub-breakglass

   # Docker Compose
   docker compose run --rm --no-deps --entrypoint privahub-admin privahub \
     reset-password --config /etc/privahub/privahub.yaml \
     --user admin --reason "INC-1234: admin locked out" --operator "$USER"
   ```

   Output (stdout, once):

   ```text
   one-time password for admin: <password>
   password reset for "admin": must change at next login, all sessions revoked, audit event auth.password.reset recorded (operator alice)
   ```

   Container logs are retained by the runtime / log shipper: to keep the
   password out of them, add `--output-file /app/data/breakglass-otp` (0600 on
   the data volume) and read it once from a maintenance pod that mounts the
   volume (the distroless runtime image has no shell or `tar`, so
   `kubectl cp` / `exec cat` into the server pod do not work), then delete the
   file. Delete the one-off Job / container afterwards.
4. **Start** Privahub (`--replicas=1`).
5. The operator hands the one-time password to the account owner out of band
   (approver present). Log in: the session is restricted to the
   change-password call (202011605 elsewhere) until a new strong password is
   set.
6. **Verify** the audit trail: the `auth.password.reset` event with the
   reason, then `auth.password.change` and the first login
   (`audit-verify --config …` must report the chain valid). Review what the
   account did before the reset.

### Fallback (tool unavailable)

Only if `privahub-admin` cannot be run (e.g. image predating it): retire the
account so the server re-creates it at start-up with a generated one-time
password written to `auth.initial_password_file` (0600, must change at first
login). This bypasses the password KDF history and writes **no** audit event
for the reset — record it manually in the incident ticket.

```sql
BEGIN;
UPDATE user_accounts
   SET name = 'admin-retired-' || strftime('%Y%m%d%H%M%S','now')
 WHERE name = 'admin';
DELETE FROM user_tokens WHERE name = 'admin';
COMMIT;
```

Ensure `auth.admin_password` is not set and the initial password file does
not exist, start the server (log: `initial admin password generated`), read
the file once, and later delete or permanently disable the retired row.

## Afterwards

- Rotate `auth.jwt_secret` if the old admin account may have been compromised
  ([key-rotation.md §1](key-rotation.md#1-jwt-signing-secret), emergency mode).
- File the incident report ([incident-response.md](incident-response.md)).
