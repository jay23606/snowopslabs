# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, use one of:

- **GitHub Private Vulnerability Reporting**: the "Report a vulnerability" button
  under this repository's **Security** tab (preferred).
- Email **admin@snowops.net** with details.

Please include: a description, reproduction steps, affected version/commit, and
impact. If you have a suggested fix, include it.

## What to expect

- **Acknowledgement** within 3 business days.
- An initial assessment and severity rating shortly after.
- Coordinated disclosure: we'll agree on a timeline and credit you (if you wish)
  once a fix is available.

## Scope

In scope: the SnowOps Labs engine, the `labctl` CLI, the SDK (`pkg/`), and
first-party platform modules/scenarios in this repository.

Out of scope: vulnerabilities in the third-party tools SnowOps Labs orchestrates
(kubectl, helm, Istio, Vault, Prometheus, etc.) - report those upstream. Also out
of scope: issues that require a pre-compromised cluster or that are inherent to
running attacker-supplied content. Scenarios and incidents run shell scripts
against your cluster by design, so content loaded from an external root
(`SNOWOPS_CONTENT_PATH`) is trusted code, only point it at sources you
trust. See `docs/adr/0008-content-extensibility-seam.md`.

## Secrets in run output

Scripts receive labctl's own environment and the settings each run passes
them, so run output is masked before it is stored in the run log or streamed
to the CLI and UI. A value of 8 or more characters under a key ending in
`_PASSWORD`, `_PASSWD`, `_PASS`, `_TOKEN`, `_KEY`, `_SECRET`, `_CREDENTIALS`,
`_DSN`, `_WEBHOOK` or `_WEBHOOK_URL`, or containing `WEBHOOK`, is replaced with
`[redacted:NAME]`. A multi-line value, such as a PEM key, is also masked line
by line. Redaction is exact-match on text: it does not cover values under 8
characters, a secret split by the 64 KiB cut on a line with no newline, other
key names, secrets a script reads from the cluster or a file, or encoded forms
such as base64. The web UI's action events (command and error text) are masked
the same way. Treat the run log as readable by anyone
with access to the UI. See
[the run engine](docs/architecture/ARCHITECTURE.md#34-durable-replayable-logs).

## Supported versions

Until the first stable release, only the latest `main` is supported. A formal
supported-versions matrix will be published with the first tagged release (see
[RELEASING.md](RELEASING.md)).
