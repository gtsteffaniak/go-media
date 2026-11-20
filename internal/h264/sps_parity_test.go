package h264

// Parity tests between validateSPSNAL's own SPS parser and the mp4ff parser it
// replaces. They are removed together with the mp4ff dependency.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// legacyValidateSPSNAL is validateSPSNAL as it was with mp4ff.
func legacyValidateSPSNAL(nal []byte) error {
	sps, err := avc.ParseSPSNALUnit(nal, false)
	if err != nil {
		return errs.UnsupProfile("h264", fmt.Sprintf("sps: %v", err))
	}
	if reason, gated := gatedProfiles[int(sps.Profile)]; gated && !allowGatedProfiles.Load() {
		return errs.UnsupProfile("h264", reason)
	}
	if sps.ChromaFormatIDC != 1 {
		return errs.UnsupProfile("h264", fmt.Sprintf("chroma format %d (need 4:2:0)", sps.ChromaFormatIDC))
	}
	if sps.BitDepthLumaMinus8 > 2 || sps.BitDepthChromaMinus8 > 2 {
		return errs.UnsupProfile("h264", fmt.Sprintf("%d-bit luma / %d-bit chroma (profile_idc %d); decoder supports 8, 9 and 10 bits",
			sps.BitDepthLumaMinus8+8, sps.BitDepthChromaMinus8+8, sps.Profile))
	}
	if sps.MbAdaptiveFrameFieldFlag {
		return errs.UnsupProfile("h264", "MBAFF")
	}
	if !sps.FrameMbsOnlyFlag {
		return errs.UnsupProfile("h264", "interlaced frame encoding")
	}
	return nil
}

// normalizeSPSErr maps the two mp4ff error texts that were reworded to the new
// wording, so the rest of the message must match exactly.
func normalizeSPSErr(err error) string {
	if err == nil {
		return "<nil>"
	}
	s := err.Error()
	s = strings.Replace(s, "sps: non-vaild chroma_format_idc value:", "sps: invalid chroma_format_idc value:", 1)
	s = strings.Replace(s, "sps: GetSARFromIDC: SAR bad index", "sps: SAR bad index", 1)
	return s
}

// checkSPSParity validates nal with both parsers and fails on any difference in
// the result or the error text. Inputs whose pic_order_cnt cycle count would
// make mp4ff allocate a huge slice are not passed to mp4ff; the new parser must
// reject them.
func checkSPSParity(t testing.TB, name string, nal []byte) {
	t.Helper()
	if s, _ := parseSPSNAL(nal); s.pocCycleLen > 1<<16 {
		if err := validateSPSNAL(nal); err == nil {
			t.Fatalf("%s: SPS with %d-entry POC cycle accepted", name, s.pocCycleLen)
		}
		return
	}
	got, want := validateSPSNAL(nal), legacyValidateSPSNAL(nal)
	if normalizeSPSErr(got) != normalizeSPSErr(want) {
		t.Fatalf("%s: validateSPSNAL(% x)\n new: %v\nmp4ff: %v", name, nal, got, want)
	}
	if want != nil && !errors.Is(got, errs.UnsupportedProfile) {
		t.Fatalf("%s: error %v does not unwrap to UnsupportedProfile", name, got)
	}
}

// spsCorpus collects SPS NAL units from every file in the repository and in the
// directories listed in GOMEDIA_PARITY_DIRS: avcC records in MP4/MOV files and
// Matroska CodecPrivate, plus Annex B start-code streams.
func spsCorpus(t testing.TB) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	roots := []string{"../.."}
	if extra := os.Getenv("GOMEDIA_PARITY_DIRS"); extra != "" {
		roots = append(roots, strings.Split(extra, ":")...)
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() > 256<<20 {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for i, nal := range spsFromBytes(b) {
				out[fmt.Sprintf("%s#%d", p, i)] = nal
			}
			return nil
		})
	}
	return out
}

// spsFromBytes finds SPS NAL units in b: in avcC records (located by the
// "avcC" box type or a Matroska CodecPrivate that starts with version 1) and
// after Annex B start codes.
func spsFromBytes(b []byte) [][]byte {
	var out [][]byte
	seen := map[string]bool{}
	add := func(nal []byte) {
		if len(nal) > 0 && nal[0]&0x1F == 7 && !seen[string(nal)] {
			seen[string(nal)] = true
			out = append(out, append([]byte(nil), nal...))
		}
	}
	// avcC records.
	for i := 0; ; {
		j := bytes.Index(b[i:], []byte("avcC"))
		if j < 0 {
			break
		}
		rec := b[i+j+4:]
		if len(rec) > 8 && rec[0] == 1 {
			n := int(rec[5] & 0x1F)
			p := 6
			for k := 0; k < n && p+2 <= len(rec); k++ {
				l := int(binary.BigEndian.Uint16(rec[p:]))
				p += 2
				if p+l > len(rec) {
					break
				}
				add(rec[p : p+l])
				p += l
			}
		}
		i += j + 4
	}
	// Matroska CodecPrivate avcC records: 0x63A2 size, then version 1.
	for i := 0; ; {
		j := bytes.Index(b[i:], []byte{0x63, 0xA2})
		if j < 0 {
			break
		}
		p := i + j + 2
		if p < len(b) && b[p]&0x80 != 0 && p+1 < len(b) {
			rec := b[p+1:]
			if len(rec) > 8 && rec[0] == 1 {
				n := int(rec[5] & 0x1F)
				q := 6
				for k := 0; k < n && q+2 <= len(rec); k++ {
					l := int(binary.BigEndian.Uint16(rec[q:]))
					q += 2
					if q+l > len(rec) {
						break
					}
					add(rec[q : q+l])
					q += l
				}
			}
		}
		i += j + 2
	}
	// Annex B.
	for i := 0; i+3 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 || b[i+2] != 1 {
			continue
		}
		start := i + 3
		end := bytes.Index(b[start:], []byte{0, 0, 1})
		if end < 0 {
			end = len(b)
		} else {
			end += start
			if end > start && b[end-1] == 0 {
				end--
			}
		}
		if start < len(b) && b[start]&0x1F == 7 && end-start < 4096 {
			add(b[start:end])
		}
	}
	return out
}

// TestSPSParity compares the two parsers on every SPS in the corpus, every
// prefix of each, every single-bit flip, and each byte forced to 0x00, 0x03
// and 0xFF.
func TestSPSParity(t *testing.T) {
	corpus := spsCorpus(t)
	if len(corpus) == 0 {
		t.Fatal("no SPS NAL units found")
	}
	var nSPS, nMut, nAccepted int
	for name, nal := range corpus {
		nSPS++
		checkSPSParity(t, name, nal)
		if validateSPSNAL(nal) == nil {
			nAccepted++
		}
		mut := make([]byte, len(nal))
		for i := range nal {
			checkSPSParity(t, name, nal[:i])
			nMut++
			for bit := 0; bit < 8; bit++ {
				copy(mut, nal)
				mut[i] ^= 1 << bit
				checkSPSParity(t, name, mut)
				nMut++
			}
			for _, v := range []byte{0x00, 0x03, 0xFF} {
				copy(mut, nal)
				mut[i] = v
				checkSPSParity(t, name, mut)
				nMut++
			}
		}
	}
	t.Logf("parity: %d distinct SPS (%d accepted), %d mutants", nSPS, nAccepted, nMut)
}

// TestSPSParityGated repeats the corpus comparison with every profile gated,
// with and without AllowGatedProfiles.
func TestSPSParityGated(t *testing.T) {
	corpus := spsCorpus(t)
	for p := 0; p < 256; p++ {
		gatedProfiles[p] = "test gate"
	}
	defer func() {
		for p := 0; p < 256; p++ {
			delete(gatedProfiles, p)
		}
	}()
	for _, allow := range []bool{false, true} {
		AllowGatedProfiles(allow)
		for name, nal := range corpus {
			checkSPSParity(t, name, nal)
		}
	}
	AllowGatedProfiles(false)
}

// TestSPSParityCases checks every handmade spsCase against mp4ff.
func TestSPSParityCases(t *testing.T) {
	for _, tc := range spsCases() {
		checkSPSParity(t, tc.name, tc.nal)
		if s, _ := parseSPSNAL(tc.nal); s.pocCycleLen > 1<<16 {
			continue
		}
		got := normalizeSPSErr(legacyValidateSPSNAL(tc.nal))
		want := tc.wantErr
		if want == "" {
			want = "<nil>"
		}
		if got != want {
			t.Errorf("%s: mp4ff gives %q, case expects %q", tc.name, got, want)
		}
	}
}

// FuzzSPSParity compares the two parsers on arbitrary NAL units.
func FuzzSPSParity(f *testing.F) {
	for _, nal := range spsCorpus(f) {
		f.Add(nal)
	}
	for _, tc := range spsCases() {
		f.Add(tc.nal)
	}
	f.Add([]byte{0x67})
	f.Add([]byte{0x67, 0x64, 0x00, 0x1f, 0x00, 0x00, 0x03, 0x01})
	f.Fuzz(func(t *testing.T, nal []byte) {
		checkSPSParity(t, "fuzz", nal)
	})
}
