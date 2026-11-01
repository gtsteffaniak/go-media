# Shared helpers for locating working ffmpeg/ffprobe binaries.
# shellcheck shell=bash
# Usage: source "$(dirname "$0")/ffmpeg-path.sh"

ffmpeg_binary_usable() {
	[[ -n "${1:-}" && -x "$1" ]] && "$1" -version >/dev/null 2>&1
}

ffmpeg_version_first_line() {
	"$1" -version 2>&1 | sed -n '1p' || true
}

# True when the ffmpeg -version first line reports ver (not a longer prefix, e.g. 7.1 vs 7.1.1).
ffmpeg_version_line_matches() {
	local line="$1" ver="$2"
	local needle="version ${ver}"
	[[ "$line" == *"$needle"* ]] || return 1
	local rest="${line#*"$needle"}"
	[[ "$rest" =~ ^[0-9.] ]] && return 1
	return 0
}

ffmpeg_pair_matches_version() {
	local ff="$1" fp="$2" ver="$3"
	local ff_line fp_line
	ff_line=$(ffmpeg_version_first_line "$ff")
	fp_line=$(ffmpeg_version_first_line "$fp")
	ffmpeg_version_line_matches "$ff_line" "$ver" && ffmpeg_version_line_matches "$fp_line" "$ver"
}

# _ffmpeg_bindir_if_usable returns 0 when dir/ffmpeg and dir/ffprobe work, else returns 1.
_ffmpeg_bindir_if_usable() {
	local dir="$1" ff fp
	[[ -n "$dir" ]] || return 1
	ff="$dir/ffmpeg"
	fp="$dir/ffprobe"
	ffmpeg_binary_usable "$ff" && ffmpeg_binary_usable "$fp"
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
	if _ffmpeg_bindir_if_usable "$dir"; then
		printf '%s\n' "$dir"
		return 0
	fi
	return 1
}

# _ffmpeg_scan_path walks PATH (colon-separated only; preserves spaces in entries).
_ffmpeg_scan_path() {
	local path_entry old_ifs="$IFS"
	IFS=':'
	for path_entry in $PATH; do
		IFS="$old_ifs"
		[[ -n "$path_entry" ]] || continue
		if _ffmpeg_bindir_if_usable "$path_entry"; then
			printf '%s\n' "$path_entry"
			return 0
		fi
		IFS=':'
	done
	IFS="$old_ifs"
	return 1
}

# _ffmpeg_scan_path_version walks PATH and returns the first dir whose ffmpeg/ffprobe match ver.
_ffmpeg_scan_path_version() {
	local ver="$1" path_entry ff fp old_ifs="$IFS"
	IFS=':'
	for path_entry in $PATH; do
		IFS="$old_ifs"
		[[ -n "$path_entry" ]] || continue
		if _ffmpeg_bindir_if_usable "$path_entry"; then
			ff="$path_entry/ffmpeg"
			fp="$path_entry/ffprobe"
			if ffmpeg_pair_matches_version "$ff" "$fp" "$ver"; then
				printf '%s\n' "$path_entry"
				return 0
			fi
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

	# Gate: prefer pinned FFMPEG_DIR (CI static build). When that dir is missing (common on
	# macOS Homebrew), accept PATH only if ffmpeg/ffprobe match FFMPEG_VERSION.
	if [[ -n "${FFMPEG_DIR:-}" && "${COMPARE_GATE:-}" == "1" ]]; then
		if _ffmpeg_bindir_if_usable "${FFMPEG_DIR}"; then
			printf '%s\n' "${FFMPEG_DIR}"
			return 0
		fi
		local gate_ver="${FFMPEG_VERSION:-9.0.2}"
		if dir=$(_ffmpeg_scan_path_version "$gate_ver"); then
			echo "COMPARE_GATE: using PATH ffmpeg at $dir (FFMPEG_DIR=${FFMPEG_DIR} not installed)" >&2
			printf '%s\n' "$dir"
			return 0
		fi
		echo "COMPARE_GATE: FFMPEG_DIR=${FFMPEG_DIR} has no usable ffmpeg/ffprobe (need ffmpeg $gate_ver on PATH or run make ffmpeg on Linux)" >&2
		return 1
	fi

	if dir=$(_ffmpeg_scan_path); then
		printf '%s\n' "$dir"
		return 0
	fi

	if [[ -n "${FFMPEG_DIR:-}" ]]; then
		if _ffmpeg_bindir_if_usable "${FFMPEG_DIR}"; then
			printf '%s\n' "${FFMPEG_DIR}"
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
