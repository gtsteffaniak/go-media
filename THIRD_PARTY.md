# Third-party code

## `third_party/hi264` (fork of github.com/Eyevinn/hi264 v0.10.0)

Vendored and patched for MP4 thumbnail latency (Main/High CABAC paths). go-media rejects H.264 Baseline at the API layer for performance; the fork still contains upstream CAVLC code used by hi264 tests.

- **CAVLC (upstream):** 64-bit MSB bit cache and VLC LUTs remain in the fork for upstream parity; not used on supported go-media thumbnail paths.
- **Transform:** zero-block and DC-only 4×4 IDCT shortcuts (`internal/transform/transform.go`).
- **Decode:** skip deblocking when `Decoder.SkipDeblock` is set (thumbnail fast preview).
- **Kernels:** `internal/kernels` — amd64 Plan9 asm for 4×4 luma reconstruct (pred+residual+clip), portable/`purego` fallback, bit-identical tests.

The module is wired via `replace github.com/Eyevinn/hi264 => ./third_party/hi264` in `go.mod`.
