---
title: Troubleshooting
navTitle: Troubleshooting
section: Operate
order: 80
description: Diagnose credential, environment, conflict, verification, and DNS propagation failures.
tags: [errors, conflicts, dns]
updated: 2026-07-26
---

# Troubleshooting

## Credentials work on inwx.de but not OT&E

Production and OT&E are separate systems. Register and confirm an OT&E account,
then provide its credentials while selecting `--environment ote`.

## Both direct and file variables are set

Unset one source. The CLI refuses ambiguous precedence:

```sh
unset INWX_PASSWORD
export INWX_PASSWORD_FILE="$HOME/.config/inwx/password"
```

## Apply reports a conflict

The zone changed after the preview, the selected ID disappeared, or another
record now violates a duplicate or CNAME constraint. No write was sent. Re-read
the complete zone, inspect the change, and generate a new preview. Never reuse
the old token.

## A write returns an API error

Mutation requests are not retried. The CLI performs a re-read and reports a
verified recovered result only when the desired state can be identified
unambiguously. Otherwise, inspect the current zone before taking another
action.

## Post-verification fails

Exit 6 means the mutation call returned success but the requested state was
not observed during the control-plane re-read. Treat the result as uncertain.
Read the zone again before deciding whether another operation is safe.

## The API state is correct but DNS still differs

API verification and DNS propagation are different checks. Query the
authoritative nameservers, inspect TTLs, and allow for propagation. Do not
repeat a mutation solely because a recursive resolver still has cached data.

## A record is visible but cannot be changed

Unfiltered inventory preserves unsupported and provider-managed record types.
The mutation commands intentionally reject types outside `A`, `AAAA`, `CNAME`,
`TXT`, and `MX`.
