package subtitle

import (
	"encoding/binary"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file ports ffmpeg 7.1's text subtitle decoders, each of which turns
// one packet into ASS dialog text for WriteWebVTT:
//   - SubRipToASS: libavcodec/srtdec.c + htmlsubtitles.c (SubRip / S_TEXT/UTF8),
//   - WebVTTToASS: libavcodec/webvttdec.c (WebVTT / D_WEBVTT / S_TEXT/WEBVTT),
//   - PlainTextToASS: libavcodec/textdec.c (S_TEXT/ASCII),
//   - MovText: libavcodec/movtextdec.c (MP4 tx3g).
// ASS and SSA need no decoder: their packets already are dialog lines.

// SubRipToASS converts SubRip text with HTML-like markup to ASS text, as
// ffmpeg's ff_htmlmarkup_to_ass does. <b>, <i>, <u> and <s> (any case)
// become ASS toggles, <br> becomes a line break, <font> becomes size, color
// and face overrides, and other tag-like markup is dropped. Spaces at line
// starts are dropped and a blank line ends the cue. HTML entities are left
// as-is, as ffmpeg leaves them.
//
// One intentional simplification: ffmpeg only emits a color override for a
// <font color> it can parse (named colors or hex); go-media treats any
// value as valid. The override never reaches WebVTT output, so this only
// matters for the rare cue that ends with such a tag after a line break.
func SubRipToASS(in string) string {
	var dst []byte
	type fontTag struct {
		face  string
		size  uint64
		color uint32
	}
	var stack [16]fontTag
	sptr, an := 0, 0
	lineStart, closingBraceMissing := true, false
	rstrip := func() {
		for len(dst) > 0 && dst[len(dst)-1] == ' ' {
			dst = dst[:len(dst)-1]
		}
	}
	at := func(i int) byte {
		if i < len(in) {
			return in[i]
		}
		return 0
	}
	end := false
	for i := 0; !end && i < len(in) && in[i] != 0; i++ {
		switch in[i] {
		case '\r':
		case '\n':
			if lineStart {
				end = true
				break
			}
			rstrip()
			dst = append(dst, '\\', 'N')
			lineStart = true
		case ' ':
			if !lineStart {
				dst = append(dst, ' ')
			}
		case '{':
			// handle_open_brace: drop {\...} override blocks (but keep the
			// first {\anN}) and MicroDVD-style {Y:...} codes.
			if strings.HasPrefix(in[i:], "{\\an") && isDigit(at(i+4)) && at(i+5) == '}' {
				an++
			}
			if !closingBraceMissing &&
				((an != 1 && at(i+1) == '\\') || (at(i+1) != 0 && strings.IndexByte("CcFfoPSsYy", at(i+1)) >= 0 && at(i+2) == ':')) {
				if j := strings.IndexByte(in[min(i+2, len(in)):], '}'); j >= 0 {
					i = min(i+2, len(in)) + j
					break
				}
				closingBraceMissing = true
			}
			dst = append(dst, '{')
		case '<':
			likely := true
			for at(i+1) == '<' {
				dst = append(dst, '<')
				likely = false
				i++
			}
			tagClose := at(i+1) == '/'
			if tagClose {
				likely = true
			}
			off := i + 1
			if tagClose {
				off++
			}
			buffer, n, ok := scanTag(in, off)
			if !ok || n <= 0 {
				dst = append(dst, '<')
				break
			}
			skip := n
			if tagClose {
				skip++
			}
			tagname := buffer
			for strings.HasPrefix(tagname, " ") {
				likely = false
				tagname = tagname[1:]
			}
			param, hasParam := "", false
			if j := strings.IndexByte(tagname, ' '); j >= 0 {
				tagname, param, hasParam = tagname[:j], tagname[j+1:], true
			}
			for k := 0; k < len(tagname); k++ {
				c := tagname[k]
				if !(isDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '/') {
					likely = false
					break
				}
			}
			switch {
			case strings.EqualFold(tagname, "font"):
				if tagClose && sptr > 0 {
					cur := stack[sptr]
					sptr--
					last := stack[sptr]
					if cur.size != 0 {
						if last.size == 0 {
							dst = append(dst, "{\\fs}"...)
						} else if last.size != cur.size {
							dst = append(dst, "{\\fs"+strconv.FormatInt(int64(int32(last.size)), 10)+"}"...)
						}
					}
					if cur.color&0xff000000 != 0 {
						if last.color&0xff000000 == 0 {
							dst = append(dst, "{\\c}"...)
						} else if last.color != cur.color {
							dst = append(dst, "{\\c&H"+strings.ToUpper(strconv.FormatUint(uint64(last.color&0xffffff), 16))+"&}"...)
						}
					}
					if cur.face != "" {
						if last.face == "" {
							dst = append(dst, "{\\fn}"...)
						} else if last.face != cur.face {
							dst = append(dst, "{\\fn"+last.face+"}"...)
						}
					}
				} else if !tagClose && sptr < len(stack)-1 {
					nt := stack[sptr]
					sptr++
					for hasParam {
						lower := strings.ToLower(param)
						switch {
						case strings.HasPrefix(lower, "size="):
							param = param[5:]
							if strings.HasPrefix(param, "\"") {
								param = param[1:]
							}
							if v, n := scanUint(param); n > 0 {
								nt.size = v
								dst = append(dst, "{\\fs"+strconv.FormatUint(uint64(uint32(v)), 10)+"}"...)
							}
						case strings.HasPrefix(lower, "color="):
							param = param[6:]
							if strings.HasPrefix(param, "\"") {
								param = param[1:]
							}
							if strcspn(strings.TrimLeft(param, "#"), "\" >") > 0 {
								// See the doc comment: the real color value
								// is not needed for WebVTT output.
								nt.color = 0xff000000
								dst = append(dst, "{\\c&H0&}"...)
							}
						case strings.HasPrefix(lower, "face="):
							quoted := len(param) > 5 && param[5] == '"'
							param = param[5:]
							if quoted {
								param = param[1:]
							}
							stop := " "
							if quoted {
								stop = "\""
							}
							l := strcspn(param, stop)
							nt.face = param[:min(l, 127)]
							param = param[l:]
							dst = append(dst, "{\\fn"+nt.face+"}"...)
						}
						if j := strings.IndexByte(param, ' '); j >= 0 {
							param = param[j+1:]
						} else {
							hasParam = false
						}
					}
					stack[sptr] = nt
				}
				i += skip
			case len(tagname) == 1 && strings.IndexByte("bisu", lower(tagname[0])) >= 0:
				v := byte('1')
				if tagClose {
					v = '0'
				}
				dst = append(dst, '{', '\\', lower(tagname[0]), v, '}')
				i += skip
			case len(tagname) >= 2 && strings.EqualFold(tagname[:2], "br") && (len(tagname) == 2 || tagname[2:] == "/"):
				dst = append(dst, '\\', 'N')
				i += skip
			case likely:
				i += skip
			default:
				dst = append(dst, '<')
			}
		default:
			dst = append(dst, in[i])
		}
		if c := at(i); c != ' ' && c != '\r' && c != '\n' {
			lineStart = false
		}
	}
	for len(dst) >= 2 && dst[len(dst)-2] == '\\' && dst[len(dst)-1] == 'N' {
		dst = dst[:len(dst)-2]
	}
	rstrip()
	return string(dst)
}

// scanTag reads a tag body starting at in[off] up to '>' (at most 127
// bytes), like ffmpeg's scantag. n counts the '>' too.
func scanTag(in string, off int) (body string, n int, ok bool) {
	for l := 0; l < 128; l++ {
		if off+l >= len(in) {
			return "", 0, false
		}
		switch in[off+l] {
		case 0, '<':
			return "", 0, false
		case '>':
			return in[off : off+l], l + 1, true
		}
	}
	return "", 0, false
}

// scanUint parses like sscanf "%u" (leading space, optional sign, digits).
func scanUint(s string) (uint64, int) {
	v, n := scanIntN(s)
	return uint64(v), n
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lower lowercases an ASCII letter.
func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// webvttReplace is ffmpeg 9.0.2's webvtt_tag_replace table, tried in order.
var webvttReplace = [][2]string{
	{"{", "\\{{}"}, {"\\", "\\\u2060"},
	{"&gt;", ">"}, {"&lt;", "<"},
	{"&lrm;", "\u200e"}, {"&rlm;", "\u200f"},
	{"&amp;", "&"}, {"&nbsp;", "\\h"},
}

// webvttValidTags is ffmpeg 9.0.2's webvtt_valid_tags table. A tag matches
// when its name starts with the entry (strncmp over the entry's length), so
// classed tags such as <i.loud> count as <i>, and so does any tag whose name
// merely begins with i, b or u.
var webvttValidTags = [][2]string{
	{"i", "{\\i1}"}, {"/i", "{\\i0}"},
	{"b", "{\\b1}"}, {"/b", "{\\b0}"},
	{"u", "{\\u1}"}, {"/u", "{\\u0}"},
}

// WebVTTToASS converts a WebVTT cue payload to ASS text, porting ffmpeg
// 9.0.2's webvtt_event_to_ass. startMs and endMs are the cue's start and
// end time in milliseconds.
//
//   - Tags whose name starts with i, b or u (with or without classes) become
//     the ASS bold/italic/underline overrides; every other tag (<c.x>,
//     <v Name>, <ruby>, ...) is dropped with its content kept. A "<" with no
//     closing ">" ends the cue text.
//   - Inline cue timestamps (<00:01.500>) strictly inside the cue and in
//     increasing order split the text into {\kf} karaoke segments, which the
//     WebVTT encoder then drops; other timestamps are dropped like tags.
//   - The entities &gt; &lt; &amp; &lrm; &rlm; and &nbsp; are decoded; others
//     stay literal. A "{" becomes "\{{}" and a "\" gets a U+2060 word joiner
//     after it, which then appear literally in the WebVTT output.
//
// Version note: ffmpeg 7.1 only recognised exact <i>, <b> and <u> tags
// (classed tags were dropped) and had no karaoke timing; go-media follows
// 9.0.2, the version the golden tests pin.
func WebVTTToASS(p string, startMs, endMs int64) string {
	if i := strings.IndexByte(p, 0); i >= 0 {
		p = p[:i]
	}
	var buf, seg strings.Builder
	flush := func(durCs int64) {
		if durCs > 0 {
			buf.WriteString("{\\kf" + strconv.FormatInt(durCs, 10) + "}")
		}
		buf.WriteString(seg.String())
		seg.Reset()
	}
	prevTs := int64(-1)
	var startCs int64
	for i := 0; i < len(p); {
		if p[i] == '<' {
			end := strings.IndexByte(p[i:], '>')
			if end < 0 {
				break
			}
			body := p[i+1 : i+end]
			i += end + 1
			if ts, ok := readCueTimestamp(body, startMs, endMs, prevTs); ok {
				endCs := (ts - startMs + 5) / 10
				flush(endCs - startCs)
				startCs = endCs
				prevTs = ts
				continue
			}
			for _, t := range webvttValidTags {
				if strings.HasPrefix(body, t[0]) {
					seg.WriteString(t[1])
					break
				}
			}
			// ffmpeg tries the replacement table at the next position in the
			// same pass and then restarts the loop; doing it on the next
			// iteration is equivalent.
			continue
		}
		replaced := false
		for _, r := range webvttReplace {
			if strings.HasPrefix(p[i:], r[0]) {
				seg.WriteString(r[1])
				i += len(r[0])
				replaced = true
				break
			}
		}
		if replaced {
			continue
		}
		switch c := p[i]; {
		case c == '\n' && i+1 < len(p):
			seg.WriteString("\\N")
		case c != '\r':
			seg.WriteByte(c)
		}
		i++
	}
	if prevTs < 0 {
		flush(0)
	} else {
		flush((endMs-startMs+5)/10 - startCs)
	}
	return buf.String()
}

// readCueTimestamp ports ffmpeg 9.0.2's read_cue_timestamp: body must be
// only digits, colons and periods, start with a digit, parse as a WebVTT
// timestamp, lie strictly inside (cueStart, cueEnd) and come after prevTs.
func readCueTimestamp(body string, cueStart, cueEnd, prevTs int64) (int64, bool) {
	if body == "" || !isDigit(body[0]) {
		return 0, false
	}
	for i := 0; i < len(body); i++ {
		if c := body[i]; !isDigit(c) && c != ':' && c != '.' {
			return 0, false
		}
	}
	ts := parseWebVTTTimestamp(body)
	if ts <= cueStart || ts >= cueEnd || (prevTs >= 0 && ts <= prevTs) {
		return 0, false
	}
	return ts, true
}

// parseWebVTTTimestamp ports ffmpeg's parse_webvtt_timestamp, including its
// sscanf field widths: "%d:%2d:%2d.%3d", then "%2d:%2d.%3d". A short
// fraction is taken as a number, so "00:01.5" is 1.005 s, as in ffmpeg.
// It returns -1 when neither form matches or minutes/seconds exceed 59.
func parseWebVTTTimestamp(s string) int64 {
	if v, ok := scanFields(s, []int{0, 2, 2, 3}, "::."); ok {
		h, m, sec, ms := v[0], v[1], v[2], v[3]
		if m > 59 || sec > 59 {
			return -1
		}
		return h*3600000 + m*60000 + sec*1000 + ms
	}
	if v, ok := scanFields(s, []int{2, 2, 3}, ":."); ok {
		m, sec, ms := v[0], v[1], v[2]
		if m > 59 || sec > 59 {
			return -1
		}
		return m*60000 + sec*1000 + ms
	}
	return -1
}

// scanFields emulates sscanf with "%Nd" conversions (width 0 = unlimited)
// separated by the literal bytes in seps, on input that holds only digits,
// colons and periods. Trailing input after the last field is ignored.
func scanFields(s string, widths []int, seps string) ([]int64, bool) {
	out := make([]int64, 0, len(widths))
	i := 0
	for f, w := range widths {
		if f > 0 {
			if i >= len(s) || s[i] != seps[f-1] {
				return nil, false
			}
			i++
		}
		j := i
		var v int64
		for j < len(s) && isDigit(s[j]) && (w == 0 || j-i < w) {
			if v < 1<<40 {
				v = v*10 + int64(s[j]-'0')
			}
			j++
		}
		if j == i {
			return nil, false
		}
		out = append(out, v)
		i = j
	}
	return out, true
}

// PlainTextToASS converts raw text to ASS text, porting ffmpeg's text
// decoder (ff_ass_bprint_text_event without custom line breaks): "{" and
// "\" are escaped as the WebVTT decoder does, and line breaks become \N
// unless they end the packet.
func PlainTextToASS(p string) string {
	var b strings.Builder
	for i := 0; i < len(p) && p[i] != 0; i++ {
		switch c := p[i]; {
		case c == '{':
			b.WriteString("\\{{}")
		case c == '\\':
			if len(p)-i <= 3 || p[i+1:i+4] != "\u2060" {
				b.WriteString("\\\u2060")
			} else {
				b.WriteByte('\\')
			}
		case c == '\n':
			if i < len(p)-1 {
				b.WriteString("\\N")
			}
		case c == '\r' && i < len(p)-1 && p[i+1] == '\n':
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// movTextStyle is one tx3g StyleRecord without its character range.
type movTextStyle struct {
	start, end int
	fontID     uint16
	flags      byte
	fontSize   byte
	rgba       uint32
}

// same is ffmpeg's styles_equivalent.
func (s movTextStyle) same(o movTextStyle) bool {
	return s.fontID == o.fontID && s.flags&7 == o.flags&7 && s.fontSize == o.fontSize && s.rgba == o.rgba
}

// MovText decodes MP4 timed text (3GPP tx3g, ffmpeg's mov_text) samples.
type MovText struct {
	def    movTextStyle
	header string
}

// NewMovText reads the default style from a tx3g sample description. extra
// is the sample entry body after its 8-byte SampleEntry header (reserved
// bytes and data reference index), which is what ffmpeg stores as the
// codec extradata. An entry shorter than ffmpeg's 40-byte minimum falls back
// to a plain default style, as in ffmpeg.
func NewMovText(extra []byte) *MovText {
	m := &MovText{}
	if len(extra) >= 40 {
		m.def = parseStyleRecord(extra[22:30])
		m.header = "[V4+ Styles]\nFormat: Name, Bold, Italic, Underline\nStyle: Default," +
			strconv.Itoa(int(m.def.flags&1)) + "," + strconv.Itoa(int(m.def.flags>>1&1)) + "," +
			strconv.Itoa(int(m.def.flags>>2&1)) + "\n"
	}
	return m
}

// Header returns the ASS header to pass to WriteWebVTT, carrying the
// default style's bold, italic and underline flags.
func (m *MovText) Header() string { return m.header }

// parseStyleRecord reads fontID, face flags, size and color (8 bytes).
func parseStyleRecord(b []byte) movTextStyle {
	return movTextStyle{fontID: binary.BigEndian.Uint16(b), flags: b[2], fontSize: b[3], rgba: binary.BigEndian.Uint32(b[4:])}
}

// Decode converts one sample to ASS text. ok is false for samples ffmpeg
// emits no subtitle for: the 2-byte empty samples that mark gaps, and
// malformed ones. Bold, italic and underline from 'styl' boxes are kept;
// highlight, karaoke, wrap and font changes only produce ASS codes the
// WebVTT renderer drops, so they are not decoded.
func (m *MovText) Decode(sample []byte) (text string, ok bool) {
	if len(sample) <= 2 {
		return "", false
	}
	textLen := int(binary.BigEndian.Uint16(sample))
	end := min(2+textLen, len(sample))
	var styles []movTextStyle
	for box := sample[end:]; len(box) >= 8; {
		size := uint64(binary.BigEndian.Uint32(box))
		typ := string(box[4:8])
		hdr := uint64(8)
		if size == 1 {
			if len(box) < 16 {
				break
			}
			size, hdr = binary.BigEndian.Uint64(box[8:]), 16
		}
		if size < hdr {
			return "", false
		}
		size -= hdr
		body := box[hdr:]
		if uint64(len(body)) < size {
			break
		}
		if typ == "styl" && size >= 2 {
			styles = m.decodeStyl(body[:size])
		}
		box = body[size:]
	}
	return m.textToASS(sample[2:end], styles), true
}

// decodeStyl ports decode_styl: entries must be ordered and not overlap,
// empty ranges and entries equal to the default style are skipped, and
// equal neighbours are merged.
func (m *MovText) decodeStyl(b []byte) []movTextStyle {
	n := int(binary.BigEndian.Uint16(b))
	if 2+n*12 > len(b) {
		return nil
	}
	var out []movTextStyle
	for k := 0; k < n; k++ {
		r := b[2+12*k:]
		s := parseStyleRecord(r[4:12])
		s.start, s.end = int(binary.BigEndian.Uint16(r)), int(binary.BigEndian.Uint16(r[2:]))
		if s.end < s.start || (len(out) > 0 && s.start < out[len(out)-1].end) {
			return nil
		}
		switch {
		case s.start == s.end, s.same(m.def):
		case len(out) > 0 && s.start == out[len(out)-1].end && s.same(out[len(out)-1]):
			out[len(out)-1].end = s.end
		default:
			out = append(out, s)
		}
	}
	return out
}

// textToASS ports text_to_ass for the codes WebVTT keeps. Positions count
// UTF-8 characters; an invalid byte counts as one character.
func (m *MovText) textToASS(text []byte, styles []movTextStyle) string {
	var b strings.Builder
	entry, pos := 0, 0
	for len(text) > 0 {
		if entry < len(styles) {
			if pos == styles[entry].end {
				b.WriteString("{\\r}")
				entry++
			}
			if entry < len(styles) && pos == styles[entry].start {
				s := styles[entry]
				for bit, code := range []string{"b", "i", "u"} {
					if (s.flags^m.def.flags)>>bit&1 != 0 {
						b.WriteString("{\\" + code + strconv.Itoa(int(s.flags>>bit&1)) + "}")
					}
				}
			}
		}
		_, l := utf8.DecodeRune(text)
		switch text[0] {
		case '\r':
		case '\n':
			b.WriteString("\\N")
		default:
			b.Write(text[:l])
		}
		text = text[l:]
		pos++
	}
	return b.String()
}
