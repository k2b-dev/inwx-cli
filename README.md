# inwx-cli

A small, robust CLI and agent skill for managing INWX services.

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
and a reusable skill under `skills/inwx`.
