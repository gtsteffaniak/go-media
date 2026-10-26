# Shared helpers for locating working ffmpeg/ffprobe binaries.
# shellcheck shell=bash
# Usage: source "$(dirname "$0")/ffmpeg-path.sh"

ffmpeg_binary_usable() {
	[[ -n "${1:-}" && -x "$1" ]] && "$1" -version >/dev/null 2>&1
}

# _ffmpeg_bindir_if_usable prints dir when dir/ffmpeg and dir/ffprobe work, else returns 1.
_ffmpeg_bindir_if_usable() {
	local dir="$1" ff fp
	[[ -n "$dir" ]] || return 1
	ff="$dir/ffmpeg"
	fp="$dir/ffprobe"
	if ffmpeg_binary_usable "$ff" && ffmpeg_binary_usable "$fp"; then
		printf '%s\n' "$dir"
		return 0
	fi
	return 1
}

# _ffmpeg_resolve_path_override normalizes FFMPEG_PATH to a directory with both binaries.
_ffmpeg_resolve_path_override() {
	local dir="${FFMPEG_PATH:-}"
	[[ -n "$dir" ]] || return 1
	if [[ -x "$dir" && ! -d "$dir" ]]; then
		dir="$(cd "$(dirname "$dir")" && pwd)"
	else
		dir="${dir%/}"
	fi
	_ffmpeg_bindir_if_usable "$dir"
}

# _ffmpeg_scan_path walks PATH (colon-separated only; preserves spaces in entries).
_ffmpeg_scan_path() {
	local path_entry old_ifs="$IFS"
	IFS=':'
	for path_entry in $PATH; do
		IFS="$old_ifs"
		[[ -n "$path_entry" ]] || continue
		if _ffmpeg_bindir_if_usable "$path_entry"; then
			return 0
		fi
		IFS=':'
	done
	IFS="$old_ifs"
	return 1
}

# find_ffmpeg_bindir prints a directory with working ffmpeg and ffprobe, or returns 1.
find_ffmpeg_bindir() {
	local dir

	if dir=$(_ffmpeg_resolve_path_override); then
		printf '%s\n' "$dir"
		return 0
	fi

	# Gate / explicit pin: prefer FFMPEG_DIR before PATH.
	if [[ -n "${FFMPEG_DIR:-}" && "${COMPARE_GATE:-}" == "1" ]]; then
		if dir=$(_ffmpeg_bindir_if_usable "${FFMPEG_DIR}"); then
			printf '%s\n' "$dir"
			return 0
		fi
	fi

	if dir=$(_ffmpeg_scan_path); then
		printf '%s\n' "$dir"
		return 0
	fi

	if [[ -n "${FFMPEG_DIR:-}" ]]; then
		if dir=$(_ffmpeg_bindir_if_usable "${FFMPEG_DIR}"); then
			printf '%s\n' "$dir"
			return 0
		fi
	fi

	return 1
}

# prepend_ffmpeg_to_path puts a usable ffmpeg/ffprobe directory first on PATH.
prepend_ffmpeg_to_path() {
	local bindir path_entry new_path old_ifs
	if ! bindir=$(find_ffmpeg_bindir); then
		return 1
	fi
	if [[ "$PATH" == "$bindir" || "$PATH" == "$bindir:"* ]]; then
		return 0
	fi
	new_path="$bindir"
	old_ifs="$IFS"
	IFS=':'
	for path_entry in $PATH; do
		IFS="$old_ifs"
		[[ -n "$path_entry" && "$path_entry" != "$bindir" ]] || continue
		new_path="$new_path:$path_entry"
		IFS=':'
	done
	IFS="$old_ifs"
	export PATH="$new_path"
	return 0
}
