package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// generateAssets transcodes the seed into a matrix of fixtures under outDir.
func generateAssets(seedPath, outDir string) ([]Asset, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	ffmpeg, ffprobe, err := ffmpegPaths()
	if err != nil {
		return nil, err
	}
	_ = ffprobe

	encoders, err := ffmpegEncoderSet(ffmpeg)
	if err != nil {
		return nil, err
	}
	vorbisEnc, err := pickEncoder(encoders, "libvorbis", "vorbis")
	if err != nil {
		return nil, fmt.Errorf("ogg_vorbis fixture: %w", err)
	}
	mpeg4Enc, err := pickEncoder(encoders, "libxvid", "mpeg4")
	if err != nil {
		return nil, fmt.Errorf("avi_mpeg4 fixture: %w", err)
	}
	h264Enc, err := pickEncoder(encoders, "libx264", "h264", "libopenh264")
	if err != nil {
		return nil, fmt.Errorf("h264 fixtures: %w", err)
	}
	vp8Enc, err := pickEncoder(encoders, "libvpx", "vp8")
	if err != nil {
		return nil, fmt.Errorf("vp8 fixtures: %w", err)
	}
	vp9Enc, err := pickEncoder(encoders, "libvpx-vp9", "vp9")
	if err != nil {
		return nil, fmt.Errorf("webm_vp9 fixture: %w", err)
	}
	hevcEnc, err := pickEncoder(encoders, "libx265", "hevc", "x265")
	if err != nil {
		return nil, fmt.Errorf("hevc fixtures: %w", err)
	}
	av1Enc, err := pickEncoder(encoders, "libsvtav1", "libaom-av1", "av1")
	if err != nil {
		return nil, fmt.Errorf("av1 fixtures: %w", err)
	}
	mpeg2Enc, err := pickEncoder(encoders, "mpeg2video")
	if err != nil {
		return nil, fmt.Errorf("mpegts_mpeg2 fixture: %w", err)
	}

	clip := filepath.Join(outDir, "_seed_clip.mp4")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", seedPath, "-t", "5", "-c", "copy", clip); err != nil {
		return nil, fmt.Errorf("seed clip: %w", err)
	}

	aviMPEG4Args := []string{"-i", clip, "-c:v", mpeg4Enc, "-g", "30", "-q:v", "3", "-an", "-f", "avi"}
	if mpeg4Enc == "mpeg4" {
		aviMPEG4Args = []string{"-i", clip, "-c:v", "mpeg4", "-vtag", "XVID", "-g", "30", "-q:v", "3", "-an", "-f", "avi"}
	}
	// Seed is video-only; audio fixtures use a short synthetic tone.
	audioSrc := "sine=frequency=440:duration=5"

	coverPNG := filepath.Join(outDir, "_cover.png")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=orange:s=320x320:d=0.1", "-frames:v", "1", coverPNG); err != nil {
		return nil, err
	}

	lyrics := filepath.Join(outDir, "_lyrics.lrc")
	if err := os.WriteFile(lyrics, []byte("[00:00.00]Test lyrics line\n[00:02.00]Second line\n"), 0o644); err != nil {
		return nil, err
	}

	av1Vid := av1VideoEncodeArgs(av1Enc)
	mp4AV1Args := append([]string{"-i", clip}, av1Vid...)
	mkvAV1Args := append([]string{"-i", clip}, append(av1Vid, "-f", "matroska")...)
	webmAV1Args := append([]string{"-i", clip}, av1Vid...)
	avifArgs := []string{"-i", coverPNG, "-c:v", av1Enc, "-frames:v", "1"}
	if av1Enc == "libaom-av1" {
		avifArgs = []string{"-i", coverPNG, "-c:v", av1Enc, "-still-picture", "1", "-frames:v", "1"}
	}

	type spec struct {
		id, cat, desc, out string
		args               []string
		sidecars           []string
	}
	specs := []spec{
		{"mp4_h264", "video", "MP4 H.264/AAC", "video/mp4_h264.mp4",
			[]string{"-i", clip, "-c", "copy"}, nil},
		{"mp4_h264_baseline_1080", "video", "MP4 H.264 Baseline 1080p", "video/mp4_h264_baseline_1080.mp4",
			[]string{"-i", clip, "-vf", "scale=1920:1080", "-c:v", h264Enc, "-profile:v", "baseline", "-pix_fmt", "yuv420p", "-g", "30", "-keyint_min", "30", "-crf", "18", "-an"}, nil},
		{"mp4_h264_main_1080", "video", "MP4 H.264 Main CABAC 1080p", "video/mp4_h264_main_1080.mp4",
			[]string{"-i", clip, "-vf", "scale=1920:1080", "-c:v", h264Enc, "-profile:v", "main", "-pix_fmt", "yuv420p", "-g", "30", "-keyint_min", "30", "-crf", "18", "-an"}, nil},
		{"mp4_h264_high_1080", "video", "MP4 H.264 High 8x8 1080p", "video/mp4_h264_high_1080.mp4",
			[]string{"-i", clip, "-vf", "scale=1920:1080", "-c:v", h264Enc, "-profile:v", "high", "-pix_fmt", "yuv420p", "-g", "30", "-keyint_min", "30", "-crf", "18", "-an"}, nil},
		{"mov_h264", "video", "MOV H.264", "video/mov_h264.mov",
			[]string{"-i", clip, "-c", "copy", "-f", "mov"}, nil},
		{"mkv_h264", "video", "MKV H.264", "video/mkv_h264.mkv",
			[]string{"-i", clip, "-c", "copy", "-f", "matroska"}, nil},
		{"mkv_vp8", "video", "MKV VP8", "video/mkv_vp8.mkv",
			[]string{"-i", clip, "-c:v", vp8Enc, "-g", "30", "-keyint_min", "30", "-crf", "10", "-b:v", "1M", "-an", "-f", "matroska"}, nil},
		{"webm_vp8", "video", "WebM VP8", "video/webm_vp8.webm",
			[]string{"-i", clip, "-c:v", vp8Enc, "-g", "30", "-keyint_min", "30", "-crf", "10", "-b:v", "1M", "-an"}, nil},
		{"webm_vp9", "video", "WebM VP9", "video/webm_vp9.webm",
			[]string{"-i", clip, "-c:v", vp9Enc, "-g", "30", "-crf", "30", "-an"}, nil},
		{"mp4_hevc", "video", "MP4 HEVC Main", "video/mp4_hevc.mp4",
			[]string{"-i", clip, "-c:v", hevcEnc, "-tag:v", "hvc1", "-pix_fmt", "yuv420p", "-g", "30", "-keyint_min", "30", "-crf", "28", "-an"}, nil},
		{"mkv_hevc", "video", "MKV HEVC", "video/mkv_hevc.mkv",
			[]string{"-i", clip, "-c:v", hevcEnc, "-pix_fmt", "yuv420p", "-g", "30", "-keyint_min", "30", "-crf", "28", "-an", "-f", "matroska"}, nil},
		{"mp4_av1", "video", "MP4 AV1", "video/mp4_av1.mp4", mp4AV1Args, nil},
		{"mkv_av1", "video", "MKV AV1", "video/mkv_av1.mkv", mkvAV1Args, nil},
		{"webm_av1", "video", "WebM AV1", "video/webm_av1.webm", webmAV1Args, nil},
		{"avif_coded", "video", "AVIF AV1 primary", "image/avif_coded.avif", avifArgs, nil},
		{"avi_mjpeg", "video", "AVI MJPEG", "video/avi_mjpeg.avi",
			[]string{"-i", clip, "-c:v", "mjpeg", "-q:v", "3", "-an", "-f", "avi"}, nil},
		{"avi_mpeg4", "video", "AVI MPEG-4 Part 2", "video/avi_mpeg4.avi",
			aviMPEG4Args, nil},
		{"avi_h264", "video", "AVI H.264", "video/avi_h264.avi",
			[]string{"-i", clip, "-c:v", h264Enc, "-profile:v", "baseline", "-pix_fmt", "yuv420p", "-g", "30", "-an", "-f", "avi"}, nil},
		{"asf_wmv3", "video", "ASF WMV3", "video/asf_wmv3.wmv",
			nil, nil},
		{"mpegts_mpeg2", "video", "MPEG-TS MPEG-2", "video/mpegts_mpeg2.ts",
			[]string{"-i", clip, "-c:v", mpeg2Enc, "-g", "30", "-an", "-f", "mpegts"}, nil},
		{"mpegts_h264", "video", "MPEG-TS H.264", "video/mpegts_h264.ts",
			[]string{"-i", clip, "-c:v", h264Enc, "-profile:v", "baseline", "-pix_fmt", "yuv420p", "-g", "30", "-an", "-f", "mpegts"}, nil},
		{"mjpeg_mp4", "video", "MJPEG in MP4", "video/mjpeg.mp4",
			[]string{"-i", clip, "-vf", "scale=960:-2", "-c:v", "mjpeg", "-q:v", "3", "-an"}, nil},
		{"mp3_cbr", "audio", "MP3 CBR + ID3", "audio/mp3_cbr.mp3",
			[]string{"-f", "lavfi", "-i", audioSrc, "-c:a", "libmp3lame", "-b:a", "128k",
				"-metadata", "title=Bunny", "-metadata", "artist=go-media", "-metadata", "album=Compare",
				"-id3v2_version", "3"}, nil},
		{"mp3_vbr", "audio", "MP3 VBR with Xing header", "audio/mp3_vbr.mp3",
			[]string{"-f", "lavfi", "-i", audioSrc, "-c:a", "libmp3lame", "-q:a", "2", "-metadata", "title=VBR"}, nil},
		{"flac_tags", "audio", "FLAC + Vorbis comments + picture", "audio/flac_tags.flac",
			[]string{"-f", "lavfi", "-i", audioSrc, "-i", coverPNG,
				"-map", "0:a", "-map", "1:v", "-c:a", "flac", "-c:v", "copy",
				"-metadata", "title=FLAC Test", "-metadata", "artist=go-media",
				"-disposition:v:0", "attached_pic"}, nil},
		{"wav_pcm", "audio", "WAV PCM", "audio/wav_pcm.wav",
			[]string{"-f", "lavfi", "-i", audioSrc, "-c:a", "pcm_s16le"}, nil},
		{"ogg_vorbis", "audio", "OGG Vorbis", "audio/ogg_vorbis.ogg",
			[]string{"-f", "lavfi", "-i", audioSrc, "-ac", "2", "-strict", "-2", "-c:a", vorbisEnc, "-metadata", "title=OGG"}, nil},
		{"opus_ogg", "audio", "Opus in Ogg", "audio/opus.ogg",
			[]string{"-f", "lavfi", "-i", audioSrc, "-strict", "-2", "-c:a", "opus"}, nil},
		{"m4a_aac", "audio", "M4A AAC + tags", "audio/m4a_aac.m4a",
			[]string{"-f", "lavfi", "-i", audioSrc, "-c:a", "aac", "-b:a", "128k",
				"-metadata", "title=M4A", "-metadata", "artist=go-media"}, nil},
	}

	var assets []Asset
	for _, s := range specs {
		out := filepath.Join(outDir, s.out)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, err
		}
		args := append([]string{"-y", "-hide_banner", "-loglevel", "error"}, s.args...)
		args = append(args, out)
		if s.id == "asf_wmv3" {
			if err := copyReferenceWMV3(out); err != nil {
				return nil, fmt.Errorf("%s: %w", s.id, err)
			}
		} else if err := runFF(ffmpeg, args...); err != nil {
			return nil, fmt.Errorf("%s: %w", s.id, err)
		}
		assets = append(assets, Asset{ID: s.id, Path: out, Category: s.cat, Description: s.desc, Sidecars: s.sidecars})
	}

	// MKV with embedded SRT + external sidecars on H.264 MP4
	videoBase := filepath.Join(outDir, "video/mp4_h264.mp4")
	subSRT := filepath.Join(outDir, "subtitles/sample.en.srt")
	subVTT := filepath.Join(outDir, "subtitles/sample.en.vtt")
	subASS := filepath.Join(outDir, "subtitles/sample.en.ass")
	if err := writeSampleSubtitles(subSRT, subVTT, subASS); err != nil {
		return nil, err
	}
	mkvSubs := filepath.Join(outDir, "video/mkv_subtitles.mkv")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", clip, "-i", subSRT,
		"-map", "0:v", "-map", "1:s",
		"-c", "copy", "-c:s", "srt",
		mkvSubs); err != nil {
		return nil, err
	}
	assets = append(assets, Asset{
		ID: "mkv_subtitles", Path: mkvSubs, Category: "video",
		Description: "MKV with embedded SRT text track",
	})

	// Sidecar subtitles: stem must match video base name for ExternalSubtitleTracks.
	sampleMP4 := filepath.Join(outDir, "video/sample.mp4")
	if err := copyFile(videoBase, sampleMP4); err != nil {
		return nil, err
	}
	for _, name := range []string{"sample.en.srt", "sample.en.vtt", "sample.en.ass"} {
		src := filepath.Join(outDir, "subtitles", name)
		dst := filepath.Join(outDir, "video", name)
		if err := copyFile(src, dst); err != nil {
			return nil, err
		}
	}
	assets = append(assets, Asset{
		ID: "mp4_sidecar_subs", Path: sampleMP4, Category: "video",
		Description: "MP4 with sidecar SRT/VTT/ASS",
		Sidecars: []string{
			filepath.Join(outDir, "video/sample.en.srt"),
			filepath.Join(outDir, "video/sample.en.vtt"),
			filepath.Join(outDir, "video/sample.en.ass"),
		},
	})

	// MP4 with embedded mov_text (tx3g) + TTML (stpp). Pinned ffmpeg 9.0.2 cannot mux WebVTT into
	// ISOBMFF; wvtt extract is covered by isobmff unit tests.
	mp4Mov := filepath.Join(outDir, "video/_mp4_mov_text.mp4")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", clip, "-i", subSRT,
		"-map", "0:v", "-map", "1", "-c:v", "copy", "-c:s", "mov_text", mp4Mov); err != nil {
		return nil, fmt.Errorf("mp4 mov_text: %w", err)
	}
	mp4TTML := filepath.Join(outDir, "video/_mp4_stpp.mp4")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", clip, "-i", subSRT,
		"-map", "0:v", "-map", "1", "-c:v", "copy", "-c:s", "ttml", mp4TTML); err != nil {
		return nil, fmt.Errorf("mp4 ttml: %w", err)
	}
	mp4Embed := filepath.Join(outDir, "video/mp4_embedded_subs.mp4")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", mp4Mov, "-i", mp4TTML,
		"-map", "0:v", "-map", "0:s", "-map", "1:s", "-c", "copy", mp4Embed); err != nil {
		return nil, fmt.Errorf("mp4_embedded_subs: %w", err)
	}
	assets = append(assets, Asset{
		ID: "mp4_embedded_subs", Path: mp4Embed, Category: "video",
		Description: "MP4 with embedded tx3g + stpp subtitles",
	})
	os.Remove(mp4Mov)
	os.Remove(mp4TTML)

	// Lyrics sidecar for FLAC
	flacPath := filepath.Join(outDir, "audio/flac_tags.flac")
	flacLyrics := strings.TrimSuffix(flacPath, filepath.Ext(flacPath)) + ".lrc"
	if err := copyFile(lyrics, flacLyrics); err != nil {
		return nil, err
	}
	for i := range assets {
		if assets[i].ID == "flac_tags" {
			assets[i].Sidecars = []string{flacLyrics}
		}
	}

	imgAssets, err := appendImageCompareAssets(ffmpeg, clip, outDir)
	os.Remove(clip)
	if err != nil {
		return nil, err
	}
	assets = append(assets, imgAssets...)
	return assets, nil
}

// copyReferenceWMV3 installs the checked-in WMV3 encode of the reference clip.
// ffmpeg has no WMV3 encoder, so this file was produced once from the same
// reference clip with the Windows Media Foundation WMV9 encoder (Main profile,
// 1080p) and is pinned by SHA-256.
func copyReferenceWMV3(dst string) error {
	src := filepath.Join(findRepoRoot("."), "test", "data", seedWMV3FileName)
	if ok, err := fileMatchesSHA256(src, seedWMV3SHA256); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("%s checksum mismatch (expected %s)", src, seedWMV3SHA256)
	}
	return copyFile(src, dst)
}

// writeSampleSubtitles creates minimal subtitle fixtures.
func writeSampleSubtitles(srt, vtt, ass string) error {
	if err := os.MkdirAll(filepath.Dir(srt), 0o755); err != nil {
		return err
	}
	srtBody := "1\n00:00:00,000 --> 00:00:02,000\nHello from SRT\n\n2\n00:00:02,000 --> 00:00:04,000\nSecond cue\n"
	vttBody := "WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nHello from VTT\n\n00:00:02.000 --> 00:00:04.000\nSecond cue\n"
	assBody := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,Hello from ASS\n"
	if err := os.WriteFile(srt, []byte(srtBody), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(vtt, []byte(vttBody), 0o644); err != nil {
		return err
	}
	return os.WriteFile(ass, []byte(assBody), 0o644)
}

// ffmpegEncoderSet lists encoder short names from ffmpeg -encoders.
func ffmpegEncoderSet(ffmpeg string) (map[string]bool, error) {
	out, err := exec.Command(ffmpeg, "-hide_banner", "-encoders").Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg -encoders: %w", err)
	}
	set := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		flags := fields[0]
		if len(flags) < 6 {
			continue
		}
		if flags[0] != 'V' && flags[0] != 'A' {
			continue
		}
		set[fields[1]] = true
	}
	return set, nil
}

func pickEncoder(set map[string]bool, names ...string) (string, error) {
	for _, name := range names {
		if set[name] {
			return name, nil
		}
	}
	return "", fmt.Errorf("no encoder among %v", names)
}

// av1VideoEncodeArgs returns fast AV1 encode flags for compare fixtures (gate wall time).
func av1VideoEncodeArgs(enc string) []string {
	out := []string{"-c:v", enc, "-pix_fmt", "yuv420p", "-g", "30", "-keyint_min", "30", "-an"}
	switch enc {
	case "libsvtav1":
		return append([]string{"-preset", "12", "-crf", "40"}, out...)
	case "libaom-av1":
		return append([]string{"-cpu-used", "8", "-crf", "40"}, out...)
	default:
		return append([]string{"-crf", "40"}, out...)
	}
}

// runFF executes ffmpeg with the given arguments.
func runFF(ffmpeg string, args ...string) error {
	cmd := exec.Command(ffmpeg, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// copyFile duplicates a small fixture file.
func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o644)
}
