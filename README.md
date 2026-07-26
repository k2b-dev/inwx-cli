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
read-only DNS inventory are implemented. Previewed mutations and release
packaging are still in progress.

Development is tracked in Dex:

```sh
dex list
```

Read the root epic and its ready child task before making changes. The current
plan uses Go, CGO-free release binaries, stable JSON output,
preview-before-apply mutations, INWX OT&E integration tests, a reusable skill
under `skills/inwx`, a Fibel documentation site, and a cryptographically
verified installer/updater modeled on the fd0.sh installation flow.

## Planned v0.1 commands

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

Global flags precede the command. Run `./inwx --help` for the complete
read-only command surface.

## Verification

```sh
gofmt -w cmd internal
go vet ./...
go test ./...
go test -race ./...
```

Tests use local HTTP fakes. Real integration work is restricted to INWX OT&E;
automated tests never mutate production.
