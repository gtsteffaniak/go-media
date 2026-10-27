#!/usr/bin/env bash
# Install a static ffmpeg/ffprobe build from the gtsteffaniak/ffmpeg releases.
# Avoids distro packages so CI does not depend on apt mirrors being healthy.
#
# On macOS (and when FFMPEG_FORCE_INSTALL is not set), skips download if working
# ffmpeg/ffprobe are already on PATH. Linux CI should set FFMPEG_FORCE_INSTALL=1
# when a pinned version is required regardless of distro ffmpeg.
#
# Usage: scripts/install-ffmpeg.sh [version] [dest]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=ffmpeg-path.sh
source "$SCRIPT_DIR/ffmpeg-path.sh"

VERSION="${1:-${FFMPEG_VERSION:-9.0.2}}"
DEST="${2:-${FFMPEG_INSTALL_DIR:-$HOME/ffmpeg-$VERSION}}"
export FFMPEG_DIR="${FFMPEG_DIR:-$DEST}"

use_path_ffmpeg() {
	local ff fp ff_dir fp_dir
	ff=$(command -v ffmpeg 2>/dev/null || true)
	fp=$(command -v ffprobe 2>/dev/null || true)
	if ! ffmpeg_binary_usable "$ff" || ! ffmpeg_binary_usable "$fp"; then
		return 1
	fi
	ff_dir=$(cd "$(dirname "$ff")" && pwd)
	fp_dir=$(cd "$(dirname "$fp")" && pwd)
	if [[ "$ff_dir" != "$fp_dir" ]]; then
		return 1
	fi
	if ! ffmpeg_pair_matches_version "$ff" "$fp" "$VERSION"; then
		return 1
	fi
	echo "Using ffmpeg $VERSION from PATH: $ff"
	ffmpeg_version_first_line "$ff"
	ffmpeg_version_first_line "$fp"
	return 0
}

if [[ "${FFMPEG_FORCE_INSTALL:-}" != "1" ]]; then
	if [[ "$(uname -s)" == "Darwin" ]]; then
		if prepend_ffmpeg_to_path && use_path_ffmpeg; then
			exit 0
		fi
	elif use_path_ffmpeg; then
		exit 0
	fi
fi

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*)
	echo "unsupported architecture: $(uname -m)" >&2
	exit 1
	;;
esac

if [[ "$(uname -s)" != "Linux" ]]; then
	echo "Static ffmpeg releases are Linux-only; install ffmpeg locally (e.g. Homebrew) or set FFMPEG_DIR / FFMPEG_PATH." >&2
	exit 1
fi

TARBALL="ffmpeg-${VERSION}-linux-${ARCH}.tar.gz"
URL="https://github.com/gtsteffaniak/ffmpeg/releases/download/v${VERSION}/${TARBALL}"

mkdir -p "$DEST"
if ffmpeg_binary_usable "$DEST/ffmpeg" && ffmpeg_binary_usable "$DEST/ffprobe"; then
	if ffmpeg_pair_matches_version "$DEST/ffmpeg" "$DEST/ffprobe" "$VERSION"; then
		echo "Static ffmpeg $VERSION ($ARCH) already present at $DEST"
		ffmpeg_version_first_line "$DEST/ffmpeg"
		ffmpeg_version_first_line "$DEST/ffprobe"
		exit 0
	fi
	echo "Replacing ffmpeg at $DEST (version mismatch with requested $VERSION)" >&2
	rm -f "$DEST/ffmpeg" "$DEST/ffprobe"
fi

echo "Downloading $URL"
curl -fsSL --retry 3 --retry-delay 2 "$URL" -o "$DEST/$TARBALL"
tar -xzf "$DEST/$TARBALL" -C "$DEST"
rm -f "$DEST/$TARBALL"
chmod +x "$DEST/ffmpeg" "$DEST/ffprobe"

ffmpeg_version_first_line "$DEST/ffmpeg"
ffmpeg_version_first_line "$DEST/ffprobe"

if [[ -n "${GITHUB_PATH:-}" ]]; then
	echo "$DEST" >>"$GITHUB_PATH"
fi
echo "Installed static ffmpeg $VERSION ($ARCH) to $DEST"
