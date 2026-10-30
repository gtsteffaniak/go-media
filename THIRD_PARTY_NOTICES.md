# Third-party notices

See also [THIRD_PARTY.md](THIRD_PARTY.md) for dependency policy.

## mgvs/go-openh264 (BSD-2-Clause)

H.264 slice/macroblock decode in `internal/h264/cleanroom/decode/` is derived from
[mgvs/go-openh264](https://github.com/mgvs/go-openh264).

- **Copyright:** Copyright (c) 2026 Oleksandr Zhabotynskyi
- **License:** BSD-2-Clause — full text in `internal/h264/cleanroom/LICENSE-mgvs-BSD-2`
- **Notice:** `internal/h264/cleanroom/NOTICE-mgvs`

Source files under `internal/h264/cleanroom/decode/` that retain mgvs lineage include a
header reference to this notice.

## OxideAV/oxideav-mpeg4video (MIT)

The ISO/IEC 14496-2 VLC and scan tables in `internal/mpeg4/tables_gen.go`
(Table B-16 intra TCOEF codes, zigzag and alternate scans) were extracted from
[oxideav-mpeg4video](https://github.com/OxideAV/oxideav-mpeg4video) 0.1.8.
Only table data was taken; the decoder itself is written for go-media.

- **Copyright:** Copyright (c) 2026 Karpelès Lab Inc.
- **License:** MIT

> Permission is hereby granted, free of charge, to any person obtaining a copy
> of this software and associated documentation files (the "Software"), to deal
> in the Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
> copies of the Software, and to permit persons to whom the Software is
> furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all
> copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
> FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
> AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
> LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
> SOFTWARE.
