# MPEG-4 Part 2 keyframe decoder

`internal/mpeg4` is a clean-room, pure-Go decoder for MPEG-4 Part 2
(ISO/IEC 14496-2) I-VOPs. go-media uses it only for thumbnails. It is not a
player: it decodes one intra VOP per call and never runs inter prediction.

## Sources

| Container | Track | Decoder config |
|---|---|---|
| MP4 / MOV / M4V | `mp4v` sample entry, esds objectTypeIndication `0x20` | VOL from the esds DecoderSpecificInfo (tag 5), or in-band in the sample |
| Matroska / WebM | `V_MPEG4/ISO/ASP`, `/SP`, `/AP` | CodecPrivate, or in-band |

The keyframe is the sync sample at or before the seek target (stss or
fragment flags, like H.264). In Matroska it is the keyframe block at or before
the target, else the first one after, with reads capped by `maxCollectBytes`.
An `mp4v` entry carrying another codec, such as MPEG-2 (OTI `0x60`–`0x65`),
MPEG-1 (`0x6A`) or JPEG (`0x6C`), is reported by name and is not decoded.
MJPEG stored as `mp4v` still goes to the MJPEG path.

## Supported coding tools

- Simple and Advanced Simple profiles, any level, rectangular shape, 4:2:0, 8-bit.
- H.263 and MPEG quantisation (`quant_type`), with default or custom intra matrices.
- Intra DC VLC and `intra_dc_vlc_thr`, DC/AC prediction, and alternate vertical scan.
- Interlaced VOLs with `dct_type` field DCT (`top_field_first` is ignored for a still).
- Resync markers / video packets. Prediction is reset at packet boundaries
  exactly as in ffmpeg: `first_slice_line` holds until MB (x₀, y₀+1).
- `quarter_sample`, `newpred` = 0, `reduced_resolution` = 0 VOLs.
- DivX "packed bitstream" samples, where the first I-VOP in the sample is decoded.
- XviD/DivX user data is parsed for build detection.
- Odd frame sizes are cropped from the MB grid.
- Colour: BT.601 limited range by default, with matrix and range taken from
  the VOS `video_signal_type` when it is present.

## Accuracy

Output is bit-exact against `ffmpeg -c:v mpeg4` I-frames for the fixture
corpus. That corpus covers lavc's mpeg4 encoder at q1/q4/q31, mpeg_quant,
`-ps` resync, ildct/ilme, aic, trellis, odd sizes (99x75, 333x187, 350x202),
1080p, B-frame GOPs, frag_keyframe MP4, and MKV remuxes. ffmpeg picks
`simple_idct` for these streams, and so do we.

For streams it detects as XviD-encoded, ffmpeg switches to the XviD IDCT. We
are bit-exact against `ffmpeg -idct simple` and measure ~75 dB average PSNR
(Y ~76 dB) against ffmpeg's default. That difference is invisible.

## Unsupported (returns `ErrUnsupportedProfile` naming the tool)

The binary, grayscale and shape-only VOL shapes; sprites/GMC (`sprite_enable`);
data partitioning and RVLC; NEWPRED; reduced-resolution VOPs; scalability;
Studio profile; `not_8_bit`; the short video header (H.263 baseline); and
a sync sample that is not an I-VOP. Missing VOL headers and truncated or
corrupt bitstreams return `ErrCorrupt`-wrapped errors with the reason.
AVI has no demuxer, so AVI DivX/XviD is not reached.

## Safety and performance

All reads are bounds-checked, and VLC lookups use fixed tables. Frame size is
capped before allocation. Decoder state is pooled and fully reset between
files. `FuzzDecodeKeyframe` and a corrupt-input loop guard against panics.
One 176x144 keyframe decodes in ~146 µs (GOMAXPROCS=1, ~29 MB/s of bitstream).

Table data credit: see `THIRD_PARTY_NOTICES.md` (oxideav-mpeg4video, MIT).
