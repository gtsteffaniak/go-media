# Shared helpers for locating working ffmpeg/ffprobe binaries.
# shellcheck shell=bash
# Usage: source "$(dirname "$0")/ffmpeg-path.sh"

ffmpeg_binary_usable() {
	[[ -n "${1:-}" && -x "$1" ]] && "$1" -version >/dev/null 2>&1
}

# find_ffmpeg_bindir prints a directory with working ffmpeg and ffprobe, or returns 1.
find_ffmpeg_bindir() {
	local dir ff fp pinned="${FFMPEG_DIR:-}"

	if [[ -n "$pinned" ]]; then
		if ffmpeg_binary_usable "$pinned/ffmpeg" && ffmpeg_binary_usable "$pinned/ffprobe"; then
			echo "$pinned"
			return 0
		fi
	fi

	local path_entry
	for path_entry in ${PATH//:/ }; do
		[[ -n "$path_entry" ]] || continue
		ff="$path_entry/ffmpeg"
		fp="$path_entry/ffprobe"
		if ffmpeg_binary_usable "$ff" && ffmpeg_binary_usable "$fp"; then
			echo "$path_entry"
			return 0
		fi
	done
	return 1
}

# prepend_ffmpeg_to_path puts a usable ffmpeg/ffprobe directory first on PATH.
prepend_ffmpeg_to_path() {
	local bindir
	if ! bindir=$(find_ffmpeg_bindir); then
		return 1
	fi
	case ":$PATH:" in
	*":$bindir:"*) ;;
	*) export PATH="$bindir:$PATH" ;;
	esac
	return 0
}
