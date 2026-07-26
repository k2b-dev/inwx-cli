# inwx-cli

A small, robust CLI and agent skill for managing INWX services.

> [!IMPORTANT]
> This is an unofficial community project. It is not affiliated with, endorsed
> by, maintained by, or supported by INWX GmbH. For official INWX services and
> support, visit [inwx.com](https://www.inwx.com/).

The repository is named `inwx-cli`; the executable is named `inwx`. The first
release focuses on safe DNS record inspection and mutation through commands
under `inwx dns`. The command hierarchy intentionally leaves room for other
INWX capabilities without implementing them speculatively.

## Status

Planning only. No CLI implementation exists yet.

Development is tracked in Dex:

```sh
dex list
```

Read the root epic and its ready child task before making changes. The current
plan uses Go, CGO-free release binaries, the maintained `libdns/inwx` provider,
stable JSON output, preview-before-apply mutations, INWX OT&E integration tests,
a reusable skill under `skills/inwx`, a Fibel documentation site, and a
cryptographically verified installer/updater modeled on the fd0.sh installation
flow.
