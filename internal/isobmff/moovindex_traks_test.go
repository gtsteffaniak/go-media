package isobmff

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// paddedReader serves data and then zeros up to size, standing in for the
// mdat of a long file without allocating it.
type paddedReader struct {
	data []byte
	size int64
}

func (p paddedReader) ReadAt(b []byte, off int64) (int, error) {
	if off >= p.size {
		return 0, io.EOF
	}
	n := 0
	if off < int64(len(p.data)) {
		n = copy(b, p.data[off:])
	}
	for i := n; i < len(b) && off+int64(i) < p.size; i++ {
		b[i] = 0
		n++
	}
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// lpcmTrak is uncompressed 16-bit stereo audio at 48 kHz: one 4-byte sample
// per audio frame, so long recordings exceed maxSamples.
func lpcmTrak(samples uint32) []byte {
	return synthSideTrak("soun", "sowt",
		synthBox("stts", u32s(0, 1, samples, 1)),
		synthBox("stsc", u32s(0, 1, 1, samples, 1)),
		synthBox("stsz", u32s(0, 4, samples)),
		synthBox("stco", u32s(0, 1, 0)),
	)
}

func TestLongLPCMAudioTrakWithH264(t *testing.T) {
	const audioSamples = maxSamples + 1_000_000 // ~196 s of 48 kHz LPCM
	data := buildSynthMP4(synthMP4Opts{Samples: 300, GOP: 30, ExtraTraks: [][]byte{lpcmTrak(audioSamples)}})
	r := paddedReader{data: data, size: int64(len(data)) + 4*audioSamples}
	track, ok := FirstAVCTrackIndex(r, r.size)
	if !ok || track != 1 {
		t.Fatalf("FirstAVCTrackIndex = %d, %v; want 1, true", track, ok)
	}
	p, ok := H264SyncSample(r, r.size, track, 50)
	if !ok || synthSampleIndex(p) != 150 {
		t.Fatalf("H264SyncSample = %d, %v; want sample 150", synthSampleIndex(p), ok)
	}
	if _, ok := OpenH264MP4Thumb(r, r.size); !ok {
		t.Fatal("OpenH264MP4Thumb failed")
	}
	// The audio trak's tables are fine to seek but too long to expand.
	if _, err := SampleRefs(r, r.size, 0); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("SampleRefs(audio) err = %v; want unsupported", err)
	}
}

func TestCorruptNonVideoTrakIgnored(t *testing.T) {
	badText := synthSideTrak("text", "tx3g", synthBox("stts", u32s(0, 1<<20))) // truncated run table
	badAudio := synthSideTrak("soun", "mp4a", synthBox("stsz", u32s(0, 0, 1<<20)))
	data := buildSynthMP4(synthMP4Opts{Samples: 300, GOP: 30, ExtraTraks: [][]byte{badText, badAudio}})
	r := bytes.NewReader(data)
	idx, err := openMovieIndex(r, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := idx.track(i); err == nil {
			t.Fatalf("track(%d) indexed a corrupt trak", i)
		}
	}
	track, ok := FirstAVCTrackIndex(r, int64(len(data)))
	if !ok || track != 2 {
		t.Fatalf("FirstAVCTrackIndex = %d, %v; want 2, true", track, ok)
	}
	if p, ok := H264SyncSample(r, int64(len(data)), track, 10); !ok || synthSampleIndex(p) != 30 {
		t.Fatalf("H264SyncSample = %d, %v; want sample 30", synthSampleIndex(p), ok)
	}
}

func TestCorruptVideoTrakUnsupported(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 300, GOP: 30, BadVideoStts: true,
		ExtraTraks: [][]byte{lpcmTrak(48000)}})
	r := bytes.NewReader(data)
	size := int64(len(data))
	track, ok := FirstAVCTrackIndex(r, size)
	if !ok || track != 1 {
		t.Fatalf("FirstAVCTrackIndex = %d, %v; want the corrupt trak 1", track, ok)
	}
	if _, ok := H264SyncSample(r, size, track, 10); ok {
		t.Fatal("H264SyncSample succeeded on a corrupt video trak")
	}
	if _, ok := OpenH264MP4Thumb(r, size); ok {
		t.Fatal("OpenH264MP4Thumb succeeded on a corrupt video trak")
	}
	if _, err := SampleRefs(r, size, track); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("SampleRefs err = %v; want unsupported", err)
	}
}
