#!/bin/sh
# gen-subtitles.sh regenerates the subtitle fixtures and their ffmpeg goldens.
#
# Every <fixture>.ffmpeg.vtt next to a fixture is ffmpeg's own output for
#
#     ffmpeg -nostdin -v error -i <fixture> -map 0:s:0 -f webvtt <golden>
#
# and the golden tests in internal/subtitle, internal/matroska and
# internal/isobmff require go-media to reproduce it byte for byte.
#
# Goldens captured with: ffmpeg 7.1.5 (Debian build, libavformat 61.7.100).
# A different ffmpeg version may print different bytes; re-run this script
# and review the diff before committing new goldens.
#
# Usage: testdata/gen-subtitles.sh            # goldens only
#        testdata/gen-subtitles.sh --fixtures # rebuild fixtures, then goldens
set -eu
cd "$(dirname "$0")/.."
FF="ffmpeg -nostdin -v error -y"
S=internal/subtitle/testdata
M=internal/matroska/testdata
I=internal/isobmff/testdata

ffmpeg -version | head -n 1

if [ "${1:-}" = "--fixtures" ]; then
	# Hand-built Matroska (bitmap codecs, lacing, zlib, header stripping,
	# missing BlockDuration, TimecodeScale) and MP4 (tx3g default style,
	# empty edit, wvtt) fixtures.
	(cd "$M" && python3 gen_subtitle_mkv.py)
	# ffmpeg-muxed Matroska: text codecs stream-copied from the sidecars,
	# WebVTT re-encoded because .vtt input decodes to ASS.
	$FF -i "$S/srt_markup.srt" -map 0 -c copy "$M/sub_ff_srt.mkv"
	$FF -i "$S/ass_v4plus.ass" -map 0 -c copy "$M/sub_ff_ass.mkv"
	$FF -i "$S/ssa_v4.ssa" -map 0 -c copy "$M/sub_ff_ssa.mkv"
	$FF -i "$S/vtt_features.vtt" -map 0 -c:s webvtt "$M/sub_ff_webvtt.mkv"
	# ffmpeg-muxed MP4 (mov_text = tx3g).
	$FF -i "$S/srt_markup.srt" -c:s mov_text -fflags +bitexact "$I/sub_ff_tx3g.mp4"
	# srt_structure ends with a negative-duration cue the mp4 muxer rejects,
	# so its last cue is dropped and the rest piped in; it also starts with
	# garbage that defeats probing, hence -f srt.
	sed -n '/^9\r\{0,1\}$/q;p' "$S/srt_structure.srt" |
		$FF -f srt -i - -c:s mov_text -fflags +bitexact "$I/sub_ff_tx3g_overlap.mp4"
	$FF -itsoffset 5 -i "$S/srt_bom_crlf.srt" -c:s mov_text -fflags +bitexact "$I/sub_ff_tx3g_offset.mp4"
	(cd "$I" && python3 gen_subtitle_mp4.py)
fi

golden() { # golden <fixture> [input options]
	f=$1
	shift
	$FF "$@" -i "$f" -map 0:s:0 -f webvtt "$f.ffmpeg.vtt"
}

for f in "$S"/*.srt "$S"/*.ass "$S"/*.ssa "$S"/*.vtt; do
	case $f in
	*/srt_structure.srt) golden "$f" -f srt ;;
	*) golden "$f" ;;
	esac
done
# Fixtures ffmpeg cannot convert (bitmap codecs, S_TEXT/WEBVTT, wvtt) have
# hand-written *.expected.vtt files instead; see the tests.
for f in "$M"/sub_*.mkv; do
	case $f in
	*_pgs.mkv | *_vobsub.mkv | *_dvbsub.mkv | *_textst.mkv | *_aribsub.mkv | *_stextwebvtt.mkv) ;;
	*) golden "$f" ;;
	esac
done
for f in "$I"/sub_*.mp4; do
	case $f in
	*_wvtt.mp4) ;;
	*) golden "$f" ;;
	esac
done
