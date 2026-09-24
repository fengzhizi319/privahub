# Security Policy

Privahub is the management console (Go backend + web console *privaconsole*)
of a privacy-preserving computing platform. It holds user credentials, node
certificates, datasource credentials and orchestrates cross-institution jobs
through Kuscia, so we treat vulnerability reports as a priority.

## Reporting a vulnerability

**Do not open a public GitHub issue, pull request or discussion for a
suspected vulnerability.**

Report privately through one of:

1. GitHub private vulnerability reporting:
   `https://github.com/fengzhizi319/privahub/security/advisories/new`
2. E-mail: `security@<PLACEHOLDER-DOMAIN>` (TODO: replace with the real
   security mailbox before the first public release). Encrypt with our PGP
   key: `<PLACEHOLDER-PGP-FINGERPRINT>` (TODO).

Please include: affected version / commit, deployment mode (master / lite /
autonomy, profile), reproduction steps or PoC, impact, and whether the issue
is already public. Never include real personal data or production secrets.

## Our commitments (SLA)

| Stage | Target |
|---|---|
| Acknowledge receipt | 2 business days |
| Initial triage + severity (CVSS v3.1/v4) | 5 business days |
| Fix available — Critical | 7 days |
| Fix available — High | 30 days |
| Fix available — Medium | 90 days |
| Fix available — Low | next regular release |
| Public advisory (GHSA / CVE) | with or after the fix; coordinated with the reporter (default embargo max 90 days) |

We keep the reporter informed at least every 7 days until resolution and
credit reporters in the advisory unless they prefer otherwise.

## Scope

In scope:

- this repository (`cmd/`, `internal/`, `pkg/`, `config/`, `deployments/`);
- the web console repository *privaconsole* (mounted at `web/`);
- the published container image and the manifests in `deployments/`;
- security-relevant defaults (e.g. a default config that is insecure in a
  production profile).

Out of scope (report upstream instead):

- Kuscia, SecretFlow, SPU/HEU, Envoy and other upstream components — unless
  the vulnerability is caused by how Privahub configures or calls them;
- findings that require `PRIVAHUB_PROFILE=dev|test` (these profiles
  deliberately relax checks and must never be used in production);
- missing hardening without a demonstrated impact (e.g. banner disclosure),
  volumetric DoS, and results of automated scanners without a PoC;
- social engineering, physical attacks, and attacks requiring a compromised
  host / root on the node.

## Safe harbour

We will not pursue legal action for good-faith research that respects this
policy: test only against your own deployment, avoid privacy violations and
service degradation, do not access data that is not yours, and give us
reasonable time to fix before disclosure.

## Supported versions

Security fixes are provided for the latest minor release and the `main`
branch. Older releases receive fixes for Critical issues only, on a
best-effort basis.

## Security documentation

- Threat model: [docs/security/threat-model.md](docs/security/threat-model.md)
- Production hardening checklist: [docs/security/hardening-guide.md](docs/security/hardening-guide.md)
- Runbooks: [credential leak](docs/security/runbooks/credential-leak.md),
  [key rotation](docs/security/runbooks/key-rotation.md),
  [incident response](docs/security/runbooks/incident-response.md),
  [break-glass admin reset](docs/security/runbooks/break-glass-admin-reset.md)
- CI security gates: `.github/workflows/ci.yml`, locally `make check` / `make sec`
  (see [docs/security/hardening-guide.md#development-gates](docs/security/hardening-guide.md#development-gates)).
