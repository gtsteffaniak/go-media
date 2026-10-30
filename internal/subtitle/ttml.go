package subtitle

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrEmpty indicates no cues were found in subtitle input.
var ErrEmpty = fmt.Errorf("subtitle: empty")

// TTMLOrXMLToWebVTT converts TTML / STPP XML payload into minimal WebVTT.
func TTMLOrXMLToWebVTT(xml string) (string, error) {
	xml = strings.TrimSpace(xml)
	if xml == "" {
		return "", ErrEmpty
	}
	re := regexp.MustCompile(`<p[^>]*begin="([^"]+)"[^>]*end="([^"]+)"[^>]*>([\s\S]*?)</p>`)
	matches := re.FindAllStringSubmatch(xml, -1)
	if len(matches) == 0 {
		re2 := regexp.MustCompile(`<p[^>]*>([\s\S]*?)</p>`)
		texts := re2.FindAllStringSubmatch(xml, -1)
		if len(texts) == 0 {
			return "", ErrEmpty
		}
		var cues []string
		for i, m := range texts {
			body := stripXMLTags(m[1])
			if body == "" {
				continue
			}
			start := formatVTTSeconds(float64(i) * 2)
			end := formatVTTSeconds(float64(i+1) * 2)
			cues = append(cues, start+" --> "+end+"\n"+body)
		}
		return "WEBVTT\n\n" + strings.Join(cues, "\n\n") + "\n", nil
	}
	type ttmlCue struct{ start, end, body string }
	var parsed []ttmlCue
	for _, m := range matches {
		body := stripXMLTags(m[3])
		if body == "" {
			continue
		}
		parsed = append(parsed, ttmlCue{ttmlTimeToVTT(m[1]), ttmlTimeToVTT(m[2]), body})
	}
	if len(parsed) == 0 {
		return "", ErrEmpty
	}
	var cues []string
	for i, c := range parsed {
		// An end before the start is clamped as in WriteWebVTT: to the next
		// later cue's start, else to the start (zero length). The timestamps
		// are fixed-width HH:MM:SS.mmm, so string order is time order.
		if c.end < c.start {
			c.end = c.start
			for _, n := range parsed[i+1:] {
				if n.start > c.start {
					c.end = n.start
					break
				}
			}
		}
		cues = append(cues, c.start+" --> "+c.end+"\n"+c.body)
	}
	return "WEBVTT\n\n" + strings.Join(cues, "\n\n") + "\n", nil
}

func stripXMLTags(s string) string {
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func ttmlTimeToVTT(t string) string {
	t = strings.TrimSpace(t)
	if strings.Contains(t, ":") {
		return toVTTTime(t)
	}
	if strings.HasSuffix(t, "s") {
		t = strings.TrimSuffix(t, "s")
	}
	sec, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return "00:00:00.000"
	}
	return formatVTTSeconds(sec)
}

func formatVTTSeconds(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int64(sec * 1000)
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
