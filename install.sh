#!/usr/bin/env bash
# Gozar installer.
#
#   curl -fsSL https://raw.githubusercontent.com/amor7/gozar/main/install.sh | sudo bash
#
# Downloads the gozar binary for this machine, checks it against the release's
# checksums, puts it in /usr/local/bin and starts the setup wizard. Nothing
# else is installed: no packages, no containers, no runtime.
#
# Environment:
#   GOZAR_VERSION=v1.2.0      a specific release instead of the latest
#   GOZAR_MIRROR=https://...  where release files are fetched from (a mirror
#                             that serves the same file names), for gates that
#                             reach GitHub slowly
#   GOZAR_BINARY=/path/file   install a binary you already downloaded
set -euo pipefail

REPO="amor7/gozar"
BIN=/usr/local/bin/gozar

c() { [ -t 1 ] && printf '\033[%sm' "$1" || true; }
say()  { printf '%s  %s%s\n' "$(c '38;5;141')▸$(c 0)" "$*" ""; }
die()  { printf '%serror:%s %s\n' "$(c 31)" "$(c 0)" "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "run as root: curl -fsSL ... | sudo bash"
[ "$(uname -s)" = Linux ] || die "Gozar runs on Linux servers"

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported CPU: $(uname -m)" ;;
esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if [ -n "${GOZAR_BINARY:-}" ]; then
  say "installing $GOZAR_BINARY"
  cp "$GOZAR_BINARY" "$TMP/gozar"
else
  if [ -n "${GOZAR_MIRROR:-}" ]; then
    BASE="${GOZAR_MIRROR%/}"
  elif [ -n "${GOZAR_VERSION:-}" ]; then
    BASE="https://github.com/$REPO/releases/download/$GOZAR_VERSION"
  else
    BASE="https://github.com/$REPO/releases/latest/download"
  fi
  command -v curl >/dev/null || die "curl is required"
  say "downloading gozar for linux/$ARCH"
  curl -fL --retry 3 --progress-bar -o "$TMP/gozar" "$BASE/gozar-linux-$ARCH" || die "download failed from $BASE"
  curl -fsSL --retry 3 -o "$TMP/checksums.txt" "$BASE/checksums.txt" || die "could not fetch checksums.txt"
  want="$(awk -v f="gozar-linux-$ARCH" '$2==f || $2=="*"f {print $1}' "$TMP/checksums.txt")"
  [ -n "$want" ] || die "no checksum for gozar-linux-$ARCH in the release"
  got="$(sha256sum "$TMP/gozar" | awk '{print $1}')"
  [ "$want" = "$got" ] || die "checksum mismatch - the download is damaged; nothing was installed"
  say "checksum verified"
fi

chmod 0755 "$TMP/gozar"
"$TMP/gozar" version >/dev/null 2>&1 || die "the downloaded file does not run on this machine"
install -m 0755 "$TMP/gozar" "$BIN"
say "installed $("$BIN" version)"

if [ "${GOZAR_NO_SETUP:-}" = 1 ]; then
  say "skipping setup (GOZAR_NO_SETUP=1). Run: sudo gozar setup"
  exit 0
fi

# Questions go to the terminal even when this script arrived through a pipe.
if [ -r /dev/tty ]; then
  exec "$BIN" setup </dev/tty
else
  say "no terminal attached. Run: sudo gozar setup"
fi
