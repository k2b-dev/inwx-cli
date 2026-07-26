---
title: Installation
navTitle: Installation
section: Start
order: 20
description: Install a release binary or build the inwx CLI from source.
tags: [install, release, build]
updated: 2026-07-26
---

# Installation

## Release archive

Releases are published at
[github.com/k2b-dev/inwx-cli/releases](https://github.com/k2b-dev/inwx-cli/releases).
Select the archive for `linux` or `darwin` and `amd64` or `arm64`, verify it
against the release checksum and signature materials, then place `inwx` on
`PATH`.

The verified installer and its canonical one-line command are documented with
the release that first includes it. The installer installs only the CLI; it
does not read or migrate INWX credentials.

## Build from source

Go 1.24 or newer is required.

```sh
git clone https://github.com/k2b-dev/inwx-cli.git
cd inwx-cli
CGO_ENABLED=0 go build -o inwx ./cmd/inwx
./inwx version
```

Move the resulting binary to a directory on `PATH`, for example:

```sh
install -d "$HOME/.local/bin"
install -m 0755 ./inwx "$HOME/.local/bin/inwx"
```

## Confirm the installation

```sh
inwx --help
inwx version
```

The root help must identify the project as an unofficial community CLI and
show only the DNS-focused command surface.
