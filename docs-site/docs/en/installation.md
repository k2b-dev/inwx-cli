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

## Verified installer

Install or update the latest stable release:

```sh
curl -fsSL https://raw.githubusercontent.com/k2b-dev/inwx-cli/main/scripts/install.sh | sh
```

Pin both the installer source and selected binary release:

```sh
curl -fsSL https://raw.githubusercontent.com/k2b-dev/inwx-cli/v0.1.0/scripts/install.sh | sh -s -- --version=v0.1.0
```

Review a downloaded script before running it when your environment requires
that policy. The installer supports Linux and macOS on amd64 and arm64. It
installs only `inwx` into `~/.local/bin`; running it again is the update path.
It never asks for, reads, stores, or migrates INWX credentials.

`curl`, `tar`, and
[Cosign](https://docs.sigstore.dev/cosign/system_config/installation/) are
required. The installer authenticates `checksums.txt` against the exact
`k2b-dev/inwx-cli` release workflow and selected tag, verifies the archive
SHA-256, checks the binary version, and replaces an existing binary atomically.
Any failed download or verification leaves the previous binary unchanged.

Use `--system` for `/usr/local/bin`, `--prefix=DIR` for another absolute
directory, `--version=vX.Y.Z` for an exact stable release, and `--yes` for a
non-interactive confirmation. An explicit older release also requires
`--allow-downgrade`; `latest` resolution cannot downgrade.

This is an unofficial community installer. It is not affiliated with, endorsed
by, maintained by, or supported by INWX GmbH.

## Manual release archive

Releases are published at
[github.com/k2b-dev/inwx-cli/releases](https://github.com/k2b-dev/inwx-cli/releases).
Select the archive for `linux` or `darwin` and `amd64` or `arm64`, verify it
against the release checksum and signature materials, then place `inwx` on
`PATH`.

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
