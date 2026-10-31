package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gtsteffaniak/go-media/bench/manifest"
)

// cmdGen generates every fixture in the manifest into the data directory.
// Generation order: sidecar text, copies, ffmpeg outputs, truncations.
// Existing files are kept unless -force is given, so the corpus is stable
// across runs; delete bench/.data (or pass -force) to regenerate.
func cmdGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "bench/manifest.json", "fixture manifest")
	dataDir := fs.String("data", "bench/.data", "output directory for generated fixtures")
	force := fs.Bool("force", false, "regenerate fixtures that already exist")
	only := fs.String("only", "", "regexp: generate only fixtures whose name matches")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := manifest.Load(*manifestPath)
	if err != nil {
		return err
	}
	var re *regexp.Regexp
	if *only != "" {
		if re, err = regexp.Compile(*only); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		return err
	}
	ffmpeg, ffErr := exec.LookPath("ffmpeg")
	if ffErr != nil {
		ffmpeg = ""
	}
	selected := m.Where(func(f manifest.Fixture) bool { return re == nil || re.MatchString(f.Name) })
	// The manifest lives at <repo>/bench/manifest.json; copy_from paths are relative to <repo>.
	repoRoot := filepath.Dir(filepath.Dir(*manifestPath))

	phases := []func(manifest.Fixture) bool{
		func(f manifest.Fixture) bool { return f.Generate != nil },
		func(f manifest.Fixture) bool { return f.CopyFrom != "" },
		func(f manifest.Fixture) bool { return len(f.FFmpeg) > 0 },
		func(f manifest.Fixture) bool { return f.TruncateOf != "" },
	}
	var failed []string
	for _, want := range phases {
		for _, f := range selected {
			if !want(f) {
				continue
			}
			out := filepath.Join(*dataDir, f.File)
			if err := genOne(m, f, out, *dataDir, repoRoot, ffmpeg, *force); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", f.Name, err))
				continue
			}
			for ext, sc := range f.Sidecars {
				sp := strings.TrimSuffix(out, filepath.Ext(out)) + ext
				if !*force && fileExists(sp) {
					continue
				}
				if err := writeText(sp, sc); err != nil {
					failed = append(failed, fmt.Sprintf("%s sidecar %s: %v", f.Name, ext, err))
				}
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d fixture(s) failed:\n  %s", len(failed), strings.Join(failed, "\n  "))
	}
	fmt.Printf("fixtures ready in %s (%d selected)\n", *dataDir, len(selected))
	return nil
}

func genOne(m *manifest.Manifest, f manifest.Fixture, out, dataDir, repoRoot, ffmpeg string, force bool) error {
	if !force && fileExists(out) {
		return nil
	}
	switch {
	case f.Generate != nil:
		return writeText(out, *f.Generate)
	case f.CopyFrom != "":
		// copy_from is a repo-relative path from the manifest. Reject absolute
		// paths and parent references so the manifest cannot read outside the repo.
		if filepath.IsAbs(f.CopyFrom) || strings.Contains(f.CopyFrom, "..") {
			return fmt.Errorf("copy_from must be a relative path inside the repo: %q", f.CopyFrom)
		}
		return copyFile(filepath.Join(repoRoot, f.CopyFrom), out)
	case f.TruncateOf != "":
		src, ok := m.Find(f.TruncateOf)
		if !ok {
			return fmt.Errorf("truncate_of references unknown fixture %q", f.TruncateOf)
		}
		return truncateFile(filepath.Join(dataDir, src.File), out, f.TruncateTo)
	case len(f.FFmpeg) > 0:
		if ffmpeg == "" {
			return fmt.Errorf("ffmpeg not found on PATH (needed to generate %s)", f.File)
		}
		return runFFmpeg(ffmpeg, f.FFmpeg, out, dataDir, repoRoot)
	}
	return fmt.Errorf("no generator and file missing: %s", out)
}

// runFFmpeg runs ffmpeg with an argument vector (no shell). Placeholders
// {dir}, {repo} and {out} are replaced; the manifest is trusted developer input.
func runFFmpeg(ffmpeg string, argv []string, out, dataDir, repoRoot string) error {
	// Keep the real extension on the temp name so ffmpeg picks the muxer.
	tmp := out + ".tmp" + filepath.Ext(out)
	full := []string{"-y", "-hide_banner", "-loglevel", "error"}
	for _, a := range argv {
		switch a {
		case "{out}":
			full = append(full, tmp)
		default:
			a = strings.ReplaceAll(a, "{dir}", dataDir)
			full = append(full, strings.ReplaceAll(a, "{repo}", repoRoot))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpeg, full...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg %s: %v: %s", filepath.Base(out), err, strings.TrimSpace(stderr.String()))
	}
	return os.Rename(tmp, out)
}

func writeText(path string, t manifest.Text) error {
	var b strings.Builder
	switch t.Generator {
	case "srt":
		for i := 1; i <= t.Cues; i++ {
			start := time.Duration(i-1) * time.Second
			fmt.Fprintf(&b, "%d\n%s --> %s\nCue %d: naïve café <i>italic</i> line\nsecond line %d\n\n",
				i, srtTime(start), srtTime(start+800*time.Millisecond), i, i)
		}
	case "vtt":
		b.WriteString("WEBVTT\n\n")
		for i := 1; i <= t.Cues; i++ {
			start := time.Duration(i-1) * time.Second
			fmt.Fprintf(&b, "cue-%d\n%s --> %s line:0 align:start\n<b>Cue %d</b> with <c.yellow>tag</c>\n\n",
				i, vttTime(start), vttTime(start+800*time.Millisecond), i)
		}
	case "ass":
		b.WriteString("[Script Info]\nScriptType: v4.00+\nPlayResX: 1280\nPlayResY: 720\n\n")
		b.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
		b.WriteString("Style: Default,Arial,40,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1\n\n")
		b.WriteString("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
		for i := 1; i <= t.Cues; i++ {
			start := time.Duration(i-1) * time.Second
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,{\\b1}Event %d{\\b0}\\Nsecond line\n",
				assTime(start), assTime(start+800*time.Millisecond), i)
		}
	case "lrc":
		b.WriteString("[ti:Bench]\n[ar:go-media]\n")
		for i := 1; i <= t.Cues; i++ {
			start := time.Duration(i-1) * time.Second
			fmt.Fprintf(&b, "[%02d:%02d.%02d]Lyric line %d\n", int(start.Minutes()), int(start.Seconds())%60, (i*7)%100, i)
		}
	default:
		return fmt.Errorf("unknown text generator %q", t.Generator)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func srtTime(d time.Duration) string {
	h, m, s, ms := split(d)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func vttTime(d time.Duration) string {
	h, m, s, ms := split(d)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func assTime(d time.Duration) string {
	h, m, s, ms := split(d)
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, ms/10)
}

func split(d time.Duration) (h, m, s, ms int) {
	total := int64(d / time.Millisecond)
	ms = int(total % 1000)
	total /= 1000
	s = int(total % 60)
	total /= 60
	m = int(total % 60)
	h = int(total / 60)
	return
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy source: %w", err)
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func truncateFile(src, dst string, n int64) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("truncate source: %w", err)
	}
	defer in.Close()
	buf := make([]byte, n)
	read, err := io.ReadFull(in, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return err
	}
	return os.WriteFile(dst, buf[:read], 0o644)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
