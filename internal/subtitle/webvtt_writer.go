package subtitle

import (
	"strconv"
	"strings"
)

// This file ports ffmpeg's WebVTT output path, release 7.1:
//   - libavcodec/webvttenc.c (webvtt encoder: ASS dialog -> cue text),
//   - libavcodec/ass_split.c (dialog splitting, style table, override codes),
//   - libavformat/webvttenc.c (webvtt muxer: header, timestamps, cue layout).
//
// ffmpeg converts every text subtitle to an ASS dialog first, and the webvtt
// encoder renders that dialog. go-media does the same, so each input format
// only needs a port of its own ffmpeg decoder (see decoders.go), and all of
// them share one renderer. That is what makes the output byte-identical to
// "ffmpeg -i in -map 0:s:N out.vtt".

// Event is one decoded subtitle event, in the form ffmpeg's subtitle decoders
// hand to its webvtt encoder.
type Event struct {
	// Start is the presentation time in milliseconds.
	Start int64
	// Duration is the display time in milliseconds. The cue ends at
	// Start+Duration; ffmpeg writes a zero-length cue when it is 0.
	Duration int64
	// Dialog is an ASS dialog line without the "Dialogue:" prefix and times:
	// "ReadOrder,Layer,Style,Name,MarginL,MarginR,MarginV,Effect,Text".
	Dialog string
}

// DefaultDialog wraps decoded ASS text in the dialog line ffmpeg's
// ff_ass_add_rect builds for decoders without their own styles (SubRip,
// WebVTT, plain text, mov_text): layer 0, style "Default", no name or effect.
func DefaultDialog(readOrder int, text string) string {
	return strconv.Itoa(readOrder) + ",0,Default,,0,0,0,," + text
}

// WriteWebVTT renders events as ffmpeg's webvtt encoder and muxer do. header
// is the ASS subtitle header (script info and styles) used to resolve dialog
// styles; pass "" for formats whose ffmpeg decoder uses the default header,
// whose Default style has no bold, italic or underline.
//
// Every event becomes a cue, even one whose rendered text is empty, as with
// the ffmpeg CLI; callers drop the packets ffmpeg's demuxers and decoders
// drop (for example empty SubRip cues). With no events the result is just "WEBVTT\n",
// which is what ffmpeg writes for a track with no cues.
func WriteWebVTT(header string, events []Event) string {
	// One renderer for the whole track: ffmpeg keeps its tag stack in the
	// encoder context, so tags left open by a malformed override block are
	// closed at the end of the next cue, exactly as here.
	r := &vttRenderer{styles: parseASSStyles(header)}
	var b strings.Builder
	b.WriteString("WEBVTT\n")
	for _, ev := range events {
		// A dialog that renders to nothing still becomes a cue with an empty
		// payload: the ffmpeg CLI muxes the zero-size encoder output anyway.
		text := r.render(ev.Dialog)
		dur := ev.Duration
		if dur < 0 {
			// ffmpeg carries the duration in AVSubtitle.end_display_time, a
			// uint32 in milliseconds, so a negative duration that survived the
			// demuxer (only possible on the last cue) wraps around.
			dur = int64(uint32(dur))
		}
		b.WriteByte('\n')
		writeVTTTime(&b, ev.Start)
		b.WriteString(" --> ")
		writeVTTTime(&b, ev.Start+dur)
		b.WriteByte('\n')
		b.WriteString(text)
		b.WriteByte('\n')
	}
	return b.String()
}

// writeVTTTime writes a timestamp the way ffmpeg's webvtt muxer does: MM:SS.mmm,
// with an "HH:" prefix only when the hour is not zero. This is the shorter of
// the two forms the WebVTT spec allows, and ffmpeg always picks it.
func writeVTTTime(b *strings.Builder, ms int64) {
	sec := ms / 1000
	ms -= 1000 * sec
	min := sec / 60
	sec -= 60 * min
	hour := min / 60
	min -= 60 * hour
	if hour > 0 {
		pad2(b, hour)
		b.WriteByte(':')
	}
	pad2(b, min)
	b.WriteByte(':')
	pad2(b, sec)
	b.WriteByte('.')
	s := strconv.FormatInt(ms, 10)
	for i := len(s); i < 3; i++ {
		b.WriteByte('0')
	}
	b.WriteString(s)
}

// pad2 writes v with at least two digits, like printf "%02d".
func pad2(b *strings.Builder, v int64) {
	if v >= 0 && v < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.FormatInt(v, 10))
}

// assStyle is the part of an ASS style ffmpeg's webvtt encoder reads.
type assStyle struct {
	name                    string
	bold, italic, underline int
}

// assStyles is the style table of an ASS header, in file order.
type assStyles []assStyle

// get returns the first style named name, or nil. An empty name means
// "Default", as in ffmpeg's ff_ass_style_get.
func (s assStyles) get(name string) *assStyle {
	if name == "" {
		name = "Default"
	}
	for i := range s {
		if s[i].name == name {
			return &s[i]
		}
	}
	return nil
}

// assSection describes one ASS header section, mirroring ffmpeg's ass_sections
// table. Only the style fields the encoder uses are kept; the others are
// listed by name so a Format line maps columns the same way ffmpeg does.
type assSection struct {
	name         string
	formatHeader string
	fieldsHeader string
	fields       []string
}

var assSections = []assSection{
	{name: "Script Info"},
	{name: "V4+ Styles", formatHeader: "Format", fieldsHeader: "Style", fields: []string{
		"Name", "Fontname", "Fontsize", "PrimaryColour", "SecondaryColour", "OutlineColour",
		"BackColour", "Bold", "Italic", "Underline", "StrikeOut", "ScaleX", "ScaleY", "Spacing",
		"Angle", "BorderStyle", "Outline", "Shadow", "Alignment", "MarginL", "MarginR", "MarginV",
		"Encoding"}},
	{name: "V4 Styles", formatHeader: "Format", fieldsHeader: "Style", fields: []string{
		"Name", "Fontname", "Fontsize", "PrimaryColour", "SecondaryColour", "TertiaryColour",
		"BackColour", "Bold", "Italic", "BorderStyle", "Outline", "Shadow", "Alignment",
		"MarginL", "MarginR", "MarginV", "AlphaLevel", "Encoding"}},
	{name: "Events", formatHeader: "Format", fieldsHeader: "Dialogue", fields: []string{
		"Layer", "Start", "End", "Style", "Name", "MarginL", "MarginR", "MarginV", "Effect", "Text"}},
}

// parseASSStyles ports the style part of ffmpeg's ff_ass_split. Both
// [V4+ Styles] (ASS) and [V4 Styles] (SSA) fill the same table, so an SSA
// style with Bold -1 renders as bold, as it does in ffmpeg. A Format line
// sets the column order; without one ffmpeg's default order is assumed.
// Field names match by prefix, as ffmpeg's strncmp does.
func parseASSStyles(header string) assStyles {
	header = strings.TrimPrefix(header, "\xef\xbb\xbf")
	var styles assStyles
	cur := -1
	orders := make([][]int, len(assSections))
	for len(header) > 0 {
		var line string
		if i := strings.IndexByte(header, '\n'); i >= 0 {
			line, header = header[:i], header[i+1:]
		} else {
			// ffmpeg's "[%15[...]]%c" needs a character after "]", so a
			// final section line without a newline does not open a section.
			line, header = header, ""
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				line = ""
			}
		}
		if strings.HasPrefix(line, "[") {
			cur = -1
			if name, ok := sectionName(line); ok {
				for i, s := range assSections {
					if s.name == name {
						cur = i
					}
				}
			}
			continue
		}
		if cur < 0 {
			continue
		}
		if line == "" || line[0] == ';' || strings.HasPrefix(line, "!:") {
			continue
		}
		sec := &assSections[cur]
		klen := strings.IndexAny(line, ":\r")
		if klen < 0 {
			klen = len(line)
		}
		if klen < len(line) && line[klen] == ':' && (sec.fieldsHeader == "" || !isPrefix(line[:klen], sec.fieldsHeader)) {
			for i, s := range assSections {
				if s.fieldsHeader != "" && isPrefix(line[:klen], s.fieldsHeader) {
					cur, sec = i, &assSections[i]
					break
				}
			}
		}
		if sec.formatHeader != "" && orders[cur] == nil && strings.HasPrefix(line, sec.formatHeader+":") {
			orders[cur] = parseFormatLine(sec, line[len(sec.formatHeader)+1:])
			continue
		}
		if sec.fieldsHeader == "" || !strings.HasPrefix(line, sec.fieldsHeader+":") {
			continue
		}
		if orders[cur] == nil {
			orders[cur] = make([]int, len(sec.fields))
			for i := range orders[cur] {
				orders[cur][i] = i
			}
		}
		if sec.fieldsHeader != "Style" {
			continue
		}
		styles = append(styles, parseStyleLine(sec, orders[cur], line[len(sec.fieldsHeader)+1:]))
	}
	return styles
}

// sectionName parses "[Name]" the way ffmpeg's sscanf "[%15[0-9A-Za-z+ ]]%c" does.
func sectionName(line string) (string, bool) {
	end := strings.IndexByte(line, ']')
	if end < 2 || end > 16 || end+1 >= len(line)+1 {
		return "", false
	}
	name := line[1:end]
	for _, c := range name {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '+' || c == ' ') {
			return "", false
		}
	}
	return name, true
}

// isPrefix reports whether key is a prefix of name, as strncmp(key, name, len(key)) == 0.
func isPrefix(key, name string) bool {
	return len(key) <= len(name) && name[:len(key)] == key
}

// isEOL matches ffmpeg's is_eol for one byte (NUL is the end of the string here).
func isEOL(s string, i int) bool {
	return i >= len(s) || s[i] == '\r' || s[i] == '\n'
}

// skipSpace skips ASCII spaces only, like ffmpeg's skip_space.
func skipSpace(s string, i int) int {
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

// parseFormatLine maps Format columns to section fields; -1 marks a column
// ffmpeg does not know.
func parseFormatLine(sec *assSection, s string) []int {
	order := []int{}
	i := 0
	for !isEOL(s, i) {
		i = skipSpace(s, i)
		n := strcspn(s[i:], ", \r\n")
		col := s[i : i+n]
		idx := -1
		for f, name := range sec.fields {
			if isPrefix(col, name) {
				idx = f
				break
			}
		}
		order = append(order, idx)
		i += n
		if i < len(s) && s[i] == ',' {
			i++
		}
		i = skipSpace(s, i)
	}
	return order
}

// parseStyleLine reads one "Style:" line's values into an assStyle.
func parseStyleLine(sec *assSection, order []int, s string) assStyle {
	var st assStyle
	i := 0
	for k := 0; !isEOL(s, i) && k < len(order); k++ {
		last := k == len(order)-1
		i = skipSpace(s, i)
		stop := ",\r\n"
		if last {
			stop = "\r\n"
		}
		n := strcspn(s[i:], stop)
		val := s[i : i+n]
		if order[k] >= 0 {
			switch sec.fields[order[k]] {
			case "Name":
				st.name = val
			case "Bold":
				st.bold, _ = scanInt(val)
			case "Italic":
				st.italic, _ = scanInt(val)
			case "Underline":
				st.underline, _ = scanInt(val)
			}
		}
		i += n
		if !last && i < len(s) {
			i++
		}
		i = skipSpace(s, i)
	}
	return st
}

// strcspn returns the length of the prefix of s with no byte from set.
func strcspn(s, set string) int {
	if i := strings.IndexAny(s, set); i >= 0 {
		return i
	}
	return len(s)
}

// scanInt parses a leading decimal integer like sscanf "%d": leading
// whitespace, an optional sign, then at least one digit.
func scanInt(s string) (int, bool) {
	v, n := scanIntN(s)
	return int(v), n > 0
}

// scanIntN is scanInt that also returns how many bytes were consumed (0 on failure).
func scanIntN(s string) (int64, int) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\v' || s[i] == '\f') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	start := i
	var v int64
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		v = v*10 + int64(s[i]-'0')
		i++
	}
	if i == start {
		return 0, 0
	}
	if neg {
		v = -v
	}
	return v, i
}

// splitDialog returns the Style and Text fields of a dialog line, ported from
// ffmpeg's ff_ass_split_dialog: nine comma-separated fields, spaces skipped
// before each, and Text is everything after the eighth comma, so commas in
// the text are kept.
func splitDialog(d string) (style, text string) {
	i := 0
	for f := 0; f < 9; f++ {
		i = skipSpace(d, i)
		n := len(d) - i
		if f < 8 {
			n = strcspn(d[i:], ",")
		}
		switch f {
		case 2:
			style = d[i : i+n]
		case 8:
			text = d[i : i+n]
		}
		i += n
		if i < len(d) {
			i++
		}
	}
	return style, text
}

// vttRenderer is ffmpeg's WebVTTContext: the output buffer and the stack of
// open <b>, <i> and <u> tags.
type vttRenderer struct {
	styles assStyles
	b      strings.Builder
	stack  []byte
}

// vttStackSize is ffmpeg's WEBVTT_STACK_SIZE.
const vttStackSize = 64

// pushPop is webvtt_stack_push_pop. close is non-zero for a closing code;
// c == 0 closes every open tag.
func (r *vttRenderer) pushPop(c byte, close int) {
	if close != 0 {
		i := 0
		if c != 0 {
			i = -1
			for k := len(r.stack) - 1; k >= 0; k-- {
				if r.stack[k] == c {
					i = k
					break
				}
			}
		}
		if i < 0 {
			return
		}
		for len(r.stack) != i {
			top := r.stack[len(r.stack)-1]
			r.stack = r.stack[:len(r.stack)-1]
			r.b.WriteString("</" + string(top) + ">")
		}
	} else if len(r.stack) < vttStackSize {
		r.stack = append(r.stack, c)
	}
}

// styleApply opens tags for a style's bold, italic and underline flags. Any
// non-zero value counts, so SSA's -1 for "on" works.
func (r *vttRenderer) styleApply(name string) {
	st := r.styles.get(name)
	if st == nil {
		return
	}
	if st.bold != 0 {
		r.b.WriteString("<b>")
		r.push('b')
	}
	if st.italic != 0 {
		r.b.WriteString("<i>")
		r.push('i')
	}
	if st.underline != 0 {
		r.b.WriteString("<u>")
		r.push('u')
	}
}

// push is webvtt_stack_push; on overflow the tag is not tracked.
func (r *vttRenderer) push(c byte) {
	if len(r.stack) < vttStackSize {
		r.stack = append(r.stack, c)
	}
}

// render turns one ASS dialog into WebVTT cue text, porting
// webvtt_encode_frame with ff_ass_split_override_codes. Kept from ASS:
// \b, \i and \u become <b>, <i> and <u>; \N and \n become line breaks;
// \r resets to a style. Everything else in an override block (colors,
// fonts, \an, \pos, karaoke, unknown codes) is dropped, and \h is not
// treated specially, matching ffmpeg. Text is written as-is, without HTML
// escaping, as ffmpeg does.
func (r *vttRenderer) render(dialog string) string {
	r.b.Reset()
	style, text := splitDialog(dialog)
	r.styleApply(style)
	if r.overrideCodes(text) {
		r.pushPop(0, 1)
	}
	return r.b.String()
}

// overrideCodes walks text and reports whether it reached the end. A
// malformed override block stops it early, and then ffmpeg does not close
// open tags either.
func (r *vttRenderer) overrideCodes(buf string) bool {
	textStart := -1
	flush := func(end int) {
		if textStart >= 0 {
			r.b.WriteString(buf[textStart:end])
			textStart = -1
		}
	}
	i := 0
	for i < len(buf) {
		isNL := buf[i] == '\\' && i+1 < len(buf) && (buf[i+1] == 'n' || buf[i+1] == 'N')
		isBlock := buf[i] == '{' && i+1 < len(buf) && buf[i+1] == '\\'
		if isNL || isBlock {
			flush(i)
		}
		switch {
		case isNL:
			r.b.WriteByte('\n')
			i += 2
		case isBlock:
			i++
			for i < len(buf) && buf[i] == '\\' {
				i = r.overrideCode(buf, i)
			}
			if i >= len(buf) || buf[i] != '}' {
				return false
			}
			i++
		default:
			if textStart < 0 {
				textStart = i
			}
			i++
		}
	}
	flush(len(buf))
	return true
}

// overrideCode handles one "\code" at buf[i] and returns the index of the
// separator after it ("\" or "}"), or the end of the string.
func (r *vttRenderer) overrideCode(buf string, i int) int {
	// "\\%1[bisu]%1[01\\}]": a style toggle.
	if i+2 < len(buf) && strings.IndexByte("bisu", buf[i+1]) >= 0 && strings.IndexByte("01\\}", buf[i+2]) >= 0 {
		c := buf[i+1]
		close := -1
		next := i + 2
		switch buf[i+2] {
		case '0':
			close, next = 1, i+3
		case '1':
			close, next = 0, i+3
		}
		if c != 's' { // strikethrough is not supported by WebVTT
			r.pushPop(c, close)
			if close == 0 {
				r.b.WriteString("<" + string(c) + ">")
			}
		}
		return next
	}
	end := i + 1 + strcspn(buf[i+1:], "\\}")
	// "\\r%127[^\\}]": reset to the named style, or the dialog default.
	// ffmpeg's earlier patterns (\c, \alpha, \fn, \fs, \a) never start
	// with "r", so this check order is the same as ffmpeg's.
	if i+1 < len(buf) && buf[i+1] == 'r' && end < len(buf) && (end-(i+2)) <= 127 {
		r.pushPop(0, 1)
		r.styleApply(buf[i+2 : end])
	}
	return end
}
