# inwx

`inwx` is an unofficial command-line tool for inspecting and changing DNS
records in an INWX account. It provides stable human and JSON output and makes
every write a separate preview-and-apply operation.

> [!IMPORTANT]
> This is an unofficial community project. It is not affiliated with, endorsed
> by, maintained by, or supported by INWX GmbH. For official INWX services and
> support, visit [inwx.com](https://www.inwx.com/).

Version 0.1 manages individual `A`, `AAAA`, `CNAME`, `TXT`, and `MX` records
under `inwx dns`. It does not register or transfer domains, manage contacts or
payments, or change NS, SOA, or DNSSEC settings.

## Install

The verified installer supports Linux and macOS on amd64 and arm64. It requires
`curl`, `tar`, and
[Cosign](https://docs.sigstore.dev/cosign/system_config/installation/).

Install the latest stable release:

```sh
curl -fsSL https://raw.githubusercontent.com/k2b-dev/inwx-cli/main/scripts/install.sh | sh
```

To pin both the installer and binary to v0.1.0:

```sh
curl -fsSL https://raw.githubusercontent.com/k2b-dev/inwx-cli/v0.1.0/scripts/install.sh | sh -s -- --version=v0.1.0
```

The installer verifies the release workflow identity, signature, checksum, and
binary version before atomically installing `inwx` to `~/.local/bin`. It never
reads or stores INWX credentials. Run the same command again to update.

Confirm that `~/.local/bin` is on `PATH`, then check the installation:

```sh
inwx version
inwx --help
```

See [installation options](docs-site/docs/en/installation.md) for custom
prefixes, system-wide installation, and downgrade protection.

## Configure credentials

`inwx` reads credentials from environment variables or protected files. File
variables keep secret values out of command arguments and the process
environment.

Store your username and password in files outside the repository, then export
their paths:

```sh
export INWX_USERNAME_FILE="$HOME/.config/inwx/username"
export INWX_PASSWORD_FILE="$HOME/.config/inwx/password"
```

If the account uses INWX `GOOGLE-AUTH` two-factor authentication, also set:

```sh
export INWX_SHARED_SECRET_FILE="$HOME/.config/inwx/shared-secret"
```

For each credential, use either the direct variable (`INWX_USERNAME`,
`INWX_PASSWORD`, `INWX_SHARED_SECRET`) or its `_FILE` variant, never both.
`inwx` does not read `.env` files and never accepts passwords or TOTP secrets as
flags.

See [authentication](docs-site/docs/en/authentication.md) for file requirements
and secret-manager usage.

## Check your connection

Select the environment explicitly:

- `ote` is the separate INWX test environment with its own account and data.
- `production` is the live INWX account at inwx.com.

Authenticate without changing anything:

```sh
inwx --environment ote auth check
```

For a live account, replace `ote` with `production`. A successful command prints
`authenticated: true` in JSON mode:

```sh
inwx --json --environment ote auth check
```

## Inspect DNS

List the zones available in the selected account:

```sh
inwx --environment ote dns zones list
```

List every record in one zone:

```sh
inwx --environment ote dns records list example.com
```

Use JSON for scripts and agents, and optionally filter by record type or name:

```sh
inwx --json --environment ote dns records list example.com \
  --type A --name www
```

The JSON schema is versioned as `inwx.cli/v1`. Existing provider-managed records
such as NS and SOA are visible but cannot be changed by v0.1.

## Change one DNS record safely

Every create, update, or delete has two steps. The first command only previews
the exact `before` and `after` state. No write occurs without both a fresh
`--expect` token and `--apply`.

### 1. Preview

```sh
inwx --json --environment ote dns records create example.com \
  --type A --name www --value 192.0.2.10 --ttl 3600
```

Review the environment, zone, record, and diff in the response. Copy the
returned `expect` token.

### 2. Apply that exact preview

Repeat the unchanged command with the token:

```sh
inwx --json --environment ote dns records create example.com \
  --type A --name www --value 192.0.2.10 --ttl 3600 \
  --expect '<token-from-preview>' --apply
```

The CLI fetches the current state again, refuses stale previews, submits the
write once, and re-reads the complete zone. Success includes `applied: true`
and `verified: true`.

### Update or delete by exact ID

Read the current records first and use the provider `id` from that response:

```sh
inwx --json --environment ote dns records update example.com \
  --id 12345 --value 192.0.2.20 --ttl 3600

inwx --json --environment ote dns records delete example.com --id 12345
```

These are previews too. Review each diff, then repeat only the intended command
with its fresh `--expect` token and `--apply`. Names and values never select a
destructive target.

For TXT values or other data that should not appear in process arguments, use
`--value-file` or `--value-stdin`:

```sh
inwx --json --environment ote dns records create example.com \
  --type TXT --name _verification --value-file /secure/path/value
```

The value still appears in the required preview output, so handle that output
appropriately. API verification confirms the INWX account state; it does not
claim that DNS caches have already expired. See the complete
[safe mutation workflow](docs-site/docs/en/dns-mutations.md).

## Command overview

```text
inwx version
inwx auth check
inwx dns zones list
inwx dns records list <zone>
inwx dns records create <zone> ...
inwx dns records update <zone> --id <id> ...
inwx dns records delete <zone> --id <id> ...
```

Global flags must precede the command:

```text
--json
--environment production|ote
--timeout 20s
--retries 2
```

Run `inwx --help` or a command with `--help` for the exact current syntax.
Detailed guides cover [environment selection](docs-site/docs/en/environments.md),
[DNS reads](docs-site/docs/en/dns-read.md),
[JSON and exit codes](docs-site/docs/en/json-and-exit-codes.md), and
[troubleshooting](docs-site/docs/en/troubleshooting.md).

## For contributors

Go 1.25 or newer is required. The CLI is built without CGO:

```sh
CGO_ENABLED=0 go build -o inwx ./cmd/inwx
go vet ./...
go test ./...
go test -race ./...
```

Installer tests use local HTTP fakes. Real integration tests run only against
INWX OT&E and require an existing disposable OT&E zone; automated tests never
mutate production. Architecture and contributor-level behavior are documented
in [docs/architecture-v0.1.md](docs/architecture-v0.1.md).
