---
title: Production and OT&E
navTitle: Environments
section: Configure
order: 40
description: Select the fixed INWX production or Operational Test and Evaluation endpoint.
tags: [production, ote, testing]
updated: 2026-07-26
---

# Production and OT&E

The CLI supports two fixed INWX API environments:

| Name | Purpose | API endpoint |
| --- | --- | --- |
| `production` | Live customer data and public DNS | `https://api.domrobot.com/jsonrpc/` |
| `ote` | INWX Operational Test and Evaluation | `https://api.ote.domrobot.com/jsonrpc/` |

OT&E is a separate test system. Production accounts, credentials, zones, and
records do not automatically exist there. Create and confirm an OT&E account
before running an integration test.

Select an environment with the global flag:

```sh
inwx --environment ote auth check
inwx --environment ote dns zones list
```

Or set a default:

```sh
export INWX_ENVIRONMENT=ote
inwx dns zones list
```

The flag takes precedence over `INWX_ENVIRONMENT`. If neither is present, the
CLI selects `production`. Automated mutation tests must set OT&E explicitly;
the repository's opt-in integration test refuses any other environment.

Arbitrary API endpoint flags are intentionally unavailable. Tests inject a
local HTTP server through internal code, not through the public command line.
