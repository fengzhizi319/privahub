# Runbook: Security Incident Response

Applies to any suspected compromise of Privahub, its data, credentials, or
the Kuscia nodes it manages. Severity decides the pace:

| Sev | Examples | Response |
|---|---|---|
| SEV1 | active exploitation, admin/JWT secret leaked publicly, cross-institution data exposure | immediate, 24x7, exec + legal informed |
| SEV2 | confirmed vulnerability reachable in prod, credential leak without evidence of use | same business day |
| SEV3 | suspicious activity, low-impact misconfiguration | next business day |

Roles: **Incident lead** (decides, communicates), **Ops** (contain/restore),
**Investigator** (evidence), **Comms** (peers/customers/regulators).

## 1. Detect & declare

Sources: alerts (auth failures, lockouts, audit spool drops, `audit-verify`
exit 2, Trivy/govulncheck findings), peer institutions, SECURITY.md reports.
Open an incident channel/ticket, assign roles, start a timeline (UTC).

## 2. Contain (preserve evidence first where possible)

- Snapshot before changing: data volume (DB + uploads), audit volume
  (`/app/audit` spool/archive), container logs, Kuscia logs.
- Isolate: scale the Ingress to deny-all or apply a NetworkPolicy blocking
  8080 from outside; keep 9001 limited to Kuscia or block it too.
- Rotate exposed credentials ([credential-leak.md](credential-leak.md),
  [key-rotation.md](key-rotation.md)). JWT secret rotation without grace
  logs everyone out.
- Disable compromised accounts (delete or reset via admin API).
- Coordinate with peer institutions if cross-domain routes/jobs may be
  affected (they may need to pause routes to this node).

## 3. Investigate

1. Verify audit integrity **before** trusting it:

   ```bash
   # inside the pod/container image (binary shipped at /usr/local/bin/audit-verify)
   audit-verify -config /etc/privahub/privahub.yaml \
     -anchor-file <WORM anchor file> -archive-dir /app/audit/archive
   # exit 0 = chain intact, 2 = broken (prints first broken link), 1 = error
   ```
   A broken chain = the database was modified outside the application →
   escalate to SEV1 and rely on anchors/archives/backups for the timeline.
2. Reconstruct: logins (success/failure, IPs), password resets, role/permission
   changes, node/route/datasource changes, project/job creation, data
   downloads — correlated with access logs and Kuscia events.
3. Scope: which institutions, projects, datasets, credentials.

## 4. Eradicate & recover

- Patch / upgrade (rebuilt image by digest), redeploy from a clean image.
- Restore from a known-good backup if integrity is in doubt; re-apply
  legitimate changes made after the backup from the audit trail.
- Re-enable traffic gradually; watch auth-failure and error metrics.

## 5. Notify

- Peers and customers per contract.
- Personal-information incidents: legal decides on regulator / data-subject
  notification under applicable law (e.g. PIPL, GDPR 72h).
- Reporter (if external) per SECURITY.md SLA; publish a GHSA when fixed.

## 6. Post-incident

Blameless post-mortem within 5 business days: timeline, root cause, what
detected it, what slowed us down, actions (owner + due date). Update the
threat model and add a regression gate (CI check, test, alert) where possible.
