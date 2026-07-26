---
name: inwx
description: Safely inspect INWX accounts and manage individual INWX DNS records with the inwx CLI. Use this skill whenever a user asks about INWX authentication, zones, DNS records, OT&E, or an INWX DNS change, even if they do not name the CLI. It enforces exact-ID targeting, preview and explicit authorization before apply, drift handling, secret-safe credential injection, API re-read, and authoritative DNS verification.
compatibility: Requires the inwx CLI and external INWX credentials supplied through environment or _FILE variables.
---

# INWX DNS operations

Use `inwx` as an unofficial community CLI. It is not affiliated with,
endorsed by, maintained by, or supported by INWX GmbH.

## Establish the boundary

1. Run `inwx --help` and the relevant nested `--help` before constructing a
   command. Help from the installed binary is the syntax source of truth.
2. Select `--environment ote` or `--environment production` explicitly.
   Never rely on the production default while deciding what data may change.
3. Keep credentials outside the repository and conversation. Accept them only
   through an external secret manager that exposes `INWX_*_FILE` variables, or
   through already-configured environment variables. Never ask for, echo, log,
   or place a password or TOTP secret in command arguments.
4. Run `inwx --environment <environment> auth check`.

If credentials are missing, stop and ask the user to configure them externally.
Do not create a `.env` file or improvise another credential store.

## Read current state

Use `--json` for decisions:

1. List zones when the exact zone is not established.
2. List the complete selected zone before filtering or proposing a change.
3. Identify records by the provider `id`. Names, types, and values provide
   context but never select a destructive target.
4. If zero or multiple records could satisfy the request, explain the
   ambiguity and stop. Do not pick the first match or clean up related records.

Read-only requests may proceed without mutation authorization. Return the
selected environment, canonical zone, relevant records, and any ambiguity.
If execution or network access is unavailable, say so and provide the exact
read-only commands that would be run; do not omit the command plan or imply
that an inventory was observed.

## Authorize one exact change

Before a mutation, explain a deterministic proposed diff containing:

- environment and canonical zone;
- operation;
- exact provider ID for update or delete;
- canonical before state, or absence for create;
- canonical requested state, or absence for delete;
- the one record affected and the fields that remain unchanged.

Obtain explicit user authorization for that exact change. Broad approval such
as "fix DNS" does not authorize a write. Delete, MX, and broad zone changes
need particularly clear authorization because they can disrupt service.
For delete, the user must select one exact provider ID and authorize its exact
complete before state. Never infer which matching record is stale from its
name, value, TTL, age, order, or apparent purpose.

`NS`, `SOA`, and DNSSEC mutation are unsupported in v0.1. Refuse them without
inventing another command or API call. Do not add registrar, domain, or account
operations.

## Preview and apply

After authorization:

1. Run the mutation command without `--apply`.
2. Inspect its JSON `before`, `after`, `environment`, `zone`, `applied`, and
   `expect` fields. It must report `applied: false`.
3. If the preview differs from the authorized diff, stop and request renewed
   authorization for the actual diff.
4. Repeat the identical mutation command with the fresh
   `--expect <token> --apply`.

Do not reuse an old token. Exit 5 means concurrent drift or another
precondition conflict: re-read the complete zone with `--json`, identify the
exact provider ID, explain the new state, and obtain renewed authorization if
the diff changed. Never bypass the token or retry a mutation blindly.

For transient TXT or other sensitive DNS values, use `--value-file` or
`--value-stdin`; never place the value in process arguments. The exact preview
still contains the value, so treat captured stdout as sensitive.

## Verify

1. Require `verified: true` from apply output.
2. Independently re-run the complete zone listing and match the exact ID and
   canonical requested state, or confirm the deleted ID is absent.
3. Query the authoritative DNS server for the exact FQDN and type. Use the
   authoritative server appropriate to the selected environment; do not invent
   one. Report API verification separately from DNS propagation.
4. Report the exact change, API result, authoritative observation, and any
   propagation uncertainty. Never claim unrelated records were validated or
   changed.

If the CLI reports exit 6 or an uncertain mutation result, stop. Re-read state
before proposing any recovery, and never submit a second write merely because
the first response was lost.
