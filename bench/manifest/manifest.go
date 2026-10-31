// Package manifest describes the benchmark fixture corpus. The manifest is a
// JSON file (bench/manifest.json) shared by the fixture generator
// (cmd/gomedia-bench gen) and the end-to-end benchmarks in package bench.
//
// Adding a case means adding one entry to manifest.json. Benchmarks pick up
// every entry whose flags make it applicable to an operation (for example a
// fixture with "thumb": true is used by the thumbnail benchmarks).
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Kind classifies a fixture.
type Kind string

const (
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindSubtitle Kind = "subtitle" // sidecar text subtitle (srt, vtt, ass)
)

// Text describes a generated sidecar text file.
type Text struct {
	// Generator is "srt", "vtt", "ass" or "lrc".
	Generator string `json:"generator"`
	// Cues is the number of cues or lines to generate.
	Cues int `json:"cues"`
}

// Fixture is one benchmark input file. Exactly one of FFmpeg, CopyFrom,
// TruncateOf or Generate should be set; a fixture with none of them must
// already exist in the data directory.
type Fixture struct {
	Name        string   `json:"name"`
	File        string   `json:"file"`
	Kind        Kind     `json:"kind"`
	Container   string   `json:"container"`
	Codec       string   `json:"codec,omitempty"`
	Description string   `json:"description"`
	Thumb       bool     `json:"thumb,omitempty"`       // Thumbnail/H264 benchmarks apply
	Subtitles   bool     `json:"subtitles,omitempty"`   // has embedded subtitle streams
	Artwork     bool     `json:"artwork,omitempty"`     // has embedded cover art
	Corrupt     bool     `json:"corrupt,omitempty"`     // errors are expected; not a failure
	FFmpeg      []string `json:"ffmpeg,omitempty"`      // args; {dir} is the data dir, {out} the output file
	CopyFrom    string   `json:"copy_from,omitempty"`   // repo-relative path to copy
	TruncateOf  string   `json:"truncate_of,omitempty"` // fixture name to truncate
	TruncateTo  int64    `json:"truncate_bytes,omitempty"`
	Generate    *Text    `json:"generate,omitempty"`
	// Sidecars maps an extension (".lrc") to a generated text sidecar that
	// sits next to the fixture with the same base name.
	Sidecars map[string]Text `json:"sidecars,omitempty"`
}

// Manifest is the top-level manifest.json document.
type Manifest struct {
	Version  int       `json:"version"`
	Fixtures []Fixture `json:"fixtures"`
}

var nameRE = regexp.MustCompile(`^[a-z0-9_]+$`)

// Load reads and validates a manifest file.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// Validate checks names are unique, well formed and that file names are
// plain base names (no path separators), so they cannot escape the data dir.
func (m *Manifest) Validate() error {
	seen := map[string]bool{}
	for _, f := range m.Fixtures {
		if !nameRE.MatchString(f.Name) {
			return fmt.Errorf("fixture name %q must match %s", f.Name, nameRE)
		}
		if seen[f.Name] {
			return fmt.Errorf("duplicate fixture name %q", f.Name)
		}
		seen[f.Name] = true
		if f.File == "" || filepath.Base(f.File) != f.File {
			return fmt.Errorf("fixture %q: file must be a plain base name", f.Name)
		}
		if f.TruncateOf != "" && f.TruncateTo <= 0 {
			return fmt.Errorf("fixture %q: truncate_of needs truncate_bytes > 0", f.Name)
		}
	}
	return nil
}

// Find returns the fixture with the given name.
func (m *Manifest) Find(name string) (Fixture, bool) {
	for _, f := range m.Fixtures {
		if f.Name == name {
			return f, true
		}
	}
	return Fixture{}, false
}

// Where returns the fixtures for which keep returns true.
func (m *Manifest) Where(keep func(Fixture) bool) []Fixture {
	var out []Fixture
	for _, f := range m.Fixtures {
		if keep(f) {
			out = append(out, f)
		}
	}
	return out
}
