package mpeg4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// The fixtures are single I-VOP raw streams made with
//
//	ffmpeg -ss 2 -i src.mp4 -frames:v 1 -vf scale=176:144 <opts> -f m4v x.m4v
//
// and the hashes are of ffmpeg's decode of the same file:
//
//	ffmpeg -idct simple -i x.m4v -f rawvideo -pix_fmt yuv420p -
//
// so a match means the reconstruction is bit-exact with ffmpeg.
var fixtures = []struct {
	file, opts, sha string
	w, h            int
	quant           string
	interlaced      bool
}{
	{"h263_q4.m4v", "-c:v mpeg4 -q:v 4", "64297de395bfeb25f8f10d5ce209d9372f43d717b387670f3b5b36be6f673c98", 176, 144, "h263", false},
	{"h263_q31.m4v", "-c:v mpeg4 -q:v 31", "929e00d7f6b65da93f8dd3eef643d7af8249e7af417fe095805b315ffe5a5c41", 176, 144, "h263", false},
	{"mpegquant.m4v", "-c:v mpeg4 -mpeg_quant 1 -q:v 3", "8939f0728bb62f80d6f52a4ff6e296b6644b597354f8fb74d8296c291f9c7112", 176, 144, "mpeg", false},
	{"aic_ps.m4v", "-c:v mpeg4 -q:v 2 -ps 100 -flags +aic (resync markers)", "25faa69a023c40789c6d251ff2a60a1bc788f53eadc93402c12047abd1d8d722", 176, 144, "h263", false},
	{"ildct.m4v", "-c:v mpeg4 -q:v 3 -flags +ildct+ilme", "be6c97ab853d25d82c0a0b8bda2377da759502490a6f8b8d4c4128cfa0b654ed", 176, 144, "h263", true},
	{"xvid.m4v", "-c:v libxvid -q:v 3", "c6d90b71f2488f3718b0617eb24baf182a4535a2045af6616b889fa35aa77cd9", 176, 144, "h263", false},
	{"odd.m4v", "-vf scale=99:75 -c:v mpeg4 -q:v 2", "00d24b7726bb4b383c23876aeb609b3c24cdf48cd73bf43ac37986ea63c65b22", 99, 75, "h263", false},
}

// i420 returns the cropped planes of pic in I420 order.
func i420(pic *Picture) []byte {
	var out []byte
	for y := 0; y < pic.Height; y++ {
		out = append(out, pic.Y[y*pic.YStride:y*pic.YStride+pic.Width]...)
	}
	cw, ch := (pic.Width+1)/2, (pic.Height+1)/2
	for _, p := range [][]byte{pic.Cb, pic.Cr} {
		for y := 0; y < ch; y++ {
			out = append(out, p[y*pic.CStride:y*pic.CStride+cw]...)
		}
	}
	return out
}

// splitConfig splits a raw stream into the headers before the first VOP
// (what an MP4 esds or Matroska CodecPrivate carries) and the VOP.
func splitConfig(t *testing.T, b []byte) (config, vop []byte) {
	t.Helper()
	i := bytes.Index(b, []byte{0, 0, 1, 0xB6})
	if i < 0 {
		t.Fatal("no VOP start code")
	}
	return b[:i], b[i:]
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeKeyframeMatchesFFmpeg(t *testing.T) {
	for _, tc := range fixtures {
		b := readFixture(t, tc.file)
		config, vop := splitConfig(t, b)
		for _, mode := range []struct {
			name           string
			config, sample []byte
		}{
			{"in-band VOL", nil, b},
			{"esds VOL", config, vop},
			// DivX "packed bitstream" samples hold more than one VOP; the
			// first coded VOP is the keyframe.
			{"packed", config, append(append([]byte{}, vop...), vop...)},
		} {
			pic, err := DecodeKeyframe(mode.config, mode.sample)
			if err != nil {
				t.Fatalf("%s (%s): %v", tc.file, mode.name, err)
			}
			if pic.Width != tc.w || pic.Height != tc.h || pic.QuantType != tc.quant || pic.Interlaced != tc.interlaced {
				t.Fatalf("%s: got %dx%d quant=%s interlaced=%v", tc.file, pic.Width, pic.Height, pic.QuantType, pic.Interlaced)
			}
			sum := sha256.Sum256(i420(pic))
			if got := hex.EncodeToString(sum[:]); got != tc.sha {
				t.Errorf("%s (%s, %s): sha256 %s, want %s", tc.file, tc.opts, mode.name, got, tc.sha)
			}
		}
	}
}

func TestDecodeKeyframeXviDUserData(t *testing.T) {
	pic, err := DecodeKeyframe(nil, readFixture(t, "xvid.m4v"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pic.Encoder) == 0 || pic.Encoder[0] != "XviD" {
		t.Fatalf("Encoder = %v, want [XviD]", pic.Encoder)
	}
}

// TestPooledStateReset decodes different sizes back to back and checks the
// pooled decoder carries nothing between pictures.
func TestPooledStateReset(t *testing.T) {
	for round := 0; round < 3; round++ {
		for _, tc := range fixtures {
			pic, err := DecodeKeyframe(nil, readFixture(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(i420(pic))
			if hex.EncodeToString(sum[:]) != tc.sha {
				t.Fatalf("round %d %s: hash changed", round, tc.file)
			}
		}
	}
}

func TestUnsupportedReasons(t *testing.T) {
	b := readFixture(t, "dp.m4v") // -c:v mpeg4 -data_partitioning 1
	_, err := DecodeKeyframe(nil, b)
	if !errors.Is(err, errs.UnsupportedProfile) || !strings.Contains(err.Error(), "data partitioning") {
		t.Fatalf("data partitioning: %v", err)
	}
	h := readFixture(t, "h263_q4.m4v")
	config, vop := splitConfig(t, h)
	if _, err := DecodeKeyframe(nil, vop); err == nil || !strings.Contains(err.Error(), "VOL") {
		t.Fatalf("missing VOL: %v", err)
	}
	pvop := append([]byte{}, vop...)
	pvop[4] = pvop[4]&0x3F | 0x40 // vop_coding_type = P
	if _, err := DecodeKeyframe(config, pvop); err == nil || !strings.Contains(err.Error(), "P-VOP") {
		t.Fatalf("P-VOP: %v", err)
	}
	if _, err := DecodeKeyframe(nil, []byte{0, 0, 0x80, 2, 0}); err == nil || !strings.Contains(err.Error(), "short video header") {
		t.Fatalf("short header: %v", err)
	}
	if _, err := DecodeKeyframe(config, []byte{1, 2, 3}); err == nil || !errors.Is(err, errs.Corrupt) {
		t.Fatalf("garbage: %v", err)
	}
}

// TestCorruptInputs truncates and flips bytes in every fixture. Decoding
// must return a picture or an error, never panic.
func TestCorruptInputs(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, tc := range fixtures {
		b := readFixture(t, tc.file)
		for i := 0; i < 200; i++ {
			c := append([]byte{}, b[:1+rng.Intn(len(b))]...)
			for j := rng.Intn(16); j > 0; j-- {
				c[rng.Intn(len(c))] = byte(rng.Intn(256))
			}
			_, _ = DecodeKeyframe(nil, c)
		}
	}
}

func FuzzDecodeKeyframe(f *testing.F) {
	for _, tc := range fixtures {
		b, err := os.ReadFile("testdata/" + tc.file)
		if err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = DecodeKeyframe(nil, b)
	})
}

func BenchmarkDecodeKeyframe(b *testing.B) {
	data, err := os.ReadFile("testdata/h263_q4.m4v")
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		if _, err := DecodeKeyframe(nil, data); err != nil {
			b.Fatal(err)
		}
	}
}
