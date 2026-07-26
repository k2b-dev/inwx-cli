---
title: JSON and exit codes
navTitle: JSON and exits
section: Automation
order: 70
description: Consume stable JSON envelopes and distinguish CLI failure classes.
tags: [json, schema, exit-codes]
updated: 2026-07-26
---

# JSON and exit codes

Place `--json` before the command:

```sh
inwx --json --environment ote dns zones list
```

Successes use a versioned envelope:

```json
{
  "schema_version": "inwx.cli/v1",
  "command": "dns.zones.list",
  "environment": "ote",
  "ok": true,
  "data": {
    "zones": []
  }
}
```

Empty collections are JSON arrays, not `null`. Records use canonical fields:
`id`, `zone`, `name`, `fqdn`, `type`, `value`, `ttl`, and optional `priority`.

Failures use the same schema version and write only to standard error:

```json
{
  "schema_version": "inwx.cli/v1",
  "command": "dns.records.update",
  "environment": "ote",
  "ok": false,
  "error": {
    "kind": "conflict",
    "message": "state changed since preview; run a fresh preview"
  }
}
```

## Exit codes

| Code | Meaning |
| ---: | --- |
| `0` | Success, including a mutation preview or a no-op |
| `2` | Invalid command or flags |
| `3` | Configuration or authentication failure |
| `4` | INWX API, HTTP, timeout, response, or output failure |
| `5` | Mutation precondition conflict |
| `6` | Mutation succeeded but post-verification failed |
| `130` | Interrupted by `SIGINT` |

Scripts should evaluate the exit code and `ok`, then inspect `error.kind`.
Human-readable diagnostics also go to standard error.
