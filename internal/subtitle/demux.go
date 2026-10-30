package subtitle

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// This file ports ffmpeg 7.1's subtitle file demuxers: libavformat/srtdec.c,
// webvttdec.c and assdec.c, plus the shared text reader and packet queue in
// libavformat/subtitles.c. Each turns a whole file into ordered Events.

// queued is one demuxed packet before decoding, like an AVPacket in
// ffmpeg's FFDemuxSubtitlesQueue. Times are in ms.
type queued struct {
	pts, dur, pos int64
	data          string
}

// finalizeQueue ports ff_subtitles_queue_finalize: sort by start then file
// position, give cues with a negative duration (end before start) the gap
// to the next cue, and drop exact duplicates unless keepDups is set (ASS).
// The last cue keeps a negative duration, which ffmpeg writes as an end
// time before the start time.
func finalizeQueue(q []queued, keepDups bool) []queued {
	sort.SliceStable(q, func(i, j int) bool {
		if q[i].pts != q[j].pts {
			return q[i].pts < q[j].pts
		}
		return q[i].pos < q[j].pos
	})
	for i := range q {
		if q[i].dur < 0 && i < len(q)-1 {
			q[i].dur = q[i+1].pts - q[i].pts
		}
	}
	if keepDups || len(q) == 0 {
		return q
	}
	out := q[:1]
	for _, p := range q[1:] {
		last := out[len(out)-1]
		if p.pts == last.pts && p.dur == last.dur && cstr(p.data) == cstr(last.data) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// cstr cuts s at its first NUL, as C string functions see it.
func cstr(s string) string {
	if i := strings.IndexByte(s, 0); i >= 0 {
		return s[:i]
	}
	return s
}

// textReader ports FFTextReader: UTF-16 files with a BOM are converted to
// UTF-8, a UTF-8 BOM is skipped, and anything else is read as raw bytes.
// A NUL character ends the text, as ff_text_r8 returns 0 for it.
func readText(b []byte) string {
	if len(b) >= 2 && (b[0] == 0xff && b[1] == 0xfe || b[0] == 0xfe && b[1] == 0xff) {
		le := b[0] == 0xff
		b = b[2:]
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			v := uint16(b[i])<<8 | uint16(b[i+1])
			if le {
				v = uint16(b[i+1])<<8 | uint16(b[i])
			}
			if v == 0 {
				break
			}
			u = append(u, v)
		}
		return string(utf16.Decode(u))
	}
	s := strings.TrimPrefix(string(b), "\xef\xbb\xbf")
	return cstr(s)
}

// lineReader ports ff_subtitles_read_line: a line ends at CR or LF, then
// any run of CRs and one LF are consumed.
type lineReader struct {
	s   string
	pos int
}

// next returns the next line and its byte offset; ok is false at the end.
func (r *lineReader) next() (line string, pos int, ok bool) {
	if r.pos >= len(r.s) {
		return "", r.pos, false
	}
	pos = r.pos
	n := strcspn(r.s[r.pos:], "\r\n")
	line = r.s[r.pos : r.pos+n]
	r.pos += n
	for r.pos < len(r.s) && r.s[r.pos] == '\r' {
		r.pos++
	}
	if r.pos < len(r.s) && r.s[r.pos] == '\n' {
		r.pos++
	}
	return line, pos, true
}

// srtTiming parses "%d:%d:%d%*1[,.]%d --> %d:%d:%d%*1[,.]%d" (spaces in the
// pattern match any run of whitespace, including none) and returns the
// start and end in ms. The millisecond field is taken as a plain integer,
// so "00:00:01,5" is 1.005 s, as in ffmpeg.
func srtTiming(line string) (start, end int64, ok bool) {
	var v [8]int64
	s := line
	for k := 0; k < 8; k++ {
		x, n := scanIntN(s)
		if n == 0 {
			return 0, 0, false
		}
		v[k] = int64(int32(x))
		s = s[n:]
		var sep string
		switch k {
		case 0, 1, 4, 5:
			sep = ":"
		case 2, 6:
			if s == "" || (s[0] != ',' && s[0] != '.') {
				return 0, 0, false
			}
			s = s[1:]
			continue
		case 3:
			s = strings.TrimLeft(s, " \t\n\r\v\f")
			sep = "-->"
		case 7:
			continue
		}
		if !strings.HasPrefix(s, sep) {
			return 0, 0, false
		}
		s = s[len(sep):]
	}
	start = (v[0]*3600+v[1]*60+v[2])*1000 + v[3]
	end = (v[4]*3600+v[5]*60+v[6])*1000 + v[7]
	return start, end, true
}

// leadingInt reports strtol's view of s: whether it starts with a number
// (after optional whitespace and sign), its sign, and the rest.
func leadingInt(s string) (neg, ok bool, rest string) {
	v, n := scanIntN(s)
	if n == 0 {
		return false, false, s
	}
	return v < 0, true, s[n:]
}

// ParseSRTEvents demuxes and decodes a SubRip file like ffmpeg's srt
// demuxer and subrip decoder. Blank lines do not separate cues: a cue's
// text runs until the next timing line, and a numeric line right before a
// timing line is taken as the cue number. Lines before the first timing
// line are ignored. Cues are sorted by start time and exact duplicates are
// dropped.
func ParseSRTEvents(b []byte) []Event {
	r := &lineReader{s: readText(b)}
	var q []queued
	var buf strings.Builder
	cache := ""
	have := false
	var cur queued
	add := func(appendCache bool) {
		if appendCache && cache != "" {
			buf.WriteString(cache + "\n")
		}
		cache = ""
		text := strings.TrimRight(buf.String(), "\n")
		if text != "" {
			cur.data = text
			q = append(q, cur)
		}
		buf.Reset()
	}
	for {
		line, pos, ok := r.next()
		if !ok {
			break
		}
		if line == "" {
			continue
		}
		start, end, isTiming := srtTiming(line)
		if !isTiming {
			if !have {
				continue
			}
			if cache != "" {
				buf.WriteString(cache + "\n")
				cache = ""
			}
			if neg, num, _ := leadingInt(line); neg || !num {
				buf.WriteString(line + "\n")
			} else {
				cache = line
			}
			continue
		}
		if have {
			neg, num, rest := leadingInt(cache)
			standalone := cache == "" || (!neg && num && rest == "")
			add(buf.Len() == 0 && !standalone)
		}
		have = true
		cur = queued{pts: start, dur: int64(int32(end - start)), pos: int64(pos)}
	}
	if have {
		add(true)
	}
	q = finalizeQueue(q, false)
	ev := make([]Event, len(q))
	for i, p := range q {
		ev[i] = Event{Start: p.pts, Duration: p.dur, Dialog: DefaultDialog(i, SubRipToASS(p.data))}
	}
	return ev
}

// vttChunk ports read_text_chunk of libavformat/webvttdec.c: a block of
// text ends at a blank line, leading line breaks are skipped, and the
// trailing line break is dropped.
func vttChunk(s string, pos int) (chunk string, next int) {
	for pos < len(s) && (s[pos] == '\r' || s[pos] == '\n') {
		pos++
	}
	var b strings.Builder
	var eol []byte
	nbEOL, lastWasCR := 0, false
	for ; pos < len(s); pos++ {
		c := s[pos]
		if c == 0 {
			pos++
			break
		}
		if c == '\r' || c == '\n' {
			if c == '\n' || lastWasCR {
				nbEOL++
			}
			lastWasCR = c == '\r'
			if nbEOL == 2 {
				pos++
				break
			}
			eol = append(eol, c)
			if len(eol) == 4 {
				pos++
				break
			}
			continue
		}
		b.Write(eol)
		eol = eol[:0]
		nbEOL = 0
		b.WriteByte(c)
	}
	return b.String(), pos
}

// vttTime ports read_ts: "%u:%u:%u.%u", else "%u:%u.%u", in ms. Like
// ffmpeg, the fraction is a plain integer, so ".5" is 5 ms.
func vttTime(s string) (int64, bool) {
	if v, ok := scanNums(s, "::."); ok {
		return (v[0]*3600+v[1]*60+v[2])*1000 + v[3], true
	}
	if v, ok := scanNums(s, ":."); ok {
		return (v[0]*60+v[1])*1000 + v[2], true
	}
	return 0, false
}

// scanNums reads len(seps)+1 integers separated by the given bytes, like
// sscanf "%u:%u.%u".
func scanNums(s, seps string) ([]int64, bool) {
	v := make([]int64, 0, len(seps)+1)
	for k := 0; ; k++ {
		x, n := scanIntN(s)
		if n == 0 {
			return nil, false
		}
		v = append(v, int64(uint32(x)))
		s = s[n:]
		if k == len(seps) {
			return v, true
		}
		if s == "" || s[0] != seps[k] {
			return nil, false
		}
		s = s[1:]
	}
}

// ParseVTTEvents demuxes and decodes a WebVTT file like ffmpeg's webvtt
// demuxer and decoder. STYLE, REGION and NOTE blocks are skipped. Cue
// identifiers and cue settings are read and then dropped, because ffmpeg
// does not carry them when converting (only a WebVTT-to-WebVTT stream copy
// would keep them). A malformed timing line ends the file, as in ffmpeg.
func ParseVTTEvents(b []byte) []Event {
	s := string(b)
	var q []queued
	for pos := 0; ; {
		start := pos
		chunk, next := vttChunk(s, pos)
		pos = next
		if chunk == "" {
			break
		}
		if strings.HasPrefix(chunk, "WEBVTT") || strings.HasPrefix(chunk, "\xef\xbb\xbfWEBVTT") ||
			strings.HasPrefix(chunk, "STYLE") || strings.HasPrefix(chunk, "REGION") || strings.HasPrefix(chunk, "NOTE") {
			continue
		}
		p := chunk
		if first := p[:strcspn(p, "\r\n")]; !strings.Contains(first, "-->") {
			// The first line is a cue identifier.
			p = p[len(first):]
			p = strings.TrimPrefix(p, "\r")
			p = strings.TrimPrefix(p, "\n")
		}
		ts0, ok := vttTime(p)
		if !ok {
			break
		}
		arrow := strings.Index(p, "-->")
		if arrow < 0 {
			break
		}
		p = p[arrow+3:]
		p = strings.TrimLeft(p, " \t")
		ts1, ok := vttTime(p)
		if !ok {
			break
		}
		p = p[strcspn(p, "\n\r\t "):]
		p = p[strcspn(p, "\r\n"):] // cue settings, dropped like ffmpeg does when converting
		p = strings.TrimPrefix(p, "\r")
		p = strings.TrimPrefix(p, "\n")
		q = append(q, queued{pts: ts0, dur: ts1 - ts0, pos: int64(start), data: p})
	}
	q = finalizeQueue(q, false)
	var ev []Event
	for i, p := range q {
		if p.data == "" {
			// A zero-size packet: ffmpeg's decoder emits no subtitle.
			continue
		}
		ev = append(ev, Event{Start: p.pts, Duration: p.dur, Dialog: DefaultDialog(i, WebVTTToASS(p.data))})
	}
	return ev
}

// ParseASSEvents demuxes an ASS or SSA file like ffmpeg's ass demuxer. It
// returns the header (every line that is not a Dialogue event, which holds
// the styles) and the events, sorted by start time; duplicates are kept.
// Times are centiseconds in ASS, so they convert to ms exactly.
func ParseASSEvents(b []byte) (header string, events []Event) {
	s := readText(b)
	var hdr strings.Builder
	var q []queued
	readOrder := 0
	for pos := 0; pos < len(s); {
		n := strings.IndexByte(s[pos:], '\n') + 1
		if n == 0 {
			n = len(s) - pos
		}
		line := s[pos : pos+n]
		if start, dur, d, ok := assDialogue(line, readOrder); ok {
			readOrder++
			q = append(q, queued{pts: start, dur: dur, pos: int64(pos), data: d})
		} else {
			hdr.WriteString(line)
		}
		pos += n
	}
	q = finalizeQueue(q, true)
	events = make([]Event, len(q))
	for i, p := range q {
		events[i] = Event{Start: p.pts, Duration: p.dur, Dialog: p.data}
	}
	return hdr.String(), events
}

// assDialogue ports read_dialogue: it matches
// "Dialogue: %*[^,],%d:%d:%d%*c%d,%d:%d:%d%*c%d," and rebuilds the line as
// the "ReadOrder,Layer,Style,...,Text" packet ffmpeg's ASS decoder expects.
func assDialogue(line string, readOrder int) (start, dur int64, dialog string, ok bool) {
	if !strings.HasPrefix(line, "Dialogue:") {
		return 0, 0, "", false
	}
	s := strings.TrimLeft(line[9:], " \t\n\r\v\f")
	n := strcspn(s, ",")
	if n == 0 || n == len(s) {
		return 0, 0, "", false
	}
	s = s[n+1:]
	var v [8]int64
	for k := 0; k < 8; k++ {
		x, m := scanIntN(s)
		if m == 0 {
			return 0, 0, "", false
		}
		v[k] = int64(int32(x))
		s = s[m:]
		switch k {
		case 0, 1, 4, 5:
			if !strings.HasPrefix(s, ":") {
				return 0, 0, "", false
			}
			s = s[1:]
		case 2, 6:
			if s == "" {
				return 0, 0, "", false
			}
			s = s[1:]
		case 3, 7:
			if !strings.HasPrefix(s, ",") {
				return 0, 0, "", false
			}
			s = s[1:]
		}
	}
	start = ((v[0]*3600+v[1]*60+v[2])*100 + v[3]) * 10
	end := ((v[4]*3600+v[5]*60+v[6])*100 + v[7]) * 10
	layer := 0
	if len(line) > 10 {
		layer, _ = scanInt(line[10:])
	}
	d := strconv.Itoa(readOrder) + "," + strconv.Itoa(layer) + "," + s
	d = strings.TrimRight(d, "\r\n")
	return start, int64(int32(end/10-start/10)) * 10, d, true
}
