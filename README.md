# inwx-cli

A small, robust CLI and agent skill for managing DNS records at INWX.

> [!IMPORTANT]
> This is an unofficial community project. It is not affiliated with, endorsed
> by, maintained by, or supported by INWX GmbH. For official INWX services and
> support, visit [inwx.com](https://www.inwx.com/).

The repository is named `inwx-cli`; the executable is named `inwx`. The first
release focuses on safe DNS record inspection and mutation through commands
under `inwx dns`. It does not implement domain registration, contacts,
transfers, billing, or account administration.

## Status

The v0.1 command and provider contract is locked in
[`docs/architecture-v0.1.md`](docs/architecture-v0.1.md). Authentication and
DNS inventory and preview-before-apply record mutations are implemented.
Release packaging is still in progress.

Development is tracked in Dex:

```sh
dex list
```

Read the root epic and its ready child task before making changes. The current
plan uses Go, CGO-free release binaries, stable JSON output,
preview-before-apply mutations, INWX OT&E integration tests, a reusable skill
under `skills/inwx`, a Fibel documentation site, and a cryptographically
verified installer/updater modeled on the fd0.sh installation flow.

## v0.1 commands

```text
inwx version
inwx auth check
inwx dns zones list
inwx dns records list <zone>
inwx dns records create <zone> ...
inwx dns records update <zone> --id <id> ...
inwx dns records delete <zone> --id <id> ...
```

Mutations are two-step operations. A preview returns an exact diff and
precondition token without changing anything. Applying that diff requires both
`--apply` and the returned `--expect` token. The CLI then re-reads INWX and
verifies the result.

Credentials are read only from `INWX_USERNAME`, `INWX_PASSWORD`,
`INWX_SHARED_SECRET`, or their `_FILE` variants. Passwords and TOTP secrets are
never accepted as command-line arguments.

## Development build

Go 1.24 or newer is required:

```sh
CGO_ENABLED=0 go build -o inwx ./cmd/inwx
```

Keep credentials outside the repository. `_FILE` variables avoid placing
secret values in the environment itself:

```sh
export INWX_USERNAME_FILE="$HOME/.config/inwx-cli/ote/username"
export INWX_PASSWORD_FILE="$HOME/.config/inwx-cli/ote/password"

./inwx --environment ote auth check
./inwx --environment ote dns zones list
./inwx --json --environment ote dns records list example.com
```

Preview a change without writing:

```sh
./inwx --json --environment ote dns records create example.com \
  --type A --name www --value 192.0.2.1
```

The response contains the exact before/after state and an `expect` token.
Apply only that preview by repeating the command with both safeguards:

```sh
./inwx --json --environment ote dns records create example.com \
  --type A --name www --value 192.0.2.1 \
  --expect '<token-from-preview>' --apply
```

Update and delete require the exact provider record ID returned by `records
list`; names, types, and values never select a destructive target. The CLI
re-reads the complete zone after a write and fails if the requested state is
not observed.

Use `--value-file` or `--value-stdin` instead of `--value` when a transient DNS
value must not appear in process arguments. Global flags precede the command.
Run `./inwx --help` for the complete command surface.

## Verification

```sh
gofmt -w cmd internal
go vet ./...
go test ./...
go test -race ./...
```

Tests use local HTTP fakes. Real integration work is restricted to INWX OT&E;
automated tests never mutate production.

An opt-in CRUD test requires an existing disposable OT&E zone:

```sh
INWX_ENVIRONMENT=ote \
INWX_INTEGRATION=1 \
INWX_TEST_ZONE=disposable-zone.example \
go test ./internal/cli -run '^TestOTERecordCRUD$' -count=1 -v
```

The test uses the normal credential environment or `_FILE` sources, creates a
unique TXT record, updates it, deletes it, and registers cleanup before the
first update. It refuses to run unless `INWX_ENVIRONMENT=ote`.
