#!/usr/bin/env bash
# Install a static ffmpeg/ffprobe build from the gtsteffaniak/ffmpeg releases.
# Avoids distro packages so CI does not depend on apt mirrors being healthy.
#
# Usage: scripts/install-ffmpeg.sh [version] [dest]
set -euo pipefail

VERSION="${1:-${FFMPEG_VERSION:-9.0.2}}"
DEST="${2:-${FFMPEG_INSTALL_DIR:-$HOME/ffmpeg-$VERSION}}"

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*)
	echo "unsupported architecture: $(uname -m)" >&2
	exit 1
	;;
esac

TARBALL="ffmpeg-${VERSION}-linux-${ARCH}.tar.gz"
URL="https://github.com/gtsteffaniak/ffmpeg/releases/download/v${VERSION}/${TARBALL}"

mkdir -p "$DEST"
if [[ -x "$DEST/ffmpeg" && -x "$DEST/ffprobe" ]]; then
	echo "Static ffmpeg $VERSION ($ARCH) already present at $DEST"
	"$DEST/ffmpeg" -version 2>&1 | sed -n '1p'
	"$DEST/ffprobe" -version 2>&1 | sed -n '1p'
	exit 0
fi

echo "Downloading $URL"
curl -fsSL --retry 3 --retry-delay 2 "$URL" -o "$DEST/$TARBALL"
tar -xzf "$DEST/$TARBALL" -C "$DEST"
rm -f "$DEST/$TARBALL"
chmod +x "$DEST/ffmpeg" "$DEST/ffprobe"

"$DEST/ffmpeg" -version 2>&1 | sed -n '1p'
"$DEST/ffprobe" -version 2>&1 | sed -n '1p'

if [[ -n "${GITHUB_PATH:-}" ]]; then
	echo "$DEST" >>"$GITHUB_PATH"
fi
echo "Installed static ffmpeg $VERSION ($ARCH) to $DEST"
