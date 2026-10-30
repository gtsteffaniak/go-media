// Package subtitle converts text subtitles to WebVTT.
//
// Every text format is converted the way ffmpeg 7.1 does it, so results match
// "ffmpeg -i in -map 0:s:N out.vtt" byte for byte: a port of the ffmpeg
// demuxer for the file format (demux.go) or the container code in the
// matroska and isobmff packages yields packets, a port of the ffmpeg decoder
// turns each into an ASS dialog (decoders.go), and a port of the ffmpeg
// webvtt encoder and muxer writes the result (webvtt_writer.go). The
// committed ffmpeg outputs in testdata pin that behaviour; see
// testdata/gen-subtitles.sh.

package subtitle

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseSRT converts SubRip input to WebVTT, matching
// "ffmpeg -i in.srt out.vtt" byte for byte (see ParseSRTEvents and
// WriteWebVTT). It fails when the input has no cues.
func ParseSRT(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	ev := ParseSRTEvents(data)
	if len(ev) == 0 {
		return "", fmt.Errorf("no cues")
	}
	return WriteWebVTT("", ev), nil
}

// ParseVTT re-encodes WebVTT input the way "ffmpeg -i in.vtt out.vtt"
// does: cues are sorted, identifiers, cue settings, STYLE, REGION and NOTE
// blocks are dropped, and only <b>, <i> and <u> markup is kept. It fails
// when the input does not start with a WEBVTT header.
func ParseVTT(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), []byte("WEBVTT")) {
		return "", fmt.Errorf("invalid vtt")
	}
	return WriteWebVTT("", ParseVTTEvents(data)), nil
}

var assTime = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2})\.(\d{2,3})`)

// ParseASS converts ASS (v4+) or SSA (v4) input to WebVTT, matching
// "ffmpeg -i in.ass out.vtt": style bold, italic and underline flags and
// the \b, \i, \u, \r and \N override codes are kept, everything else is
// dropped. It fails when the input has no Dialogue lines.
func ParseASS(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	header, ev := ParseASSEvents(data)
	if len(ev) == 0 {
		return "", fmt.Errorf("no dialogue")
	}
	return WriteWebVTT(header, ev), nil
}

// StripASSTags removes ASS override blocks such as {\an7} and converts \N
// line breaks, leaving the plain dialogue text.
func StripASSTags(s string) string {
	return stripASSTags(s)
}

// stripASSTags removes ASS override blocks and converts \N line breaks.
func stripASSTags(s string) string {
	s = strings.ReplaceAll(s, "\\N", "\n")
	for {
		i := strings.Index(s, "{")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			break
		}
		s = s[:i] + s[i+j+1:]
	}
	return strings.TrimSpace(s)
}

// SidecarToWebVTT picks a parser based on extension.
func SidecarToWebVTT(ext string, r io.Reader) (string, error) {
	switch strings.ToLower(ext) {
	case ".srt", "srt":
		return ParseSRT(r)
	case ".vtt", "vtt":
		return ParseVTT(r)
	case ".ass", ".ssa", "ass", "ssa":
		return ParseASS(r)
	default:
		return "", fmt.Errorf("unsupported sidecar %s", ext)
	}
}

// Fuzz-friendly: detect format from first bytes
func SniffFormat(head []byte) string {
	if bytes.HasPrefix(head, []byte("WEBVTT")) {
		return "vtt"
	}
	if bytes.Contains(head, []byte("Dialogue:")) {
		return "ass"
	}
	return "srt"
}

// ParseDuration for tests
func ParseDuration(s string) (time.Duration, error) {
	m := assTime.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("bad time")
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	return time.Duration(h)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second, nil
}

// cueBlock is an internal subtitle helper.
func cueBlock(start, end, text string) string {
	return start + " --> " + end + "\n" + text
}

// toVTTTime is an internal subtitle helper.
func toVTTTime(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.SplitN(s, ".", 2)
	clock := strings.Split(parts[0], ":")
	if len(clock) == 2 {
		clock = append([]string{"00"}, clock...)
	}
	if len(clock) == 3 && len(clock[0]) == 1 {
		clock[0] = "0" + clock[0]
	}
	out := strings.Join(clock, ":")
	if len(parts) == 2 {
		out += "." + (parts[1] + "000")[:3]
	}
	return out
}
