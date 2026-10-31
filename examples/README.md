# go-media examples

Copy-paste recipes for the public API in [`pkg/media`](https://pkg.go.dev/github.com/gtsteffaniak/go-media/pkg/media).

```go
import "github.com/gtsteffaniak/go-media/pkg/media"
```

> [!NOTE]
> Every function takes an [`io.ReaderAt`](https://pkg.go.dev/io#ReaderAt) and the file size, so it works on an `*os.File`, a `bytes.Reader`, or anything that can read at an offset (S3 range reads, `mmap`, and so on). go-media reads only what it needs, never the whole file.

## Contents

| Topic | Examples |
|---|---|
| [Basics](#basics) | Opening files, handling unsupported input |
| [Probe and duration](#probe-and-duration) | Streams and codecs, display size, duration with provenance, caching |
| [Thumbnails](#thumbnails) | Presets, custom size and format, cover art, PNG raster, HTTP handler |
| [Audio metadata](#audio-metadata) | Tags, cover art, one-call inspection, lyrics |
| [Subtitles](#subtitles) | Sidecar to WebVTT, embedded tracks, sibling discovery |
| [Advanced](#advanced) | MP4 keyframe index, SIMD status |

---

## Basics

### Open a file

All the examples below use this helper.

```go
// open returns a ReaderAt and its size. Close the file when done.
func open(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}
```

### Fall back to ffmpeg when go-media says no

go-media returns typed errors for anything outside its fast path. Check for them and hand off to ffmpeg or [go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg).

```go
func thumbnailOrFallback(path string) ([]byte, error) {
	f, size, err := open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	res, err := media.ThumbnailLarge(f, size, 10)
	switch {
	case err == nil:
		return res.Data, nil
	case errors.Is(err, media.ErrUnsupported), errors.Is(err, media.ErrUnsupportedProfile):
		// Not on go-media's fast path: let ffmpeg do it.
		return exec.Command("ffmpeg", "-v", "error", "-i", path,
			"-frames:v", "1", "-f", "image2", "-c:v", "mjpeg", "-").Output()
	default:
		return nil, err // ErrCorrupt, ErrNotMedia, or an I/O error
	}
}
```

| Error | Meaning | What to do |
|---|---|---|
| `ErrUnsupported` | The format or operation isn't implemented | Fall back to ffmpeg |
| `ErrUnsupportedProfile` | The format is known, but this codec profile or tool isn't | Fall back to ffmpeg |
| `ErrCorrupt` | The file is truncated or malformed | Usually report it; ffmpeg may still recover something |
| `ErrNotMedia` | Not a recognised media file | Skip it |

### See why something is unsupported

```go
func explain(err error) {
	var ue *media.UnsupportedError
	if errors.As(err, &ue) {
		log.Printf("unsupported %s: %s", ue.Op, ue.Reason)
	}
}
```

> [!TIP]
> `ErrUnsupported` and `ErrUnsupportedProfile` are both designed for `errors.Is`. Log the error text: it names the codec or tool that was rejected.

---

## Probe and duration

### List streams and codecs

```go
func describe(path string) error {
	f, size, err := open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := media.Probe(f, size)
	if err != nil {
		return err
	}
	fmt.Printf("%s (%s), %s\n", info.FormatName, info.Format, info.Duration)
	for _, s := range info.Streams {
		switch s.Type {
		case media.StreamVideo:
			fmt.Printf("  #%d video %s %dx%d @ %.3f fps\n", s.Index, s.Codec, s.Width, s.Height, s.FrameRate)
		case media.StreamAudio:
			fmt.Printf("  #%d audio %s %d Hz, %d ch\n", s.Index, s.Codec, s.SampleRate, s.Channels)
		case media.StreamSubtitle:
			fmt.Printf("  #%d subtitle %s [%s]\n", s.Index, s.Codec, s.Language)
		}
	}
	fmt.Printf("  artwork=%v lyrics=%v subtitles=%d\n", info.HasArtwork, info.HasLyrics, len(info.SubtitleTracks))
	return nil
}
```

### Get the display size of a rotated video

`Width` and `Height` are the coded size. Phones often record sideways and set a rotation instead.

```go
func displaySize(s media.Stream) (w, h int) {
	if s.Rotation == 90 || s.Rotation == -90 || s.Rotation == 270 || s.Rotation == -270 {
		return s.Height, s.Width
	}
	return s.Width, s.Height
}
```

### Read the duration and where it came from

```go
func duration(path string) (time.Duration, error) {
	f, size, err := open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	d, err := media.Duration(f, size)
	if err != nil {
		return 0, err
	}
	if d.Source == media.SourceEstimate {
		log.Printf("%s: duration is an estimate", path)
	}
	return d.Duration, nil
}
```

| `Source` | How the value was found |
|---|---|
| `SourceHeader` | Read from a container header such as `mvhd` or FLAC `STREAMINFO` |
| `SourceScan` | Found by scanning the file, for example MP3 VBR frames |
| `SourceEstimate` | Estimated, for example MP3 CBR without a VBR header |

### Cache results by path, size and modification time

go-media is fast, but a directory listing can still hit thousands of files. Key a cache on what changes when the file changes.

```go
type cacheKey struct {
	path  string
	size  int64
	mtime int64
}

var durations sync.Map // cacheKey -> media.DurationResult

func cachedDuration(path string) (media.DurationResult, error) {
	st, err := os.Stat(path)
	if err != nil {
		return media.DurationResult{}, err
	}
	key := cacheKey{path, st.Size(), st.ModTime().UnixNano()}
	if v, ok := durations.Load(key); ok {
		return v.(media.DurationResult), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return media.DurationResult{}, err
	}
	defer f.Close()
	d, err := media.Duration(f, st.Size())
	if err != nil {
		return d, err
	}
	durations.Store(key, d)
	return d, nil
}
```

### Probe from memory

```go
func probeBytes(data []byte) (*media.ProbeInfo, error) {
	return media.Probe(bytes.NewReader(data), int64(len(data)))
}
```

---

## Thumbnails

### Use a preset

```go
func presets(path string) error {
	f, size, err := open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	small, err := media.ThumbnailSmall(f, size, 10) // fits 256x256, for listings
	if err != nil {
		return err
	}
	large, err := media.ThumbnailLarge(f, size, 10) // fits 1280x720, for previews
	if err != nil {
		return err
	}
	if err := os.WriteFile("small.jpg", small.Data, 0o644); err != nil {
		return err
	}
	return os.WriteFile("large.jpg", large.Data, 0o644)
}
```

| Preset | Bounding box | JPEG quality | Decode | Use it for |
|---|---|---|---|---|
| `ThumbnailPresetSmall` | 256 × 256 | 76 | Fast preview | Grid and list views |
| `ThumbnailPresetLarge` | 1280 × 720 | 90 | Full quality | Preview panes and lightboxes |

> [!NOTE]
> The third argument is how far into the video to look, as a percentage (0 to 100). go-media seeks to the nearest keyframe at or before that point, like `ffmpeg -ss`.

### Choose size, format, quality and scaler

```go
func custom(f io.ReaderAt, size int64) (media.ThumbnailResult, error) {
	return media.Thumbnail(f, size, media.ThumbnailOptions{
		MaxWidth:    640,
		MaxHeight:   360,
		SeekPercent: 25,
		Format:      "webp-lossless", // "jpeg" (default), "png", or "webp-lossless"
		Filter:      media.ThumbnailFilterBalanced,
	})
}
```

| Option | Values |
|---|---|
| `Format` | `"jpeg"`, `"png"`, `"webp-lossless"` |
| `Quality` | JPEG quality, 1 to 100 |
| `Filter` | `ThumbnailFilterFast`, `ThumbnailFilterArea`, `ThumbnailFilterBalanced`, `ThumbnailFilterHigh` |
| `MaxWidth`, `MaxHeight` | Bounding box; aspect ratio is preserved |
| `PreferEmbedded` | Use cover art before decoding video |

### Start from a preset and tweak it

```go
func previewPNG(f io.ReaderAt, size int64) (media.ThumbnailResult, error) {
	opt := media.OptionsForPreset(media.ThumbnailPresetLarge, 50)
	opt.Format = "png"
	return media.Thumbnail(f, size, opt)
}
```

Or apply a preset onto options you already have:

```go
func withPreset(opt media.ThumbnailOptions) media.ThumbnailOptions {
	media.ApplyPreset(&opt, media.ThumbnailPresetSmall)
	return opt
}
```

### Prefer cover art for music and videos

```go
func albumOrFrame(f io.ReaderAt, size int64) (media.ThumbnailResult, error) {
	res, err := media.Thumbnail(f, size, media.ThumbnailOptions{
		MaxWidth:       512,
		MaxHeight:      512,
		SeekPercent:    10,
		PreferEmbedded: true,
	})
	if err == nil && res.Source == media.SourceEmbedded {
		log.Println("used embedded cover art")
	}
	return res, err
}
```

### Get a PNG raster

```go
func raster(f io.ReaderAt, size int64) ([]byte, error) {
	return media.ThumbnailRasterPNG(f, size, media.OptionsForPreset(media.ThumbnailPresetLarge, 10))
}
```

### Serve thumbnails over HTTP

```go
func thumbHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(root, filepath.Clean("/"+r.URL.Query().Get("path")))
		f, size, err := open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		res, err := media.ThumbnailSmall(f, size, 10)
		if errors.Is(err, media.ErrUnsupported) || errors.Is(err, media.ErrUnsupportedProfile) {
			http.Error(w, "no fast thumbnail", http.StatusNotImplemented) // or queue an ffmpeg job
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", res.MIME)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(res.Data)
	}
}
```

> [!TIP]
> Thumbnails also work on HEIC, HEIF and AVIF photos and on camera RAW files (CR2, CR3, NEF, DNG and others): go-media extracts the embedded JPEG preview, which is far faster than decoding the image.

---

## Audio metadata

### Read tags

```go
func tags(f io.ReaderAt, size int64) error {
	t, err := media.ReadTags(f, size)
	if err != nil {
		return err
	}
	fmt.Printf("%s - %s (%s, %d) track %d [%s]\n", t.Artist, t.Title, t.Album, t.Year, t.Track, t.Genre)
	for k, v := range t.Raw { // every tag, unnormalised
		fmt.Printf("  %s = %s\n", k, v)
	}
	return nil
}
```

### Save the cover art

```go
func saveCover(f io.ReaderAt, size int64, out string) error {
	if !media.HasArtwork(f, size) {
		return nil
	}
	data, mime, _, err := media.EmbeddedArtwork(f, size)
	if err != nil {
		return err
	}
	if mime == "image/png" {
		out += ".png"
	} else {
		out += ".jpg"
	}
	return os.WriteFile(out, data, 0o644)
}
```

### Get everything in one call

`InspectAudio` reads tags, lyrics presence and (optionally) artwork in a single pass.

```go
func inspect(f io.ReaderAt, size int64) (media.AudioInfo, error) {
	info, err := media.InspectAudio(f, size, media.AudioOptions{WithArtwork: true})
	if err != nil {
		return info, err
	}
	fmt.Printf("%s by %s, lyrics=%v, art=%d bytes (%s)\n",
		info.Tags.Title, info.Tags.Artist, info.HasEmbeddedLyrics, len(info.Artwork), info.ArtworkMIME)
	return info, nil
}
```

### Lyrics, embedded or from a sidecar

```go
func lyrics(path string) (string, error) {
	// A sidecar next to the file wins: song.lrc, song.elrc, song.srt or song.vtt.
	if text, ext, ok := media.LyricsSidecar(path); ok {
		log.Printf("lyrics from .%s sidecar", ext)
		return text, nil
	}
	f, size, err := open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if !media.HasLyrics(f, size, path) {
		return "", nil
	}
	text, _, err := media.LyricsFromReader(f, size)
	return text, err
}
```

---

## Subtitles

> [!NOTE]
> WebVTT output matches `ffmpeg -i file -map 0:s:0 -f webvtt out.vtt` byte for byte, so it can go straight into an HTML `<track>`. Bitmap formats (PGS, VobSub, DVB) return `ErrUnsupported`.

### Convert a sidecar file to WebVTT

```go
func sidecarToVTT(path string) (string, error) {
	return media.SubtitleToWebVTT(path) // .srt, .vtt, .ass or .ssa
}
```

### List and extract embedded tracks

```go
func embeddedSubs(f io.ReaderAt, size int64) (map[string]string, error) {
	tracks, err := media.ListSubtitleTracks(f, size)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, t := range tracks {
		vtt, err := media.ExtractEmbeddedSubtitle(f, size, t.Index)
		if errors.Is(err, media.ErrUnsupported) {
			continue // bitmap subtitles
		}
		if err != nil {
			return nil, err
		}
		out[fmt.Sprintf("%d-%s", t.Index, t.Language)] = vtt
	}
	return out, nil
}
```

### Find sidecar subtitles next to a video

```go
func sidecars(videoPath string) ([]media.SubtitleTrack, error) {
	entries, err := os.ReadDir(filepath.Dir(videoPath))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// movie.mkv picks up movie.srt, movie.en.srt, movie.fr.vtt, ...
	return media.ExternalSubtitleTracks(videoPath, names), nil
}
```

---

## Advanced

### Index MP4 keyframes

```go
func keyframes(f io.ReaderAt, size int64) error {
	samples, err := media.MP4KeyframeSamples(f, size, 0) // first video track
	if err != nil {
		return err
	}
	for _, s := range samples {
		fmt.Printf("sample %d at offset %d (%d bytes)\n", s.Index, s.Offset, s.Size)
	}
	return nil
}
```

### Check whether SIMD kernels are active

```go
func buildInfo() {
	log.Printf("go-media SIMD kernels active: %v", media.SIMDActive())
}
```

> [!IMPORTANT]
> SIMD is only compiled in with `GOEXPERIMENT=simd` and is experimental. Production builds should use `CGO_ENABLED=0 go build -pgo=default.pgo`. See [Profiling](../docs/PROFILING.md) and [SIMD](../docs/SIMD.md).

---

## Try it from the command line

The repo's demo CLI wraps the same API:

```sh
go run . probe    movie.mkv
go run . duration song.flac
go run . thumb    clip.mp4 --small --seek 25 thumb.jpg
```

See [Supported capabilities](../docs/SUPPORTED_CAPABILITIES.md) for exactly which formats each function handles.
