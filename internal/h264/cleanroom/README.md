# Clean-room H.264 intra decoder

ITU-T H.264 decode in pure Go (ffmpeg/openh264 as behavioral references). See **`docs/H264.md`** for gate policy, profiling, and package map.

- **64-bit bit cache** — `bitstream/`
- **Table-driven CAVLC** — `decode/cavlc_lut.go`, canonical tables in `cavlc_canon_decode.go`
- **CABAC** — Main/High (`decode/cabac*.go`)
- **Kernels** — `internal/kernels/h264` (dequant, IDCT, recon, intra pred)
- **Thumbnails:** skip in-loop deblock on fast path; pooled buffers; **PGO** via `default.pgo`

Baseline/Main/High intra paths are exercised by `test/compare` and kernel parity tests.
