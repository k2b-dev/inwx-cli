---
title: Change DNS safely
navTitle: Safe changes
section: DNS
order: 60
description: Preview, authorize, apply, and verify one exact DNS record change.
tags: [preview, apply, expect]
updated: 2026-07-26
---

# Change DNS safely

Each command changes at most one record. A preview and an apply are separate
API sessions; they are not presented as a transaction.

## 1. Read current state

```sh
inwx --json --environment ote dns records list example.com --name www
```

For update or delete, retain the exact `id` from this response.

## 2. Preview the exact change

Create:

```sh
inwx --json --environment ote dns records create example.com \
  --type A --name www --value 192.0.2.10 --ttl 3600
```

Update:

```sh
inwx --json --environment ote dns records update example.com \
  --id 12345 --value 192.0.2.20 --ttl 3600
```

Delete:

```sh
inwx --json --environment ote dns records delete example.com --id 12345
```

The preview returns canonical `before` and `after` records, `applied: false`,
and a lower-case SHA-256 `expect` token. It performs no mutation.

## 3. Authorize and apply that preview

Review the exact environment, zone, provider ID, before state, and after state.
After the specific change is authorized, repeat the same command with the
fresh token:

```sh
inwx --json --environment ote dns records update example.com \
  --id 12345 --value 192.0.2.20 --ttl 3600 \
  --expect '<token-from-preview>' --apply
```

The CLI re-fetches the zone and compares the token in constant time. Concurrent
drift causes exit 5 before a write. Mutation requests are submitted once and
are never retried automatically.

## 4. Verify

After the write, the CLI re-reads the complete zone and requires the requested
state. The successful response contains `applied: true`, `verified: true`, and
the final record or deleted ID.

API verification does not claim immediate DNS propagation. Query an
authoritative server separately:

```sh
dig NS example.com
dig @authoritative-server.example A www.example.com +short
```

## Value sources

Use exactly one source for create and at most one for update:

```sh
inwx --json --environment ote dns records create example.com \
  --type TXT --name _verification --value-file /secure/path/value

printf '%s' 'transient-value' |
  inwx --json --environment ote dns records create example.com \
    --type TXT --name _verification --value-stdin
```

`--value-file` and `--value-stdin` keep transient values out of process
arguments. The exact value still appears in the required preview diff on
standard output; handle that output as sensitive when appropriate.

## Supported mutations

Create and update normalize `A`, `AAAA`, `CNAME`, `TXT`, and `MX`. TTL must be
between 300 and 2147483647. MX requires priority from 0 through 65535;
priority is invalid for other supported types. CNAME coexistence and canonical
duplicates fail closed.

Delete and update require an exact ID. `NS`, `SOA`, DNSSEC, unsupported record
types, fuzzy deletion, and broad zone changes are outside this command surface.
