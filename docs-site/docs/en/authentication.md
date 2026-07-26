---
title: Authentication
navTitle: Authentication
section: Configure
order: 30
description: Supply INWX credentials without storing them in the repository or command arguments.
tags: [credentials, files, totp]
updated: 2026-07-26
---

# Authentication

`inwx` reads credentials from the environment. It does not accept passwords or
TOTP shared secrets as command-line flags and does not load `.env` files.

## Credential variables

| Value | Direct variable | File variable |
| --- | --- | --- |
| Username | `INWX_USERNAME` | `INWX_USERNAME_FILE` |
| Password | `INWX_PASSWORD` | `INWX_PASSWORD_FILE` |
| TOTP shared secret | `INWX_SHARED_SECRET` | `INWX_SHARED_SECRET_FILE` |

For each value, set either the direct variable or its `_FILE` alternative,
never both. The file must be regular, no larger than 64 KiB, and readable by
the current process. One trailing LF or CRLF is removed.

File variables keep credential values out of process arguments and the
environment:

```sh
export INWX_USERNAME_FILE="$HOME/.config/inwx/username"
export INWX_PASSWORD_FILE="$HOME/.config/inwx/password"
export INWX_SHARED_SECRET_FILE="$HOME/.config/inwx/shared-secret"
inwx --environment ote auth check
```

The shared secret is needed only when INWX reports the `GOOGLE-AUTH` two-factor
method. TOTP codes are generated locally and are redacted from failures.

## External secret managers

A secret manager can expose protected temporary files and set the `_FILE`
variables for the command. Secret values must not be interpolated into a shell
command, written to repository files, or passed through `--value`.

## Check authentication

```sh
inwx --environment ote auth check
```

A successful check prints the selected environment. Authentication,
configuration, and API failures go to standard error.
