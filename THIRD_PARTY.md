# Third-party code

## `third_party/hi264` (fork of github.com/Eyevinn/hi264 v0.10.0)

Vendored and patched for MP4 thumbnail latency:

- **CAVLC:** 64-bit MSB bit cache, `bits.LeadingZeros64` Exp-Golomb / level_prefix, table-driven `coeff_token`, `total_zeros`, and `run_before` VLC (see `internal/cavlc/`).
- **Transform:** zero-block and DC-only 4×4 IDCT shortcuts (`internal/transform/transform.go`).
- **Decode:** skip deblocking when `Decoder.SkipDeblock` is set (thumbnail fast preview).

The module is wired via `replace github.com/Eyevinn/hi264 => ./third_party/hi264` in `go.mod`.
