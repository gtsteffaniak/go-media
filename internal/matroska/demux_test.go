package matroska

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"
)

// Synthetic MJPEG Matroska builder for the demuxer tests. Files are generated
// at test time, so no large binaries are checked in.

const (
	idSimpleBlockTest = 0xA3
	idTimecodeTest    = 0xE7
)

// tinyJPEG returns a small JPEG whose top-left pixel encodes v, so tests can
// tell which frame was decoded.
func tinyJPEG(tb testing.TB, v uint8) []byte { return sizedJPEG(tb, v, 16, 16) }

// sizedJPEG returns a w x h JPEG with a flat level v.
func sizedJPEG(tb testing.TB, v uint8, w, h int) []byte {
	tb.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = v
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// simpleBlock encodes a SimpleBlock for track with a relative timecode.
func simpleBlock(track uint64, rel int16, key bool, payload []byte) []byte {
	b := []byte{0x80 | byte(track)}
	var tc [2]byte
	binary.BigEndian.PutUint16(tc[:], uint16(rel))
	b = append(b, tc[:]...)
	flags := byte(0)
	if key {
		flags = 0x80
	}
	b = append(b, flags)
	return ebmlElem(idSimpleBlockTest, append(b, payload...))
}

// mkvOpts configures buildMJPEGMKV.
type mkvOpts struct {
	clusters      int
	framesPer     int
	frameStepMs   int
	cues          bool
	extraBlocks   [][]byte // prepended to the first cluster
	jpegForFrame  func(i int) []byte
	durationMsArg float64
}

// buildMJPEGMKV builds a Matroska file with one V_MJPEG track (number 1),
// SeekHead, Info, Tracks, Clusters, and optionally Cues at the end (like
// ffmpeg's muxer).
func buildMJPEGMKV(tb testing.TB, o mkvOpts) []byte {
	tb.Helper()
	tracks := videoTrackTest("V_MJPEG")
	totalMs := o.clusters * o.framesPer * o.frameStepMs
	dur := float64(totalMs)
	if o.durationMsArg > 0 {
		dur = o.durationMsArg
	}
	info := infoTest(1_000_000, dur)
	// SeekHead with a fixed-size Cues position placeholder (8 bytes).
	seekHead := func(pos uint64) []byte {
		var p [8]byte
		binary.BigEndian.PutUint64(p[:], pos)
		return ebmlElem(idSeekHead, ebmlElem(idSeek,
			ebmlElem(idSeekID, []byte{0x1C, 0x53, 0xBB, 0x6B}),
			ebmlElem(idSeekPosition, p[:])))
	}
	sh := seekHead(0)
	body := append([]byte(nil), sh...)
	body = append(body, info...)
	body = append(body, tracks...)
	var cuePoints [][]byte
	frame := 0
	for c := 0; c < o.clusters; c++ {
		clusterPos := uint64(len(body))
		tc := c * o.framesPer * o.frameStepMs
		parts := [][]byte{u64Elem(idTimecodeTest, uint64(tc))}
		if c == 0 {
			parts = append(parts, o.extraBlocks...)
		}
		for f := 0; f < o.framesPer; f++ {
			var payload []byte
			if o.jpegForFrame != nil {
				payload = o.jpegForFrame(frame)
			}
			parts = append(parts, simpleBlock(1, int16(f*o.frameStepMs), true, payload))
			frame++
		}
		body = append(body, ebmlElem(idClusterTest, parts...)...)
		cuePoints = append(cuePoints, ebmlElem(idCuePoint,
			u64Elem(idCueTime, uint64(tc)),
			ebmlElem(idCueTrackPositions, u64Elem(idCueTrack, 1), u64Elem(idCueClusterPos, clusterPos))))
	}
	if o.cues {
		cuesPos := uint64(len(body))
		body = append(body, ebmlElem(idCues, cuePoints...)...)
		copy(body, seekHead(cuesPos))
	}
	out := ebmlHeaderTest()
	return append(out, ebmlElem(idSegmentTest, body)...)
}

// frameValueJPEG makes frame i's gray level i%250, so decoded output tells
// which frame was picked.
func frameValueJPEG(tb testing.TB) func(int) []byte {
	cache := map[uint8][]byte{}
	return func(i int) []byte {
		v := uint8(i % 250)
		if b, ok := cache[v]; ok {
			return b
		}
		b := tinyJPEG(tb, v)
		cache[v] = b
		return b
	}
}

// grayAt returns the decoded gray level at (0,0).
func grayAt(img *image.RGBA) uint8 {
	return color.GrayModel.Convert(img.At(0, 0)).(color.Gray).Y
}

func thumbAt(tb testing.TB, data []byte, pct float64) (*image.RGBA, *countingReader, bool) {
	tb.Helper()
	cr := &countingReader{r: bytes.NewReader(data)}
	h, err := ProbeHeader(cr, int64(len(data)))
	if err != nil {
		tb.Fatal(err)
	}
	img, ok := ThumbnailRGBA(cr, int64(len(data)), h, ThumbnailParams{SeekPercent: pct, MaxWidth: 16, MaxHeight: 16})
	return img, cr, ok
}

func TestMKVSeekPicksFrameAtOrBeforeTarget(t *testing.T) {
	for _, cues := range []bool{true, false} {
		data := buildMJPEGMKV(t, mkvOpts{clusters: 20, framesPer: 10, frameStepMs: 100, cues: cues, jpegForFrame: frameValueJPEG(t)})
		// 20s file; 50% = 10000ms = frame 100 exactly.
		img, _, ok := thumbAt(t, data, 50)
		if !ok {
			t.Fatalf("cues=%v: no thumbnail", cues)
		}
		if g := grayAt(img); g < 98 || g > 102 {
			t.Fatalf("cues=%v: gray %d, want ~100 (frame 100)", cues, g)
		}
	}
}

func TestMKVUndeclaredTrackBlockDoesNotPanic(t *testing.T) {
	jp := tinyJPEG(t, 50)
	// Track 9 is not declared in Tracks. mkvcore dereferenced nil on such a
	// block inside its goroutine and crashed the process.
	bad := [][]byte{simpleBlock(9, 0, true, []byte("junk")), simpleBlock(100, 0, false, []byte{1, 2, 3})}
	for _, cues := range []bool{true, false} {
		data := buildMJPEGMKV(t, mkvOpts{clusters: 3, framesPer: 2, frameStepMs: 500, cues: cues, extraBlocks: bad,
			jpegForFrame: func(int) []byte { return jp }})
		img, _, ok := thumbAt(t, data, 10)
		if !ok || img == nil {
			t.Fatalf("cues=%v: want thumbnail despite undeclared track block", cues)
		}
	}
}

func TestMKVSeekWithoutCuesSkipsClusterBodies(t *testing.T) {
	data := buildMJPEGMKV(t, mkvOpts{clusters: 400, framesPer: 10, frameStepMs: 100, cues: false, jpegForFrame: frameValueJPEG(t)})
	_, cr, ok := thumbAt(t, data, 90)
	if !ok {
		t.Fatal("no thumbnail")
	}
	if cr.bytes*20 > int64(len(data)) {
		t.Fatalf("read %d of %d bytes; cluster bodies should be skipped by size", cr.bytes, len(data))
	}
}

// TestMKVKeyframeGrabScaling checks that doubling the file length does not
// double the cost of a seek with Cues. Allocations must match exactly; bytes
// read may only grow by the extra Cues bytes; time must stay under 1.6x.
func TestMKVKeyframeGrabScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	// A 320x240 frame keeps decode cost in the measurement, as in production
	// (VP8/MJPEG decode dominates a real thumbnail).
	jp := sizedJPEG(t, 80, 320, 240)
	gen := func(n int) []byte {
		return buildMJPEGMKV(t, mkvOpts{clusters: n, framesPer: 5, frameStepMs: 200, cues: true, jpegForFrame: func(int) []byte { return jp }})
	}
	short, long := gen(600), gen(1200)
	probe := func(data []byte) (allocs float64, bytesRead int64) {
		_, cr, ok := thumbAt(t, data, 50)
		if !ok {
			t.Fatal("no thumbnail")
		}
		return testing.AllocsPerRun(20, func() { thumbAt(t, data, 50) }), cr.bytes
	}
	batch := func(data []byte) time.Duration {
		start := time.Now()
		for range 20 {
			thumbAt(t, data, 50)
		}
		return time.Since(start) / 20
	}
	// Alternate short and long batches and keep each side's best, so a noisy
	// CI runner slows both sides alike instead of skewing the ratio.
	timeBoth := func() (tS, tL time.Duration) {
		tS, tL = time.Duration(1<<62), time.Duration(1<<62)
		for range 10 {
			tS = min(tS, batch(short))
			tL = min(tL, batch(long))
		}
		return tS, tL
	}
	aS, bS := probe(short)
	aL, bL := probe(long)
	tS, tL := timeBoth()
	// Retry the timing (never the deterministic checks) before failing, since
	// a single burst of contention can still land on one side.
	for attempt := 0; attempt < 2 && float64(tL)/float64(tS) >= 1.6; attempt++ {
		tS, tL = timeBoth()
	}
	cuesDelta := int64(len(long)-len(short)) - int64(600*5*(len(jp)+ebmlBlockOverhead(len(jp)))) - 600*int64(len(u64Elem(idTimecodeTest, 0))+12)
	t.Logf("short: %d B file, %.0f allocs, %d B read, %v; long: %d B file, %.0f allocs, %d B read, %v; ratio %.2fx",
		len(short), aS, bS, tS, len(long), aL, bL, tL, float64(tL)/float64(tS))
	if aL != aS {
		t.Fatalf("allocs grew with length: %.0f -> %.0f", aS, aL)
	}
	if bL-bS != cuesDelta {
		t.Fatalf("bytes read grew by %d, want only the Cues growth %d", bL-bS, cuesDelta)
	}
	if r := float64(tL) / float64(tS); r >= 1.6 {
		t.Fatalf("doubling length scaled time by %.2fx (want < 1.6x)", r)
	}
}

// ebmlBlockOverhead is the SimpleBlock framing around a payload of n bytes:
// 1-byte ID, 8-byte size, 4-byte block header.
func ebmlBlockOverhead(n int) int { return 1 + 8 + 4 }

// mkvBenchData is shared by the length benchmarks.
func benchMKV(b *testing.B, clusters int) {
	jp := sizedJPEG(b, 80, 320, 240)
	data := buildMJPEGMKV(b, mkvOpts{clusters: clusters, framesPer: 5, frameStepMs: 200, cues: true, jpegForFrame: func(int) []byte { return jp }})
	r := bytes.NewReader(data)
	h, err := ProbeHeader(r, int64(len(data)))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, ok := ThumbnailRGBA(r, int64(len(data)), h, ThumbnailParams{SeekPercent: 50, MaxWidth: 16, MaxHeight: 16}); !ok {
			b.Fatal("no thumbnail")
		}
	}
}

// One 1-second cluster of five frames each: 60 clusters is 1 minute, 3600 is 1 hour.
func BenchmarkMKVThumbnailShort(b *testing.B) { benchMKV(b, 60) }
func BenchmarkMKVThumbnailLong(b *testing.B)  { benchMKV(b, 3600) }

// BenchmarkMKVSeekDemuxLong isolates the demux (index walk, Cues pick, and
// frame read) from JPEG decode on a 1-hour file.
func BenchmarkMKVSeekDemuxLong(b *testing.B) {
	jp := tinyJPEG(b, 80)
	data := buildMJPEGMKV(b, mkvOpts{clusters: 3600, framesPer: 5, frameStepMs: 200, cues: true, jpegForFrame: func(int) []byte { return jp }})
	r := bytes.NewReader(data)
	b.ReportAllocs()
	for b.Loop() {
		ix, err := indexSegment(r, int64(len(data)))
		if err != nil {
			b.Fatal(err)
		}
		fr, err := collectSeekFrames(r, ix, 1, 1_000_000, 1_800_000, false)
		if err != nil || len(fr) != 1 {
			b.Fatal("no frame", err)
		}
	}
}
