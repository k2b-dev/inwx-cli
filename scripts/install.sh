#!/bin/sh
# Install or update the unofficial community INWX DNS CLI.
#
# This project is not affiliated with, endorsed by, maintained by, or supported
# by INWX GmbH. Official services and support: https://www.inwx.com/

set -eu

REPOSITORY="k2b-dev/inwx-cli"
RELEASE_BASE="${INWX_INSTALL_RELEASE_BASE:-https://github.com/${REPOSITORY}/releases}"
API_BASE="${INWX_INSTALL_API_BASE:-https://api.github.com/repos/${REPOSITORY}}"
COSIGN="${INWX_INSTALL_COSIGN_BIN:-cosign}"
PREFIX="${HOME:?HOME is required}/.local/bin"
VERSION="latest"
SYSTEM=0
CUSTOM_PREFIX=0
ASSUME_YES=0
ALLOW_DOWNGRADE=0

die() {
    printf 'inwx installer: %s\n' "$*" >&2
    exit 1
}

have() {
    command -v "$1" >/dev/null 2>&1
}

usage() {
    cat <<'EOF'
Usage: install.sh [options]

Install or update the inwx DNS CLI.

  --system            install to /usr/local/bin (uses sudo when needed)
  --prefix=DIR        install to DIR (default: ~/.local/bin)
  --version=vX.Y.Z    install a specific stable release (default: latest)
  --allow-downgrade   allow an explicitly selected older release
  -y, --yes           confirm the install non-interactively
  -h, --help          show this help

The installer downloads only inwx. It never reads or stores INWX credentials.
It verifies checksums.txt with keyless Cosign identity bound to the exact
k2b-dev/inwx-cli release workflow and tag, then verifies the archive SHA-256.

This is an unofficial community installer. It is not affiliated with, endorsed
by, maintained by, or supported by INWX GmbH. Official services and support:
https://www.inwx.com/

Test-only environment:
  INWX_INSTALL_RELEASE_BASE  override the release download base
  INWX_INSTALL_API_BASE      override the GitHub API base
  INWX_INSTALL_COSIGN_BIN    override the cosign executable
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --system)
            SYSTEM=1
            PREFIX="/usr/local/bin"
            ;;
        --prefix=*)
            PREFIX=${1#--prefix=}
            CUSTOM_PREFIX=1
            ;;
        --version=*)
            VERSION=${1#--version=}
            ;;
        --allow-downgrade)
            ALLOW_DOWNGRADE=1
            ;;
        -y|--yes)
            ASSUME_YES=1
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            printf 'unknown option: %s\n\n' "$1" >&2
            usage >&2
            exit 2
            ;;
    esac
    shift
done

[ "$SYSTEM" = "0" ] || [ "$CUSTOM_PREFIX" = "0" ] ||
    die "--system and --prefix cannot be combined"
case "$PREFIX" in
    /*) ;;
    *) die "--prefix must be an absolute directory" ;;
esac
[ "$PREFIX" != "/" ] || die "--prefix must not be /"
if printf '%s' "$PREFIX" | LC_ALL=C grep '[[:cntrl:]]' >/dev/null 2>&1; then
    die "--prefix contains a control character"
fi

case "$VERSION" in
    latest) ;;
    v[0-9]*)
        printf '%s\n' "$VERSION" |
            grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' ||
            die "invalid version: $VERSION (expected vX.Y.Z)"
        ;;
    *) die "invalid version: $VERSION (expected vX.Y.Z)" ;;
esac

version_lt() {
    awk -v left="${1#v}" -v right="${2#v}" 'BEGIN {
        split(left, a, ".")
        split(right, b, ".")
        for (i = 1; i <= 3; i++) {
            if ((a[i] + 0) < (b[i] + 0)) exit 0
            if ((a[i] + 0) > (b[i] + 0)) exit 1
        }
        exit 1
    }'
}

confirm() {
    [ "$ASSUME_YES" = "1" ] && return 0
    if [ ! -t 0 ] && [ ! -r /dev/tty ]; then
        die "not a terminal; pass --yes to confirm non-interactively"
    fi
    printf '%s [y/N] ' "$1"
    if [ -r /dev/tty ]; then
        IFS= read -r answer < /dev/tty || answer=""
    else
        IFS= read -r answer || answer=""
    fi
    case "$answer" in
        y|Y|yes|YES) return 0 ;;
        *) return 1 ;;
    esac
}

have curl || die "curl is required"
have tar || die "tar is required"
have "$COSIGN" || die "cosign is required to authenticate releases"

REQUESTED_LATEST=0
if [ "$VERSION" = "latest" ]; then
    REQUESTED_LATEST=1
    RELEASE_JSON=$(curl -fsSL "${API_BASE}/releases/latest") ||
        die "could not resolve the latest stable release"
    VERSION=$(printf '%s\n' "$RELEASE_JSON" | awk '
        /"tag_name"[[:space:]]*:/ {
            value = $0
            sub(/^.*"tag_name"[[:space:]]*:[[:space:]]*"/, "", value)
            sub(/".*$/, "", value)
            print value
            exit
        }
    ')
    printf '%s\n' "$VERSION" |
        grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' ||
        die "latest release did not return a stable vX.Y.Z tag"
fi
VERSION_NUMBER=${VERSION#v}

CURRENT=""
if [ -e "$PREFIX/inwx" ]; then
    [ -x "$PREFIX/inwx" ] ||
        die "existing $PREFIX/inwx is not executable; refusing to replace it"
    CURRENT_OUTPUT=$("$PREFIX/inwx" version 2>/dev/null || true)
    CURRENT=$(printf '%s\n' "$CURRENT_OUTPUT" |
        awk 'NR == 1 && $1 == "inwx" {print $2}')
    case "$CURRENT" in
        [0-9]*)
            printf '%s\n' "$CURRENT" |
                grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || CURRENT=""
            ;;
        *) CURRENT="" ;;
    esac
    [ -n "$CURRENT" ] ||
        die "could not determine the existing inwx version; refusing to replace it"
fi

if [ -n "$CURRENT" ] && [ "$CURRENT" = "$VERSION_NUMBER" ]; then
    printf 'inwx %s is already installed at %s/inwx\n' "$CURRENT" "$PREFIX"
    exit 0
fi
if [ -n "$CURRENT" ] && version_lt "$VERSION_NUMBER" "$CURRENT"; then
    if [ "$REQUESTED_LATEST" = "1" ] || [ "$ALLOW_DOWNGRADE" != "1" ]; then
        die "refusing downgrade from $CURRENT to $VERSION_NUMBER; select an explicit version and pass --allow-downgrade"
    fi
fi

printf '\ninwx installer (unofficial community project)\n'
printf '  target:  %s/inwx\n' "$PREFIX"
if [ -n "$CURRENT" ]; then
    printf '  current: %s\n' "$CURRENT"
    printf '  target:  %s\n' "$VERSION_NUMBER"
    ACTION="upgrade"
    version_lt "$VERSION_NUMBER" "$CURRENT" && ACTION="downgrade"
else
    printf '  version: %s\n' "$VERSION_NUMBER"
    ACTION="install"
fi
printf '  verify:  SHA-256 + Cosign (exact release workflow and tag)\n\n'
confirm "Proceed with ${ACTION}?" || die "aborted"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
MACHINE=$(uname -m)
case "$OS" in
    linux|darwin) ;;
    *) die "unsupported OS: $OS" ;;
esac
case "$MACHINE" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) die "unsupported architecture: $MACHINE" ;;
esac

umask 077
TMP=$(mktemp -d "${TMPDIR:-/tmp}/inwx-install.XXXXXXXX") ||
    die "could not create a private temporary directory"
cleanup() {
    rm -rf "$TMP"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM

ARCHIVE="inwx_${OS}_${ARCH}.tar.gz"
DOWNLOAD_BASE="${RELEASE_BASE}/download/${VERSION}"
printf 'Downloading %s\n' "$ARCHIVE"
curl -fsSL "$DOWNLOAD_BASE/$ARCHIVE" -o "$TMP/$ARCHIVE" ||
    die "could not download $ARCHIVE for $VERSION"
curl -fsSL "$DOWNLOAD_BASE/checksums.txt" -o "$TMP/checksums.txt" ||
    die "missing checksums.txt"
curl -fsSL "$DOWNLOAD_BASE/checksums.txt.sig" -o "$TMP/checksums.txt.sig" ||
    die "missing checksums.txt.sig"
curl -fsSL "$DOWNLOAD_BASE/checksums.txt.pem" -o "$TMP/checksums.txt.pem" ||
    die "missing checksums.txt.pem"

IDENTITY_TAG=$(printf '%s' "$VERSION" | sed 's/\./\\./g')
"$COSIGN" verify-blob \
    --certificate "$TMP/checksums.txt.pem" \
    --signature "$TMP/checksums.txt.sig" \
    --certificate-identity-regexp "^https://github\\.com/k2b-dev/inwx-cli/\\.github/workflows/release\\.yml@refs/tags/${IDENTITY_TAG}$" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    "$TMP/checksums.txt" >/dev/null 2>&1 ||
    die "Cosign verification failed; refusing to install"

awk '
    NF != 2 ||
    length($1) != 64 ||
    $1 !~ /^[0-9A-Fa-f]+$/ ||
    $2 !~ /^\*?[A-Za-z0-9._-]+$/ { exit 1 }
    END { if (NR == 0) exit 1 }
' "$TMP/checksums.txt" || die "malformed checksums.txt"
MATCHES=$(awk -v archive="$ARCHIVE" '
    $2 == archive || $2 == "*" archive { count++; hash = $1 }
    END {
        if (count == 1) print hash
        else exit 1
    }
' "$TMP/checksums.txt") || die "archive must appear exactly once in checksums.txt"

if have sha256sum; then
    ACTUAL=$(sha256sum "$TMP/$ARCHIVE" | awk '{print $1}')
elif have shasum; then
    ACTUAL=$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print $1}')
else
    die "sha256sum or shasum is required"
fi
[ "$ACTUAL" = "$MATCHES" ] ||
    die "SHA-256 mismatch; refusing to install"

tar -xOzf "$TMP/$ARCHIVE" inwx > "$TMP/inwx" ||
    die "inwx is missing from the release archive"
tar -xOzf "$TMP/$ARCHIVE" LICENSE > "$TMP/LICENSE" ||
    die "LICENSE is missing from the release archive"
[ -s "$TMP/inwx" ] || die "release binary is empty"
chmod 0755 "$TMP/inwx"
BUILT_VERSION=$("$TMP/inwx" version 2>/dev/null |
    awk 'NR == 1 && $1 == "inwx" {print $2}')
[ "$BUILT_VERSION" = "$VERSION_NUMBER" ] ||
    die "release binary version does not match $VERSION"

install_atomic() {
    SOURCE=$1
    DESTINATION=$2
    DIRECTORY=$(dirname "$DESTINATION")
    if [ "$SYSTEM" = "1" ] && [ "$(id -u)" != "0" ]; then
        have sudo || die "sudo is required for --system"
        STAGED="${DESTINATION}.install.$$"
        sudo install -d -m 0755 "$DIRECTORY"
        sudo install -m 0755 "$SOURCE" "$STAGED" ||
            die "could not stage $DESTINATION"
        if ! sudo mv -f "$STAGED" "$DESTINATION"; then
            sudo rm -f "$STAGED"
            die "could not atomically install $DESTINATION"
        fi
    else
        install -d -m 0755 "$DIRECTORY"
        STAGED=$(mktemp "$DIRECTORY/.inwx.install.XXXXXXXX") ||
            die "could not stage installation in $DIRECTORY"
        if ! install -m 0755 "$SOURCE" "$STAGED"; then
            rm -f "$STAGED"
            die "could not stage $DESTINATION"
        fi
        if ! mv -f "$STAGED" "$DESTINATION"; then
            rm -f "$STAGED"
            die "could not atomically install $DESTINATION"
        fi
    fi
}

install_atomic "$TMP/inwx" "$PREFIX/inwx"
printf 'Installed inwx %s at %s/inwx\n' "$VERSION_NUMBER" "$PREFIX"

case ":${PATH:-}:" in
    *":$PREFIX:"*) ;;
    *)
        printf '\n%s is not in PATH. Add this to your shell configuration:\n' "$PREFIX"
        # shellcheck disable=SC2016
        printf '  export PATH="%s:$PATH"\n' "$PREFIX"
        ;;
esac
