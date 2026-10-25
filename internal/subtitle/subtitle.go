package subtitle

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseSRT converts SubRip input to WebVTT.
func ParseSRT(r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	var cues []string
	var idx int
	var start, end string
	var text []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			if start != "" && len(text) > 0 {
				cues = append(cues, cueBlock(start, end, strings.Join(text, "\n")))
				start, end, text = "", "", nil
			}
			continue
		}
		if _, err := strconv.Atoi(line); err == nil && start == "" {
			idx++
			continue
		}
		if strings.Contains(line, "-->") {
			parts := strings.Split(line, "-->")
			if len(parts) == 2 {
				start = toVTTTime(strings.TrimSpace(parts[0]))
				end = toVTTTime(strings.TrimSpace(parts[1]))
			}
			continue
		}
		if start != "" {
			text = append(text, line)
		}
	}
	if start != "" && len(text) > 0 {
		cues = append(cues, cueBlock(start, end, strings.Join(text, "\n")))
	}
	if len(cues) == 0 {
		return "", fmt.Errorf("no cues")
	}
	return "WEBVTT\n\n" + strings.Join(cues, "\n\n") + "\n", nil
}

// ParseVTT normalizes WebVTT (passthrough with header guarantee).
func ParseVTT(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	s := string(data)
	if !strings.HasPrefix(s, "WEBVTT") {
		return "", fmt.Errorf("invalid vtt")
	}
	return s, nil
}

var assTime = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2})\.(\d{2,3})`)

// ParseASS converts SSA/ASS dialogue lines to WebVTT (text only).
func ParseASS(r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	var cues []string
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		parts := strings.SplitN(line, ",", 10)
		if len(parts) < 10 {
			continue
		}
		start := toVTTTime(parts[1])
		end := toVTTTime(parts[2])
		text := stripASSTags(parts[9])
		cues = append(cues, cueBlock(start, end, text))
	}
	if len(cues) == 0 {
		return "", fmt.Errorf("no dialogue")
	}
	return "WEBVTT\n\n" + strings.Join(cues, "\n\n") + "\n", nil
}

func cueBlock(start, end, text string) string {
	return start + " --> " + end + "\n" + text
}

func toVTTTime(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ".", ",")
	if strings.Count(s, ":") == 1 {
		s = "00:" + s
	}
	// drop extra styling after timestamp in srt
	if i := strings.Index(s, " "); i >= 0 {
		s = s[:i]
	}
	return s
}

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
