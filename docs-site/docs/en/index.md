---
title: inwx CLI
navTitle: Overview
section: Start
order: 10
description: Inspect and safely change INWX DNS records from a terminal or agent.
tags: [dns, cli, automation]
updated: 2026-07-26
---

# inwx CLI

`inwx` is a small command-line client for inspecting and changing DNS records
held at INWX. Human-readable tables support terminal work, while versioned JSON
supports scripts and agents.

> **Unofficial community project**
>
> This project is not affiliated with, endorsed by, maintained by, or supported
> by INWX GmbH. For official services and support, visit
> [inwx.com](https://www.inwx.com/).

## Safety at a glance

- All DNS commands live below `inwx dns`.
- A mutation first returns an exact preview and does not write.
- A write requires `--apply` and the preview's `--expect` token.
- Update and delete select a record only by its exact provider ID.
- Every write is followed by a complete API re-read and state verification.
- Passwords and TOTP secrets have no command-line flags.

## Command groups

```text
inwx version
inwx auth check
inwx dns zones list
inwx dns records list <zone>
inwx dns records create <zone> ...
inwx dns records update <zone> --id <id> ...
inwx dns records delete <zone> --id <id> ...
```

Start with [Installation](/en/installation), then configure
[Authentication](/en/authentication). Use [OT&E](/en/environments) for integration
work before operating against a production account.
