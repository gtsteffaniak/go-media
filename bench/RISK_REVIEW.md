# Concurrency and shared-state risk review: go-media perf branches

Base: `845a22b`. Review is read-only. No branch, worktree, or commit was changed. All tests ran in throwaway `git archive` copies under `/tmp/rr/wt/`.

## Summary

Blockers: 0. Highs: 1 (pre-existing, not introduced by these branches). Mediums: 1.

| Branch | Change | Decision | Severity |
|---|---|---|---|
| perf/type-mp4-h264 | Header-walk moov/trak, hand decode hdlr/mdhd/tkhd (e30f51b, 0a34d1e) | Keep, mitigate (size check before alloc) | Low |
| perf/type-mp4-other | jpegfast DC-only IDCT rows/cols, truncated dequant (da53ddf) | Keep | Low |
| perf/type-matroska-webm | VP9 decoder `sync.Pool` (7d09dd7) | Reject for now, or re-measure and mitigate | Medium |
| perf/type-matroska-webm | Walker-owned header/block buffers (413ac08, 50f7a85) | Keep | Low |
| perf/type-matroska-webm | Subtitle payload buffer reuse (413ac08) | Keep | Low |
| perf/type-matroska-webm | In-place WebVTT render, pre-sized output (dc20836) | Keep, resolve conflict with group-f | Low |
| perf/type-avi-ts | AVI header-only open, `wantIndex=false` (b8cab84) | Keep | Low |
| perf/type-avi-ts | AVI idx1 index preallocation `len(body)/16` (b8cab84) | Mitigate (cap) | Low |
| perf/type-avi-ts | MPEG-TS header-only PID/PUSI checks, reused record and reader buffers (6e0973e) | Keep. Fix pre-existing PAT panic in same PR | High (pre-existing) |
| perf/type-avi-ts | MPEG-PS SCR start-code test order (2d20a51) | Keep | Low |
| perf/type-subtitles | Sized file read `readWholeFile` (d309676) | Keep | Low |
| perf/type-subtitles | `vttChunk` returns input substrings (2781fe4) | Keep | Low |
| perf/type-subtitles | SRT cue buffer reuse (624de8d) | Keep | Low |
| perf/type-subtitles | In-place cue rendering, `scanNums` into caller array (4228c05) | Keep | Low |

## Findings per branch

### perf/type-matroska-webm (group-c)

1. **VP9 decoder pool, `internal/matroska/vp9.go:42-116`** (`vp9DecoderPool`, `getVP9Decoder`, `withVP9SeekPicture`). Medium. Decision: reject for now.
   - Sharing: `sync.Pool.Get` removes the decoder, so two goroutines never hold one decoder. No data race found. The 8-goroutine test passed.
   - What can go wrong: a stale decoder state leaks into a later seek. `govpx` `VP9Decoder.Reset()` (`vp9_decoder.go:1905`) clears frame contexts, `lastHeader`, reference slots, MV buffers and `initialized`. It keeps per-frame scratch (`dqcoeff`, `coefTokenCache`, `planes`, `lfi`, `dq`, `frameY`). Those are rebuilt each frame, so they are probably harmless.
   - Test gap: I removed the success-path `dec.Reset()` in a temp copy. The pooled-vs-fresh tests still passed, including a headless-start case (see Verification). So the tests do not prove that `Reset` is needed or complete.
   - Gain is marginal. The lessons file reports webm -5.1% (at the 5% gate) and mkv -3.1% (below the bar). The commit is kept on the basis of "same code path".
   - Mitigation if kept: interleaved A/B in one window on both VP9 fixtures, keep only if ≥5% on both. Add a test that fails when `Reset` is removed. Add a headless-start case to the pooled-vs-fresh test.
   - Retention: pooled decoders keep frame buffers until two GC cycles pass. Peak extra memory is about 8 decoders under 8 concurrent seeks. This was not measured.
2. **Walker-owned buffers, `internal/matroska/demux.go:336-353`** (`clusterWalker.buf`, `walkBuffers`). Low. Keep. Buffers belong to one walker. `parseBlockHeader` and `readElementHeaderInto` return integers, not slices into the buffer. Lifetime is one call.
3. **Subtitle payload reuse, `internal/matroska/subtitle.go:66,76` and `demux.go:580`** (`readFramesInto`). Low. Keep. Frames alias `payload` until the next `readFramesInto`. The loop converts each frame to a Go string (`cString`, `string(...)`) before the next read. I grepped the changed packages: no `unsafe` use.
4. **WebVTT in-place rendering, `internal/subtitle/webvtt_writer.go`** (group-c version: `vttRenderer.b` is a `*strings.Builder` set in `render`). Low. Keep. Conflicts with group-f's value-type version. Use one implementation and rerun the subtitle tests.
5. **`internal/vp9decode/decode.go:27`** (`FrameDecoder.Reset`). Low. Thin wrapper over govpx `Reset`. Keep if pool is kept.

### perf/type-mp4-h264 (group-a)

6. **Header-walk trak lookup, `internal/isobmff/trak.go:74-125`** (`moovTrakBoxes`). Low. Keep. Local state only. Any irregular layout (two moov boxes, zero-size box, trailing bytes, bad sizes, ftyp over 4 KiB or not a multiple of 4) returns `ok=false`, and the go-mp4 path runs unchanged. The `qt  ` brand detection matches go-mp4's `ftyp` compatible-brand check (`read.go:66-79`), since both read brands from offset 8. Probe on the fixtures is about 60-75% faster per the lessons file.
7. **Hand decode of hdlr/mdhd/tkhd, `internal/isobmff/trak_fast.go:55-62, 86-124`** (`payloadBytes`, `decode*Fast`). Low. Mitigate. `payloadBytes` allocates up to 1 MiB before the exact-size check. Check the expected size first (hdlr ≥24, mdhd ==24, tkhd ==84, version 0) so malformed headers do not allocate. Only version 0 is decoded by hand. Other versions fall back.
   - Corpus test (`trak_fast_test.go`) compares DeepEqual with go-mp4 on fixtures only. No `.mov` fixture is covered, so QuickTime-specific hdlr name handling is unverified (see Open questions).
   - Conflict note: group-b's lessons file says "Rejected for safety: fast trak parser in isobmff". Group-a ships it. This needs an explicit decision.

### perf/type-mp4-other (group-b)

8. **IDCT DC-only shortcuts, `internal/jpegfast/dct.go:362-380, 449-470`**. Low. Keep. No shared state. Exact by reasoning (`dctBox(0,0)==0`). Empirically identical to base on 300k random blocks, including int32 extremes. Measured bundled gain: ThumbnailSmall -8.6%. The lessons file says the IDCT shortcut alone was -4%, so it only clears the bar as a bundle.
9. **Dequant only to `truncZig`, `internal/jpegfast/scan.go:465-480`**. Low. Keep. `truncZig` is per-decoder. The store loop rewrite is index-equivalent (verified by the concurrent decode test and the thumbnail differential).

### perf/type-avi-ts (group-d)

10. **Header-only open, `internal/avi/demux.go:93-130`, `probe.go:11,19`** (`openHeaders`, `wantIndex`). Low. Keep. The code comment says "errors match a full open". That is only partly true: `scanMovi` is skipped, so its error paths no longer run. Those paths look unreachable once `openFile` has passed the `moviOff` checks, but I did not prove it. Mutated AVI inputs (including idx1 stripped and truncated) gave identical results in base and branch.
11. **Index preallocation, `internal/avi/demux.go:349`** (`make([]sampleIndex, 0, len(body)/16)`). Low. Mitigate. `sampleIndex` is 32 bytes, so this allocates 2x the idx1 body up front, before the entries are checked. Cap it, or count valid entries first.
12. **MPEG-TS header-only checks and reused buffers, `internal/mpegts/probe.go:101-102, 160-166, 211-240`** (`headerPIDAndPUSI`, `firstPATPMTPID`, `rec`/`rdr` reuse). High, pre-existing. Keep the change. `Packet.Payload` aliases `rec`. This matches the old `pktBuf` reuse, and `parsePMT` and `peekPESPTS` return codec strings and numbers, so I found no new aliasing.
    - **Pre-existing panic:** `body := p[8 : len(p)-4]` (`probe.go:220, 239`; base `probe.go:199`) panics when the PAT payload is 8 bytes. I reproduced it with a 40-packet TS whose PAT has 8 payload bytes: `runtime error: slice bounds out of range [8:3]`. Reproduced through public `media.Probe` in both base and branch. `pkg/media.Probe` has no `recover`, so a crafted file can crash a caller that does not recover. The branch copies the same bug into `firstPATPMTPID`. Fix: require `len(p) >= 12` (`p` is `payload[1:]`) before slicing, in both `parsePAT` and `firstPATPMTPID`.
13. **MPEG-PS start-code test order, `internal/mpegps/probe.go:40`**. Low. Keep. Same predicate, and the index range is unchanged (`i+14 <= len(data)`). Claimed gain is -24% on Probe.

### perf/type-subtitles (group-f)

14. **Sized file read, `pkg/media/subtitles.go:40-58`** (`readWholeFile`). Low. Keep. Allocates one buffer of `Stat().Size()+1` up front. The peak is the same as `io.ReadAll` for regular files. It is larger only for non-regular files, where size is 0 and growth applies.
15. **`vttChunk` substrings, `internal/subtitle/demux.go:234-268`**. Low. Keep. Returns `s[start:end]` of an immutable Go string. No alias into a caller buffer. `readText` builds a new string on every path. Differential test vs the byte-by-byte reference: 200k random inputs, pass. Retention is bounded by one input string per call.
16. **SRT cue buffer reuse, `internal/subtitle/demux.go:165-225`**. Low. Keep. `buf` is local to one `ParseSRTEvents` call. Each cue is copied with `string(buf)`.
17. **In-place cue rendering and `scanNums` array, `internal/subtitle/webvtt_writer.go`, `demux.go:286-300`**. Low. Keep. `vttRenderer` is per-call. `writeVTTTime` fast path matches the general formatter (200k values plus boundaries). Negative times use the general path. `SubRipToASS` capacity hint `len(in)` only sizes the buffer; the result is copied with `string(dst)`.
18. **Duplicated extension switch, `internal/subtitle/subtitle.go` (`CheckSidecarExt` vs `SidecarBytesToWebVTT`)**. Low. Two lists must stay in sync. Keep, and add a test.

## Verification run and results

Setup: `git archive` of each branch plus base into `/tmp/rr/wt/{base,h264,other,mkv,avi,subs}`, with `bench/.data` copied in. Go 1.27.1, darwin/arm64. The machine was shared with other group workers, so timings are noisy. Correctness results do not depend on timing.

1. **`go test -race` on touched packages** (`internal/isobmff`, `jpegfast`, `matroska`, `subtitle`, `vp9decode`, `avi`, `mpegts`, `mpegps`, `pkg/media`), all six trees, `-count=1`. **Result: all pass, no data races reported.** Details below.
2. **Concurrency harness** (`pkg/media/zz_conc_test.go`): 41 jobs (VP8/VP9/H.264/MJPEG thumbnails at 3 seeks, embedded MKV subtitles, sidecar SRT/VTT/ASS/SSA, header probes on 12 fixtures). Serial reference, then 8 goroutines × 3 rounds in random order across all fixtures. **Result: 0 failures on base and all five branches.** The reference output is identical across all six trees (41/41 lines).
3. **VP9 pool across streams** (`internal/matroska/zz_vp9pool_test.go`, mkv tree only): 5 VP9 streams (2 fixtures and 3 generated with ffmpeg: 320x180 webm, 480x270 mkv, 640x360 mandelbrot webm), 3 target points × 3 output sizes per stream. Two serial passes with shuffled order, then 8 goroutines sharing the pool. Reference uses a fresh decoder, converting before `Close`. **Result: pass, 0 failures across 120 jobs.**
4. **VP9 headless start after a pooled decode**: 24 seeks, frame list with the leading keyframe removed. Pooled and fresh results agree (both fail, no picture). **Pass, 0 mismatches.** Mutation check: removing the success-path `Reset()` in a temp copy **still passes**. This is the test gap in finding 1.
5. **JPEG IDCT differential** (`internal/jpegfast/zz_idct_ref_test.go`, `zz_jpeg_test.go`): base `idct`/`idctRows`/`idctCols` copied and renamed. 300k random blocks in five sparsity modes, including int32 extremes. **Pass, 0 mismatches.** Concurrent decode of 12 generated JPEGs (3 decode modes incl. `DecodeTrunc`), 8 goroutines × 4 rounds. **Pass, 0 mismatches.**
6. **Malformed-input differential** (`pkg/media/zz_diff_test.go`): 16 fixtures × 200 mutations each. Mutations: truncation, byte flips in head and tail, zeroed windows, and structural box-size/version corruption for `moov`, `trak`, `mdia`, `hdlr`, `mdhd`, `tkhd`, `stsd`, `LIST`, `hdrl`, `strh`, `idx1`, `PAT`, `PMT`, and others. Outputs from `Duration`, `ListSubtitleTracks`, `Probe`, sidecar conversion, and (for selected fixtures) `Thumbnail` and embedded subtitles. **10,550 result lines per tree. Result: 0 differing lines in all five branches versus base.** Panics are recovered and recorded; none appeared in base.
7. **Pre-existing panic repro**: `media.Probe` on a 7.4 KB crafted TS (40 packets, PAT payload 8 bytes). Base and `perf/type-avi-ts` both panic: `slice bounds out of range [8:3]`.

Harness files (throwaway, not in the repo): `/tmp/rr/zz_*_test.go`.

### Race results

| Tree | isobmff | jpegfast | matroska | subtitle | vp9decode | avi | mpegts | pkg/media |
|---|---|---|---|---|---|---|---|---|
| base 845a22b | ok | ok | ok | ok | ok | ok | ok | ok |
| perf/type-mp4-h264 | ok | no tests | ok | ok | ok | ok | ok | ok |
| perf/type-mp4-other | ok | ok | ok | ok | ok | ok | ok | ok |
| perf/type-matroska-webm | ok | ok | ok (includes VP9 pool test) | ok | ok | ok | ok | ok |
| perf/type-avi-ts | ok | ok | ok | ok | ok | ok | ok | ok |
| perf/type-subtitles | ok | ok | ok | ok | ok | ok | ok | ok |

`internal/mpegps` has no test files. `ZZDiffMalformed` was skipped in race runs because it is a long differential run. It was run without `-race` (item 6). Wall time under race was long on this shared machine (the matroska package took about 14 minutes on the matroska tree because of the VP9 tests).

## Recommended merge order with conditions

1. **perf/type-mp4-other** (group-b). Isolated to `jpegfast`. Condition: none beyond normal review. It is exact and verified.
2. **perf/type-avi-ts** (group-d). Condition: fix the PAT length guard (finding 12) and cap the idx1 preallocation (finding 11) in the same PR. Otherwise keep the pre-existing crash.
3. **perf/type-mp4-h264** (group-a). Condition: check the expected box size before allocating in `payloadBytes` (finding 7). Resolve the conflict with group-b's "rejected for safety" note explicitly.
4. **perf/type-subtitles** (group-f). Before group-c, because both change `webvtt_writer.go` and `decoders.go`. Condition: keep this version of `vttRenderer` (value type).
5. **perf/type-matroska-webm** (group-c), without commit 7d09dd7. Take 413ac08, 50f7a85 and dc20836, rebased on group-f's writer. Condition: rerun the subtitle and matroska tests and the mkv-subtitle concurrency job. Keep the VP9 pool out unless the re-measure (below) passes and the Reset tests land.

## Open questions

1. **VP9 pool gain:** does an interleaved A/B on `webm_vp9_640x360_10s` and `mkv_vp9_640x360_10s` clear 5% in one window? If not, drop 7d09dd7.
2. **`govpx` Reset completeness:** is there a stream that starts at a non-keyframe after a pooled decode and produces output that fresh decoding does not? My headless test found none, but it also did not catch a Reset removal. Ask the govpx owner whether the per-frame scratch fields are safe to keep.
3. **Policy conflict on the isobmff fast path:** group-b's lessons reject the same trak parser that group-a ships. Pick one.
4. **Probe panic reachability:** does a server path call `media.Probe` without recovery? If so, the TS fix becomes urgent even though it is pre-existing.
5. **QuickTime hdlr names:** the corpus has no `.mov` fixture. Does go-mp4 decode QuickTime hdlr names (Pascal strings) the same way as the hand decoder's C-string handling? Add a `.mov` fixture to the DeepEqual test.
6. **Pool memory:** how many bytes does one pooled `govpx.VP9Decoder` retain at 1280x720? Not measured. Needed for finding 1.
