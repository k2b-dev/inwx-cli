---
title: Security and limitations
navTitle: Security
section: Operate
order: 90
description: Understand the CLI trust boundaries, mutation safeguards, and deliberate non-goals.
tags: [security, limitations, scope]
updated: 2026-07-26
---

# Security and limitations

## Credential boundary

- Credentials come only from environment or `_FILE` variables.
- Password and TOTP flags do not exist.
- Loaded credentials and generated TOTP codes are redacted from errors.
- The HTTP client uses a private cookie jar and rejects redirects.
- Credential files and `.env` files do not belong in the repository.

The CLI cannot prevent a shell history entry when a DNS value is supplied with
`--value`. Use `--value-file` or `--value-stdin` for transient values, and
protect preview output when the exact diff is sensitive.

## Mutation boundary

- Preview is the default.
- Apply requires both `--apply` and the fresh `--expect` token.
- Update and delete use exact provider IDs.
- A mutation request is never automatically retried.
- Full-zone re-read verification follows every attempted write.
- Provider-managed and unsupported record types are protected from mutation.

These safeguards do not make separate API calls transactional. Another actor
can change DNS between calls; stale state fails closed when it affects the
precondition.

## Deliberate limitations

The CLI manages individual DNS records and lists zones. It does not provide:

- domain registration, transfer, contact, billing, or account administration;
- DNSSEC, `NS`, or `SOA` mutation;
- bulk desired-state files or reconciliation;
- a daemon, database, ACME controller, or plugin framework;
- arbitrary API endpoints;
- automatic authoritative DNS or recursive propagation guarantees.

The tool is not a Terraform replacement. New registrar or account capabilities
require separate product design and are not implied by the command hierarchy.

## Project independence

`inwx` is an unofficial community project. It is not affiliated with, endorsed
by, maintained by, or supported by INWX GmbH. Official services and support
are available at [inwx.com](https://www.inwx.com/).
