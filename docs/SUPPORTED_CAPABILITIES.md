# Supported capabilities

What **go-media** implements in pure Go (`CGO_ENABLED=0`). This is **not** a ffmpeg compatibility matrix: it documents library behavior. Where validation or a common fallback exists, the **ffmpeg / ffprobe** equivalent is listed for comparison.

Unsupported inputs should return typed errors (`ErrUnsupported`, `ErrUnsupportedProfile`, …) so callers can use [go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg) or ffmpeg.

**Import:** `github.com/gtsteffaniak/go-media/pkg/media`

Regression checks vs **ffmpeg 9.0.2**: `make compare-gate` ([`test/compare/README.md`](../test/compare/README.md)).

## Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Supported |
| 🟡 | Partial or conditional |
| 📋 | Planned |
| 🔬 | Research / not committed |
| ❌ | Out of scope (use ffmpeg) |

---

## Container detection

**API:** `Probe` (format sniff), `Format` constants  

**ffmpeg equivalent:** `ffprobe -show_entries format=format_name`

| Container / extension | Video | Audio | Status | Notes |
|----------------------|:-----:|:-----:|:------:|-------|
| MP4 / `.mp4` | ✅ | ✅ | ✅ | ISO BMFF |
| M4V / `.m4v` | ✅ | — | ✅ | Same family as MP4 |
| M4A / `.m4a` | — | ✅ | ✅ | |
| MOV / `.mov` | ✅ | ✅ | ✅ | |
| 3GP / `.3gp` | ✅ | ✅ | ✅ | |
| MKV / `.mkv` | ✅ | ✅ | ✅ | |
| WebM / `.webm` | ✅ | ✅ | ✅ | |
| WAV / `.wav` | — | ✅ | ✅ | |
| FLAC / `.flac` | — | ✅ | ✅ | |
| OGG Vorbis / `.ogg` | — | ✅ | ✅ | |
| Opus in OGG | — | ✅ | ✅ | |
| MP3 / `.mp3` | — | ✅ | ✅ | |
| M4A AAC (in MP4) | — | ✅ | 🟡 | Container yes; AAC ADTS elementary ❌ |
| AIFF / `.aiff` | — | ✅ | 📋 | |
| AU / `.sun` | — | ✅ | 📋 | |
| MPEG-TS, AVI, FLV, WMV, ASF | ✅ | ✅ | ❌ | |
| HEIC / HEIF | ✅ | — | ❌ | 🔬 embedded thumb research only |

---

## Duration

**API:** `Duration()` — returns `Duration` + `Source` (`header`, `estimate`, `scan`)

**ffmpeg equivalent:** `ffprobe -show_entries format=duration -of default=noprint_wrappers=1:nokey=1`

| Container / extension | Status | Method / notes |
|----------------------|:------:|----------------|
| MP4, M4V, M4A, MOV, 3GP | ✅ | `mvhd` / movie header; fragmented MP4 (`mvhd` duration 0) uses `mehd`, or the `moof`/`trun` sample durations when `mehd` is absent |
| MKV, WebM | 🟡 | Matroska `Duration` when present; missing on some streamed WebM |
| WAV | ✅ | `data` size ÷ byte rate |
| FLAC | ✅ | `STREAMINFO` total samples ÷ sample rate |
| OGG (Vorbis / Opus) | ✅ | Last page granule position |
| MP3 | 🟡 | Xing / Info / VBRI; else CBR frame-header estimate |
| AIFF, AU | 📋 | |
| AAC (ADTS), TS, AVI, FLV, WMV, ASF | ❌ | |

---

## Stream probe

**API:** `Probe()` — streams, duration, tag flags, subtitle track list

**ffmpeg equivalent:** `ffprobe -show_streams -show_format` (JSON compared field-wise in the compare harness)

### Video containers

| Extension | Status | Typical fields |
|-----------|:------:|----------------|
| `.mp4`, `.m4v`, `.mov`, `.3gp` | ✅ | H.264, MJPEG, … via `stsd` / `tkhd` |
| `.mkv`, `.webm` | ✅ | Codecs via `Tracks`, named with ffprobe codec names |
| TS, AVI, FLV, WMV | ❌ | |

### Audio containers

| Extension | Status | Typical fields |
|-----------|:------:|----------------|
| `.wav`, `.flac`, `.ogg`, `.mp3`, `.m4a` | ✅ | Sample rate, channels, codec name |
| `.aiff`, `.au` | 🟡 | |
| AAC ADTS | ❌ | |

MP4 sample entries (AV1, ProRes, PCM, AC-3, E-AC-3, Opus, FLAC) and MKV codec IDs map to ffprobe codec names. `Stream.Rotation` reports the `tkhd` display rotation in degrees; `Width` and `Height` stay the coded size, as in ffprobe.

---

## Tags (metadata)

**API:** `ReadTags()`  

**ffmpeg equivalent:** `ffprobe -show_entries format_tags` (and stream tags where applicable); ID3/Vorbis via same sources as [dhowden/tag](https://github.com/dhowden/tag)

| Tag system | Containers / files | Status | Notes |
|------------|-------------------|:------:|-------|
| ID3v1 / ID3v2 | `.mp3` | ✅ | |
| MP4 `ilst` | `.m4a`, `.mp4`, `.m4v`, `.mov` | ✅ | |
| Vorbis comments | `.flac`, `.ogg` | ✅ | |
| Matroska `Tags` | `.mkv`, `.webm` | 📋 | Not implemented; no MKV/WebM tag fixtures in the compare gate yet |
| WAV INFO | `.wav` | 🟡 | Limited (gate fixture has no tags, so it is skipped) |

---

## Embedded artwork

**API:** `HasArtwork()`, `EmbeddedArtwork()`

**ffmpeg equivalent:** `ffmpeg -i file -map 0:v:0 -c copy cover.jpg` or ffprobe disposition `attached_pic`; for audio, extract APIC/`covr`/FLAC picture

| Source | Containers | Status |
|--------|------------|:------:|
| ID3 `APIC` | `.mp3` | ✅ |
| MP4 `covr` | `.m4a`, `.mp4`, `.mov`, … | ✅ |
| FLAC `PICTURE` | `.flac` | ✅ |
| Vorbis cover (METADATA_BLOCK_PICTURE) | `.ogg` | 🟡 |
| MKV attachments | `.mkv`, `.webm` | 📋 |
| HEIC/AVIF embedded preview | `.heic` | 🔬 |

---

## Lyrics

**API:** `HasLyrics()`, `LyricsFromReader()`, `LyricsSidecar()` (`.lrc` next to media)

**ffmpeg equivalent:** no single standard; often USLT/LYRICS ID3 or custom tags — compare harness uses fixtures + sidecar `.lrc`

| Source | Applies to | Status | Notes |
|--------|------------|:------:|-------|
| Embedded (ID3 USLT, etc.) | Audio containers | 🟡 | Format-dependent. FLAC lyrics match ffprobe in the gate; ffmpeg-written MP3 `USLT` is not surfaced yet |
| Sidecar `.lrc` | Any path with sibling `.lrc` | ✅ | Text matches the file in the gate |

---

## Subtitles

**API:** `ListSubtitleTracks()`, `ExtractEmbeddedSubtitle()` → WebVTT, `SubtitleToWebVTT()`, `ExternalSubtitleTracks()`

**ffmpeg equivalent:** `ffprobe -show_streams` (subtitle streams); extract/convert e.g. `ffmpeg -i file -map 0:s:0 out.vtt`

### Embedded (in video container)

| Container | Text tracks (SRT, ASS, WebVTT in MKV) | MP4 `tx3g` / `wvtt` | Image (PGS, VobSub) |
|-----------|:-------------------------------------:|:-------------------:|:-------------------:|
| `.mkv` | ✅ extract to WebVTT | — | ❌ |
| `.webm` | ✅ | — | ❌ |
| `.mp4`, `.mov` | 🟡 list / partial | ✅ `tx3g` (cue count matches ffmpeg in the gate); 📋 `wvtt` | ❌ |

### Sidecar (not a container)

| File types | API | Status |
|------------|-----|:------:|
| `.srt`, `.vtt`, `.ass` | `SubtitleToWebVTT()` | ✅ parse + convert to WebVTT |
| Sibling discovery | `ExternalSubtitleTracks()` | ✅ Language is taken from a 2- or 3-letter suffix such as `movie.en.srt` |

---

## Video thumbnails

**API:** `Thumbnail()`, `ThumbnailSmall`, `ThumbnailLarge`, `ThumbnailRasterPNG`, `OptionsForPreset`, `ApplyPreset`

**ffmpeg equivalent (compare harness):** `ffmpeg -ss <seek> -i file -frames:v 1 -vf scale=… -q:v … out.jpg` (H.264: keyframe seek + skip loop filter; see [`test/compare/README.md`](../test/compare/README.md))

Seek default ~**10%** into timeline; sync sample (`stss`) or MKV `Cues`. H.264 sync samples may be IDR or open-GOP keyframes: non-IDR ones decode when all slices are intra or a recovery point SEI has `recovery_frame_cnt` 0, otherwise `ErrUnsupportedProfile`. Returns `ErrUnsupported` / `ErrUnsupportedProfile` when decode or policy rejects the stream, including MP4 files with a non-zero display rotation (callers fall back to ffmpeg, which autorotates). Scaled outputs round to the nearest pixel, so their size can differ from the ffmpeg preset by 1 px.

**Colour (matches ffmpeg):** H.264 honors the SPS VUI range and matrix (MP4 `colr` is not read); untagged video is BT.601 limited at all resolutions; JPEG/WebP output is BT.601 for every source (ffmpeg's YUV→YUV capture drops the matrix tag), PNG keeps the tag; VP8 is BT.601 limited. Residuals: ~5 dB PSNR lower on full-range/BT.709 sources (RGB clipping) and JPEG quantizer differences.

### By video codec

| Codec | Containers | Status | Notes |
|-------|------------|:------:|-------|
| **MJPEG** | MP4, MOV, MKV, … | ✅ | Direct frame extract |
| **VP8** | `.webm`, `.mkv` | ✅ | Pure-Go VP8 decode (`gen2brain/vpx`) |
| **H.264** Baseline CAVLC, 8-bit 4:2:0 intra keyframe (IDR or open-GOP) | MP4, MOV, M4V, … | ✅ | Clean-room intra decoder; H.264 in MKV returns `ErrUnsupportedProfile` |
| **H.264 `avc3`** (in-band SPS/PPS) | MP4, MOV | ✅ | SPS/PPS read from the keyframe sample; an SPS in the sample wins over avcC, as in ffmpeg. avcC may be absent or empty |
| **H.264** Main / High CABAC intra | MP4, MOV, M4V, … | ✅ | Not MKV (see above) |
| **H.264** High 10 (9/10-bit 4:2:0) intra | MP4, MOV, M4V, … | ✅ | CAVLC and CABAC, 4x4/8x8 transforms; reconstruction bit-exact with ffmpeg's 10-bit output, then rounded to 8 bits |
| **H.264** interlaced, >10-bit, 4:2:2, 4:4:4, inter prediction | — | ❌ | |
| **MPEG-4 Part 2** SP/ASP I-VOP (DivX, XviD, lavc `mpeg4`) | MP4, MOV (`mp4v`), MKV (`V_MPEG4/ISO/ASP`) | ✅ | H.263 and MPEG quantisation, custom matrices, alternate scan, field DCT, resync markers, DivX packed B-frames; bit-exact with ffmpeg (`simple_idct`). See [`MPEG4.md`](MPEG4.md) |
| **MPEG-4 Part 2** shape, sprites/GMC, data partitioning, RVLC, NEWPRED, reduced resolution, scalability, Studio, short header (H.263) | — | ❌ | `ErrUnsupportedProfile` naming the tool |
| **VP9** | `.webm`, `.mkv` | ❌ | Typed unsupported in gate |
| **AV1**, **HEVC** | — | ❌ | |

### Cover art instead of video frame

**Option:** `ThumbnailOptions.PreferEmbedded`

| Container | Status |
|-----------|:------:|
| MP4, M4A, MOV, MP3, FLAC | ✅ |
| MKV, WebM | 📋 |

---

## Thumbnail output encoding

**API:** `ThumbnailOptions.Format`, `Quality` / `JPEGQuality`

**ffmpeg equivalent:** `-f image2`, `-q:v` (JPEG), PNG/WebP encoders in ffmpeg

| Output | Status | Notes |
|--------|:------:|-------|
| JPEG | ✅ | Primary; stdlib `image/jpeg` |
| PNG | ✅ | stdlib; `ThumbnailRasterPNG` for lossless raster |
| WebP lossless | ✅ | `Format: webp-lossless` |
| WebP lossy | 🔬 | |

---

## H.264 / MP4 helpers (advanced)

**API:** `MP4KeyframeSamples()`, `DecodeH264Keyframe()`, `H264KeyframeYUV420()`, `H264KeyframeTimeSec()`

**ffmpeg equivalent:** `ffprobe` sample index / `ffmpeg -frames:v 1` raw YUV for metrics

| Capability | Containers | Status |
|------------|------------|:------:|
| MP4 sync sample index | `.mp4`, `.mov`, `.m4v`, `.3gp` | ✅ |
| Keyframe (IDR / open-GOP) decode to RGB/YUV | H.264 in MP4/MOV | ✅ | MKV H.264 thumbnail/decode not supported |
| Gated profiles (`ErrUnsupportedProfile`) | — | — | Profiles that fail timing gate in `gatedProfiles` (`internal/h264/sps.go`); none gated today |

---

## Runtime

| API | Purpose |
|-----|---------|
| `SIMDActive()` | Whether experiment `GOEXPERIMENT=simd` kernels are active (not required for production) |

---

## Public API index

| Function | Purpose |
|----------|---------|
| `Probe` | Format, duration, streams, tags flags, subtitle tracks |
| `Duration` | Duration + source |
| `ReadTags` | Title, artist, album, … |
| `InspectAudio` | Tags, embedded lyrics flag, and optional cover bytes from one parse (use this instead of `ReadTags` + `HasLyrics` + `EmbeddedArtwork`) |
| `HasArtwork`, `EmbeddedArtwork` | Cover image bytes |
| `HasLyrics`, `LyricsFromReader`, `LyricsSidecar` | Lyrics |
| `ListSubtitleTracks`, `ExtractEmbeddedSubtitle`, `SubtitleToWebVTT`, `ExternalSubtitleTracks` | Subtitles |
| `Thumbnail`, `ThumbnailSmall`, `ThumbnailLarge`, `ThumbnailRasterPNG` | Thumbnails |
| `ApplyPreset`, `OptionsForPreset` | FileBrowser-style presets |
| `AllThumbnailFilters`, `ThumbnailFilterLabel` | Scaler metadata |
| `MP4KeyframeSamples` | Sync samples |
| `DecodeH264Keyframe`, `H264KeyframeYUV420`, `H264KeyframeTimeSec` | H.264 metrics / debug |
| `SIMDActive` | SIMD experiment status |
| `Unsupported`, `UnsupportedProfile` | Error helpers |

**Errors:** `ErrUnsupported`, `ErrUnsupportedProfile`, `ErrCorrupt`, `ErrNotMedia`.

**Unsupported thumbnail reasons.** Every thumbnail fallback says exactly why, prefixed by the container, while still matching `errors.Is(err, ErrUnsupported)` (or `ErrUnsupportedProfile` for codec feature gaps). The reason is built only from state the index already read, so it adds no I/O. Examples:

| Message (after `media: unsupported thumbnail: `) | Meaning |
|---|---|
| `mp4: no moov box` / `mp4: moov exceeds 64 MiB` / `mp4: "moov" box at offset N: size S invalid (...)` | Container index cannot be built (missing, oversized or truncated moov) |
| `mp4: moov has no trak boxes` / `mp4: no video track` | Nothing to decode |
| `mp4: video codec hvc1 (HEVC) not supported` | Video sample entry fourcc and codec name (ffprobe wording) |
| `mp4 seek: fragmented MP4: no moof boxes after moov` / `no sync sample for track N in M moof boxes` / `duration unknown ...` | fragmented file whose fragments cannot be located or contain no keyframe |
| `mp4: avcC box missing from avc1 sample entry` / `mp4: avcC invalid: ...` | H.264 decoder configuration unusable |
| `mp4: sync sample N unreadable (...)` | Keyframe bytes could not be read |
| `mp4: mp4v track carries MPEG-2 video (go-media has no decoder for this codec)` | `mp4v` entry whose esds objectTypeIndication is not MPEG-4 Part 2 (ffmpeg's `ff_mp4_obj_type` names) |
| `mp4: esds box missing from mp4v sample entry` / `mp4: esds has no DecoderConfigDescriptor` | MPEG-4 decoder configuration unusable |
| `matroska: no decodable V_VP8 frame near seek target (...)` | Matroska VP8/MJPEG seek or decode failed |
| `mp3: no embedded cover art` | Audio file without artwork |
| `<format>: container has no thumbnail decoder in go-media` | Other containers |

MPEG-4 Part 2 gaps use `media: unsupported profile mpeg4 keyframe: ...`, for example `binary shape (video_object_layer_shape=1) not supported`, `sprite coding / GMC (sprite_enable=2) not supported`, `reversible VLC with data partitioning ...`, `sync sample starts with a P-VOP, not an I-VOP ...`, `short video header (H.263 baseline in MPEG-4) not supported`.

H.264 profile gaps use `media: unsupported profile h264: ...` (for example `12-bit luma / 12-bit chroma (profile_idc 244); decoder supports 8, 9 and 10 bits`, `chroma format 2 (need 4:2:0)`).

## Fragmented H.264 MP4

Fragmented MP4 (`frag_keyframe+empty_moov`, `default_base_moof`, CMAF, ismv)
H.264 thumbnails are supported. The seek picks the keyframe at or before the
target decode time, like ffmpeg's fast `-ss`. When `mfra`/`tfra` is present
the lookup is a binary search (cost independent of file length); otherwise
top-level box headers are hopped from the end of `moov` and only `moof`
payloads are read. `tfhd`/`trex` defaults, base-data-offset,
default-base-is-moof, `first_sample_flags` and non-sync / depends-on flags are
honored. Black-frame keyframe skipping used for progressive MP4 is not applied
to fragmented files.

## High bit depth H.264 (High 10)

9- and 10-bit 4:2:0 keyframes decode on a separate scalar `uint16` path
(`internal/h264/cleanroom/decode/hbd*.go`); 8-bit streams keep the byte/SIMD
path unchanged. As in ffmpeg: intra DC default `1<<(BitDepth-1)`, QP range
`[-QpBdOffsetY, 51]` with `QP'Y = QPY + QpBdOffsetY` dequantization (no
int16 saturation), chroma `QP'C` with `QpBdOffsetC`, clipping to
`(1<<BitDepth)-1`, I_PCM samples at the stream bit depth (deblocked with
`QP'Y = 0`), and deblocking `alpha/beta/tc0` scaled by `1<<(BitDepth-8)`.
The decoded planes are rounded to 8 bits (`(v + (1<<(bd-9))) >> (bd-8)`)
before the shared RGB/scaler stage. The rounded planes equal ffmpeg's
`-pix_fmt yuv420p10le` output rounded the same way (0 differing samples on
High 10 CAVLC, CABAC and 8x8-transform assets at several seeks, including
negative QP and chroma QP offsets). Caveat: RGB is produced from the 8-bit
planes; ffmpeg's swscale converts 10-bit YUV to RGB directly with different
chroma interpolation, so full-resolution PNGs differ from ffmpeg's direct
10-bit→rgb24 by ~31–35 dB PSNR on noisy test content while matching
ffmpeg's 8-bit YUV→RGB path at the usual ~44 dB. 4:2:2, 4:4:4 and depths
above 10 bits still return `ErrUnsupportedProfile` with the exact depth.

## MPEG-4 Part 2 (DivX / XviD / mp4v)
Simple and Advanced Simple Profile I-VOPs decode in pure Go
(`internal/mpeg4`, see [`MPEG4.md`](MPEG4.md)) from MP4/MOV `mp4v`
(esds objectTypeIndication 0x20) and Matroska `V_MPEG4/ISO/ASP|SP|AP`.
Output is bit-exact with ffmpeg's `mpeg4` decoder using `simple_idct`
(its default for every stream except XviD-built ones, where ffmpeg picks
the XviD IDCT; those match `-idct simple` bit-exactly and the default to
~75 dB PSNR). Unsupported coding tools return `ErrUnsupportedProfile`
naming the tool, for example
`mpeg4 keyframe: data partitioning (data_partitioned=1) not supported`.
AVI has no demuxer in go-media, so AVI DivX/XviD still returns
`ErrUnsupported`.
