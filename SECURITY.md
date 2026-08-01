# Security Policy

## Supported versions

This project doesn't yet have tagged releases — security fixes land on `main`. Always run the latest commit.

## Reporting a vulnerability

Please **don't** open a public GitHub issue for a security vulnerability. Instead, email adish.madhu@gmail.com with:

- A description of the issue and its impact
- Steps to reproduce
- Affected version/commit

Expect an initial response within a few days.

## Design context for reports

Some things below are intentional, documented tradeoffs of a single-user, localhost-bound tool — not bugs on their own, but worth knowing before filing:

- **No authentication.** Anyone who can reach the listening address (default `127.0.0.1:8090`, not exposed to the network) can trigger workflow runs — which is arbitrary code execution via Docker — and manage secrets. If you find a way to reach it from off-box despite the `127.0.0.1` bind, or a way to escalate beyond what an authenticated local user already could do, that's worth reporting.
- **Secrets at rest**: encrypted with AES-256-GCM; the key lives at `$XDG_CONFIG_HOME/local-action/seed.key` (`0600`), generated on first run.
- **Secrets in use**: decrypted values are written to a short-lived temp dotenv file (`0600`) only for the duration of a run, then deleted.

Reports that these boundaries can be crossed (key extraction without local file access, secret leakage after a run completes, binding escaping `127.0.0.1` unexpectedly, etc.) are exactly what this policy is for.
