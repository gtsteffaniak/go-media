package matroska

import (
	"bytes"
	"compress/zlib"
	"io"
	"strings"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/subtitle"
)

// ExtractSubtitleWebVTT converts the Matroska subtitle track at streamIndex to
// WebVTT, matching "ffmpeg -i in.mkv -map 0:N out.vtt" (ffmpeg 7.1) byte for
// byte for every text codec ffmpeg decodes:
//   - S_TEXT/UTF8 (SubRip markup) and S_TEXT/ASCII (plain text);
//   - S_TEXT/ASS and S_TEXT/SSA, styled with the track's CodecPrivate header;
//   - D_WEBVTT/* (WebM) and S_TEXT/WEBVTT, see webvttPayload.
//
// Bitmap codecs (S_HDMV/PGS, S_VOBSUB, S_DVBSUB, S_HDMV/TEXTST, S_ARIBSUB and
// any other codec) return an errs.Unsup error naming the codec, since they
// cannot become WebVTT without OCR.
//
// Timing follows ffmpeg's matroska demuxer: a cue ends at BlockDuration,
// else at DefaultDuration (times the lace count), else it has zero length
// (end equals start), exactly as ffmpeg writes it. Zlib and header-stripping
// ContentEncodings are undone; other encodings are unsupported.
func ExtractSubtitleWebVTT(r io.ReaderAt, size int64, streamIndex int) (string, error) {
	head, err := readHead(r, size)
	if err != nil {
		return "", err
	}
	if streamIndex < 0 || streamIndex >= len(head.Segment.Tracks.TrackEntry) {
		return "", errs.Unsupported
	}
	te := head.Segment.Tracks.TrackEntry[streamIndex]
	if te.TrackType != 17 {
		return "", errs.Unsup("subtitle", "not a subtitle track")
	}
	kind, err := subtitleKind(te.CodecID)
	if err != nil {
		return "", err
	}
	enc, err := trackEncoding(te.ContentEncodings)
	if err != nil {
		return "", err
	}
	var target uint64 = te.TrackNumber
	if target == 0 {
		target = uint64(streamIndex + 1)
	}
	ix, err := indexSegment(r, size)
	if err != nil {
		return "", err
	}
	if ix.firstCluster < 0 {
		return "", errs.Unsup("subtitle", "no subtitle blocks")
	}
	scale := int64(scaleOrDefault(head.Segment.Info.TimecodeScale))
	var events []subtitle.Event
	var total int64
	readOrder := 0
	w := clusterWalker{r: r, segEnd: ix.segEnd, off: ix.firstCluster}
	for {
		bi, err := w.next()
		if err != nil {
			// io.EOF or a corrupt tail: keep the cues read so far.
			break
		}
		if bi.track != target || bi.payloadLen <= 0 {
			continue
		}
		frames, err := readFrames(r, bi)
		if err != nil {
			break
		}
		// Block duration in ticks, as matroska_parse_block computes it: a
		// missing or zero BlockDuration falls back to DefaultDuration.
		blockDur := bi.duration
		if blockDur < 0 {
			blockDur = 0
		}
		if blockDur == 0 && te.DefaultDuration != 0 {
			blockDur = int64(te.DefaultDuration) * int64(len(frames)) / scale
		}
		laces := int64(len(frames))
		tc := bi.timecode
		for n, fb := range frames {
			laceDur := blockDur*int64(n+1)/laces - blockDur*int64(n)/laces
			start := tc
			tc += laceDur
			total += int64(len(fb))
			if total > maxCollectBytes {
				return "", errs.Unsup("subtitle", "subtitle track too large")
			}
			fb, err = enc.decode(fb)
			if err != nil {
				return "", err
			}
			dialog, ok := kind.dialog(fb, readOrder)
			if !ok {
				continue
			}
			readOrder++
			events = append(events, subtitle.Event{
				Start:    ticksToMs(start, scale),
				Duration: rescaleRound(laceDur*scale, 1_000_000),
				Dialog:   dialog,
			})
		}
	}
	if len(events) == 0 {
		return "", errs.Unsup("subtitle", "no subtitle blocks")
	}
	header := ""
	if kind == kindASS {
		header = string(te.CodecPrivate)
	}
	return subtitle.WriteWebVTT(header, events), nil
}

// subKind is the ffmpeg decoder a Matroska subtitle codec maps to.
type subKind int

// Matroska subtitle decoders, named after the ffmpeg decoder each mirrors.
const (
	kindSubRip       subKind = iota // S_TEXT/UTF8: ffmpeg "subrip" decoder
	kindText                        // S_TEXT/ASCII: ffmpeg "text" decoder
	kindASS                         // S_TEXT/ASS, S_TEXT/SSA: ffmpeg "ass" decoder
	kindWebVTTStrict                // D_WEBVTT/*: ffmpeg "webvtt" decoder
	kindWebVTTText                  // S_TEXT/WEBVTT: as kindWebVTTStrict, plus a text-only fallback
)

// subtitleKind maps a CodecID to its decoder, or returns a descriptive
// unsupported error for bitmap and unknown codecs.
func subtitleKind(codecID string) (subKind, error) {
	switch {
	case codecID == "S_TEXT/UTF8":
		return kindSubRip, nil
	case codecID == "S_TEXT/ASCII":
		return kindText, nil
	case codecID == "S_TEXT/ASS", codecID == "S_TEXT/SSA", codecID == "S_ASS", codecID == "S_SSA":
		return kindASS, nil
	case strings.HasPrefix(codecID, "D_WEBVTT"):
		// ffmpeg matches D_WEBVTT/SUBTITLES, CAPTIONS, DESCRIPTIONS and
		// METADATA to its webvtt codec.
		return kindWebVTTStrict, nil
	case codecID == "S_TEXT/WEBVTT":
		return kindWebVTTText, nil
	case codecID == "S_HDMV/PGS":
		return 0, errs.Unsup("subtitle", "S_HDMV/PGS is a bitmap subtitle format (Blu-ray PGS) and cannot be converted to WebVTT")
	case codecID == "S_VOBSUB":
		return 0, errs.Unsup("subtitle", "S_VOBSUB is a bitmap subtitle format (DVD VobSub) and cannot be converted to WebVTT")
	case codecID == "S_DVBSUB":
		return 0, errs.Unsup("subtitle", "S_DVBSUB is a bitmap subtitle format (DVB) and cannot be converted to WebVTT")
	case codecID == "S_HDMV/TEXTST":
		return 0, errs.Unsup("subtitle", "S_HDMV/TEXTST (Blu-ray text subtitles) is not supported; ffmpeg has no decoder for it")
	case codecID == "S_ARIBSUB":
		return 0, errs.Unsup("subtitle", "S_ARIBSUB (ARIB STD-B24 captions) is not supported")
	default:
		return 0, errs.Unsup("subtitle", "unsupported subtitle codec "+codecID)
	}
}

// dialog turns one frame into an ASS dialog line, or reports false when
// ffmpeg's demuxer or decoder produces no subtitle for it.
func (k subKind) dialog(fb []byte, readOrder int) (string, bool) {
	switch k {
	case kindASS:
		// The frame is already the "ReadOrder,Layer,Style,..." dialog line
		// ffmpeg's ass decoder takes; it is C-string text.
		s := cString(fb)
		if s == "" {
			return "", false
		}
		return s, true
	case kindWebVTTStrict, kindWebVTTText:
		text, ok := webvttPayload(fb, k == kindWebVTTText)
		if !ok {
			return "", false
		}
		return subtitle.DefaultDialog(readOrder, subtitle.WebVTTToASS(text)), true
	case kindText:
		if len(fb) == 0 {
			return "", false
		}
		return subtitle.DefaultDialog(readOrder, subtitle.PlainTextToASS(cString(fb))), true
	default:
		if len(fb) == 0 {
			return "", false
		}
		return subtitle.DefaultDialog(readOrder, subtitle.SubRipToASS(cString(fb))), true
	}
}

// webvttPayload extracts the cue text of a Matroska WebVTT frame the way
// ffmpeg's matroska_parse_webvtt does: the frame must be
// "identifier\nsettings\ntext" (CRLF allowed); identifier and settings are
// discarded and trailing CR/LF is trimmed from the text. ffmpeg drops frames
// that do not have that shape or whose text is empty.
//
// Intentional difference: with textOnly (S_TEXT/WEBVTT, as written by
// mkvmerge, which stores identifier and settings in BlockAdditions) a frame
// without the two header lines is taken as text, where ffmpeg would drop it
// and show nothing.
func webvttPayload(fb []byte, textOnly bool) (string, bool) {
	s := string(fb)
	rest, ok := cutVTTLine(s)
	if ok {
		rest, ok = cutVTTLine(rest)
	}
	if !ok {
		if !textOnly {
			return "", false
		}
		rest = s
	}
	rest = strings.TrimRight(rest, "\r\n")
	if rest == "" {
		return "", false
	}
	return rest, true
}

// cutVTTLine drops one line ending in "\n" or "\r\n"; it fails when there is
// no line break, or a lone "\r" ends the line.
func cutVTTLine(s string) (string, bool) {
	i := strings.IndexAny(s, "\r\n")
	if i < 0 {
		return "", false
	}
	if s[i] == '\r' {
		i++
	}
	if i >= len(s) || s[i] != '\n' {
		return "", false
	}
	return s[i+1:], true
}

// cString cuts b at its first NUL, as ffmpeg's decoders read packet data as
// a C string.
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// frameEncoding undoes a track's ContentEncoding on each frame.
type frameEncoding struct {
	algo     int // -1 none, 0 zlib, 3 header stripping
	settings []byte
}

// trackEncoding reads the first ContentEncoding, as ffmpeg does (it ignores
// any further ones). Encryption and bzlib/lzo compression are unsupported.
func trackEncoding(ce *contentEncodings) (frameEncoding, error) {
	if ce == nil || len(ce.ContentEncoding) == 0 {
		return frameEncoding{algo: -1}, nil
	}
	e := ce.ContentEncoding[0]
	scope := e.ContentEncodingScope
	if scope == 0 {
		scope = 1
	}
	if scope&1 == 0 {
		// Only CodecPrivate or the next encoding is affected, not frames.
		return frameEncoding{algo: -1}, nil
	}
	if e.ContentEncodingType != 0 {
		return frameEncoding{}, errs.Unsup("subtitle", "encrypted Matroska subtitle tracks are not supported")
	}
	fe := frameEncoding{}
	if e.ContentCompression != nil {
		fe.algo = int(e.ContentCompression.ContentCompAlgo)
		fe.settings = e.ContentCompression.ContentCompSettings
	}
	switch fe.algo {
	case 0, 3:
		return fe, nil
	default:
		return frameEncoding{}, errs.Unsup("subtitle", "Matroska subtitle compression other than zlib or header stripping is not supported")
	}
}

// decode returns the frame with its encoding removed.
func (e frameEncoding) decode(fb []byte) ([]byte, error) {
	switch e.algo {
	case 0:
		zr, err := zlib.NewReader(bytes.NewReader(fb))
		if err != nil {
			return nil, errs.Corrupt
		}
		out, err := io.ReadAll(io.LimitReader(zr, maxCollectBytes+1))
		if err != nil {
			return nil, errs.Corrupt
		}
		if len(out) > maxCollectBytes {
			return nil, errs.Unsup("subtitle", "subtitle track too large")
		}
		return out, nil
	case 3:
		return append(append([]byte{}, e.settings...), fb...), nil
	}
	return fb, nil
}

// ticksToMs converts a timestamp in Matroska ticks (scale ns each) to
// milliseconds as the ffmpeg CLI does: first to microseconds, then to
// milliseconds, rounding half away from zero each time.
func ticksToMs(ticks, scale int64) int64 {
	return rescaleRound(rescaleRound(ticks*scale, 1000), 1000)
}

// rescaleRound returns a/d rounded half away from zero (av_rescale's
// AV_ROUND_NEAR_INF).
func rescaleRound(a, d int64) int64 {
	if a < 0 {
		return -((-a + d/2) / d)
	}
	return (a + d/2) / d
}

// scaleOrDefault applies the 1 ms Matroska default to an unset TimecodeScale.
func scaleOrDefault(s uint64) uint64 {
	if s == 0 {
		return 1_000_000
	}
	return s
}
