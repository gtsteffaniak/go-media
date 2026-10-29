package isobmff

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

func TestSampleAtTimeRunLengthStts(t *testing.T) {
	// 3 samples x 10 ticks, 2 samples x 0 ticks, 4 samples x 5 ticks.
	st := sampleTable{sampleCount: 9, stts: []uint32{3, 10, 2, 0, 4, 5}}
	cases := []struct {
		t    uint64
		want int
	}{{0, 0}, {9, 0}, {10, 1}, {29, 2}, {30, 5}, {34, 5}, {35, 6}, {49, 8}, {50, 8}, {1 << 40, 8}}
	for _, c := range cases {
		if got := st.sampleAtTime(c.t); got != c.want {
			t.Errorf("sampleAtTime(%d) = %d, want %d", c.t, got, c.want)
		}
	}
	times := []uint64{0, 10, 20, 30, 30, 30, 35, 40, 45}
	for i, want := range times {
		if got := st.sampleTime(i); got != want {
			t.Errorf("sampleTime(%d) = %d, want %d", i, got, want)
		}
	}
	if got := st.totalTicks(); got != 50 {
		t.Errorf("totalTicks = %d, want 50", got)
	}
	empty := sampleTable{}
	if got := empty.sampleAtTime(100); got != 0 {
		t.Errorf("empty stts sampleAtTime = %d", got)
	}
}

func TestChunkOfRunLengthStsc(t *testing.T) {
	// chunks 1-2: 3 samples, chunk 3: 1 sample, chunks 4-5: 2 samples.
	st := sampleTable{sampleCount: 11, chunkCount: 5, stsc: []uint32{1, 3, 1, 3, 1, 1, 4, 2, 1}}
	want := []struct {
		chunk uint32
		first int
	}{{0, 0}, {0, 0}, {0, 0}, {1, 3}, {1, 3}, {1, 3}, {2, 6}, {3, 7}, {3, 7}, {4, 9}, {4, 9}}
	for i, w := range want {
		c, f, err := st.chunkOf(i)
		if err != nil || c != w.chunk || f != w.first {
			t.Errorf("chunkOf(%d) = %d,%d,%v want %d,%d", i, c, f, err, w.chunk, w.first)
		}
	}
	if _, _, err := st.chunkOf(11); err == nil {
		t.Error("expected error past last chunk")
	}
	bad := sampleTable{sampleCount: 4, chunkCount: 2, stsc: []uint32{2, 1, 1, 1, 1, 1}}
	if _, _, err := bad.chunkOf(0); err == nil {
		t.Error("expected error for non-increasing stsc")
	}
	zero := sampleTable{sampleCount: 4, chunkCount: 2, stsc: []uint32{0, 1, 1}}
	if _, _, err := zero.chunkOf(0); err == nil {
		t.Error("expected error for first_chunk 0")
	}
}

// TestSyntheticLayouts checks seek, locate, and table iteration across stco/co64,
// table/fixed stsz, odd chunking, moov placement, and missing stss.
func TestSyntheticLayouts(t *testing.T) {
	layouts := []synthMP4Opts{
		{Samples: 1000, GOP: 30, SamplesPerChunk: 7},
		{Samples: 1000, GOP: 30, SamplesPerChunk: 7, Co64: true},
		{Samples: 1000, GOP: 30, SamplesPerChunk: 1, FixedSize: true},
		{Samples: 1000, GOP: 25, SamplesPerChunk: 10, MoovAfterMdat: true},
		{Samples: 1000, GOP: 0, SamplesPerChunk: 13}, // no stss: all intra
		{Samples: 1, GOP: 1, SamplesPerChunk: 1},
	}
	for _, o := range layouts {
		data := buildSynthMP4(o)
		r := bytes.NewReader(data)
		size := int64(len(data))
		track, ok := FirstAVCTrackIndex(r, size)
		if !ok || track != 0 {
			t.Fatalf("%+v: FirstAVCTrackIndex = %d,%v", o, track, ok)
		}
		if _, ok := AVCDecoderConfig(r, size, track); !ok {
			t.Fatalf("%+v: AVCDecoderConfig failed", o)
		}
		for _, pct := range []float64{0, 1, 10, 33.3, 50, 99, 100, 250} {
			p, ok := H264SyncSample(r, size, track, pct)
			if !ok {
				t.Fatalf("%+v pct %v: no sample", o, pct)
			}
			target := int(float64(o.Samples*synthDelta) * normalizeSeekPercent(pct) / 100 / synthDelta)
			if target >= o.Samples {
				target = o.Samples - 1
			}
			want := target
			if o.GOP > 0 {
				want = target - target%o.GOP
			}
			if got := synthSampleIndex(p); got != want {
				t.Errorf("%+v pct %v: sample %d, want %d (at-or-before %d)", o, pct, got, want, target)
			}
			sec, ok := H264SyncTimeSec(r, size, track, pct)
			if !ok || sec != float64(want*synthDelta)/synthTimescale {
				t.Errorf("%+v pct %v: time %v,%v want sample %d", o, pct, sec, ok, want)
			}
		}
		refs, err := SampleRefs(r, size, track)
		if err != nil || len(refs) != o.Samples {
			t.Fatalf("%+v: SampleRefs len %d err %v", o, len(refs), err)
		}
		for i, ref := range refs {
			if ref.Index != i || ref.Size != synthSampleSize || synthSampleIndex(data[ref.Offset:ref.Offset+ref.Size]) != i {
				t.Fatalf("%+v: bad ref %d: %+v", o, i, ref)
			}
		}
		kf, err := KeyframeSamples(r, size, track)
		if err != nil {
			t.Fatal(err)
		}
		gop := o.GOP
		if gop == 0 {
			gop = 1
		}
		if len(kf) != (o.Samples+gop-1)/gop {
			t.Fatalf("%+v: %d keyframes", o, len(kf))
		}
		for _, k := range kf {
			if k.Index%gop != 0 || refs[k.Index] != k {
				t.Fatalf("%+v: keyframe %+v mismatches %+v", o, k, refs[k.Index])
			}
		}
		sn, err := SyncSampleNumbers(r, size, track)
		if err != nil || len(sn) != len(kf) || int(sn[0]) != 1 {
			t.Fatalf("%+v: SyncSampleNumbers %d %v", o, len(sn), err)
		}
	}
}

// sparseReaderAt serves a header prefix and zeros up to a large virtual size.
type sparseReaderAt struct {
	head []byte
	size int64
}

func (s sparseReaderAt) ReadAt(p []byte, off int64) (int, error) {
	for i := range p {
		p[i] = 0
		if o := off + int64(i); o < int64(len(s.head)) {
			p[i] = s.head[o]
		}
	}
	return len(p), nil
}

func TestOversizedMoovUnsupported(t *testing.T) {
	head := synthBox("ftyp", []byte("isom"), u32s(0))
	moov := make([]byte, 8)
	binary.BigEndian.PutUint32(moov, maxMoovSize+1)
	copy(moov[4:], "moov")
	head = append(head, moov...)
	r := sparseReaderAt{head: head, size: int64(len(head)) + maxMoovSize + 1}
	_, err := openMovieIndex(r, r.size)
	if !errors.Is(err, errs.Unsupported) {
		t.Fatalf("oversized moov: err = %v, want unsupported", err)
	}
	if _, ok := H264SyncSample(r, r.size, 0, 10); ok {
		t.Fatal("expected no sample for oversized moov")
	}
}

func TestNoMoovAndFragmentedUnsupported(t *testing.T) {
	noMoov := append(synthBox("ftyp", []byte("isom"), u32s(0)), synthBox("mdat", make([]byte, 64))...)
	if _, err := openMovieIndex(bytes.NewReader(noMoov), int64(len(noMoov))); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("no moov: err = %v", err)
	}
	// Fragmented layout: moov with an empty sample table (as written with
	// empty_moov), samples living in moof/mdat.
	frag := buildSynthMP4(synthMP4Opts{Samples: 0, GOP: 1})
	r := bytes.NewReader(frag)
	idx, err := openMovieIndex(r, int64(len(frag)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.tracks[0].keyframeAt(10); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("fragmented: err = %v, want unsupported", err)
	}
	if _, ok := H264SyncSample(r, int64(len(frag)), 0, 10); ok {
		t.Fatal("fragmented: expected no sample")
	}
	if _, err := KeyframeSamples(r, int64(len(frag)), 0); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("fragmented KeyframeSamples: err = %v", err)
	}
}

func TestLargesizeAndTruncatedBoxes(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 50, GOP: 10, SamplesPerChunk: 5})
	// Rewrite ftyp header as a 64-bit largesize box by inserting 8 bytes.
	ftypLen := int(binary.BigEndian.Uint32(data))
	ls := make([]byte, 16)
	binary.BigEndian.PutUint32(ls, 1)
	copy(ls[4:], "ftyp")
	binary.BigEndian.PutUint64(ls[8:], uint64(ftypLen+8))
	moved := append(append(ls, data[8:ftypLen]...), data[ftypLen:]...)
	// Chunk offsets now point 8 bytes early; index must still parse moov.
	if _, err := openMovieIndex(bytes.NewReader(moved), int64(len(moved))); err != nil {
		t.Fatalf("largesize ftyp: %v", err)
	}
	for _, n := range []int{0, 7, 20, len(data) / 3} {
		if _, err := openMovieIndex(bytes.NewReader(data[:n]), int64(n)); err == nil && n < ftypLen+8 {
			t.Errorf("truncated at %d: expected error", n)
		}
	}
}

// FuzzMovieIndex feeds arbitrary bytes through the moov index and seek paths;
// they must never panic or read outside the input.
func FuzzMovieIndex(f *testing.F) {
	f.Add(buildSynthMP4(synthMP4Opts{Samples: 20, GOP: 5, SamplesPerChunk: 3}))
	f.Add(buildSynthMP4(synthMP4Opts{Samples: 9, GOP: 0, SamplesPerChunk: 2, Co64: true, MoovAfterMdat: true}))
	f.Add(buildSynthMP4(synthMP4Opts{Samples: 4, GOP: 2, FixedSize: true}))
	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		size := int64(len(data))
		idx, err := openMovieIndex(r, size)
		if err != nil {
			return
		}
		for i := range idx.tracks {
			idx.h264SyncSample(i, 10)
			idx.h264SyncSample(i, 100)
			idx.tracks[i].avcConfig()
			_, _ = KeyframeSamples(r, size, i)
			_, _ = SampleRefs(r, size, i)
		}
	})
}
