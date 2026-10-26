package h264

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func TestMP4FirstKeyframeMatchesFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(err)
	}
	st, _ := f.Stat()
	avcc, sample, err := firstSyncSample(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	fr, err := decodeFrame(avcc, sample, false)
	if err != nil {
		t.Fatal(err)
	}
	yPSNR, uPSNR, vPSNR := yuvPlanePSNR(ffmpeg, path, fr)
	t.Logf("YUV PSNR vs ffmpeg yuv: Y=%.2f U=%.2f V=%.2f dB", yPSNR, uPSNR, vPSNR)
	if yPSNR < 45 {
		t.Fatalf("YUV decode PSNR %.1f < 45 dB", yPSNR)
	}
}

func firstSyncSample(r interface{ ReadAt([]byte, int64) (int, error) }, size int64) (*mp4.AVCDecoderConfiguration, []byte, error) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, nil, err
	}
	track := -1
	for i, tr := range info.Tracks {
		if tr.Codec == mp4.CodecAVC1 {
			track = i
			break
		}
	}
	if track < 0 {
		return nil, nil, fmt.Errorf("no avc track")
	}
	rs := ioat.New(r, size)
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil {
		return nil, nil, err
	}
	bips, err := mp4.ExtractBoxesWithPayload(rs, traks[track], []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeAvc1(), mp4.BoxTypeAvcC()},
	})
	if err != nil {
		return nil, nil, err
	}
	var avcc *mp4.AVCDecoderConfiguration
	for _, bip := range bips {
		if c, ok := bip.Payload.(*mp4.AVCDecoderConfiguration); ok {
			avcc = c
			break
		}
	}
	if avcc == nil {
		return nil, nil, fmt.Errorf("missing avcC")
	}
	if len(info.Tracks[track].Samples) == 0 || len(info.Tracks[track].Chunks) == 0 {
		return nil, nil, fmt.Errorf("no samples")
	}
	off := info.Tracks[track].Chunks[0].DataOffset
	sz := info.Tracks[track].Samples[0].Size
	payload, err := ioat.ReadAll(r, size, int64(off), int64(sz))
	return avcc, payload, err
}

func yuvPlanePSNR(ffmpeg, mp4Path string, fr *frame.Frame) (float64, float64, float64) {
	yuvPath := filepath.Join(os.TempDir(), "ff_yuv_plane.yuv")
	cmd := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-skip_frame", "nokey", "-i", mp4Path, "-frames:v", "1", "-pix_fmt", "yuv420p", yuvPath)
	if err := cmd.Run(); err != nil {
		return 0, 0, 0
	}
	raw, err := os.ReadFile(yuvPath)
	if err != nil {
		return 0, 0, 0
	}
	w, h := fr.Width, fr.Height
	ySize := w * h
	uSize := (w / 2) * (h / 2)
	return planePSNR(fr.Y, fr.StrideY, w, h, raw[:ySize], w),
		planePSNR(fr.Cb, fr.StrideC, w/2, h/2, raw[ySize:ySize+uSize], w/2),
		planePSNR(fr.Cr, fr.StrideC, w/2, h/2, raw[ySize+uSize:ySize+2*uSize], w/2)
}

func planePSNR(got []byte, stride, w, h int, want []byte, wantStride int) float64 {
	var mse float64
	n := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := float64(int(got[y*stride+x]) - int(want[y*wantStride+x]))
			mse += d * d
			n++
		}
	}
	if n == 0 || mse == 0 {
		return 100
	}
	mse /= float64(n)
	return 10 * math.Log10(255*255/mse)
}
