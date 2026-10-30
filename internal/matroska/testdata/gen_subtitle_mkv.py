#!/usr/bin/env python3
"""Writes the hand-built Matroska subtitle fixtures (sub_hand_*.mkv).

ffmpeg's muxer cannot produce these shapes (S_TEXT/ASCII, mkvmerge-style
S_TEXT/WEBVTT, blocks without BlockDuration, DefaultDuration, lacing,
ContentEncoding compression, a non-default TimecodeScale, bitmap codec ids),
so a minimal EBML writer builds them. Run from this directory; the WebVTT
goldens are then captured from ffmpeg by testdata/gen-subtitles.sh.
"""
import struct
import zlib


def vint_size(n):
    for length in range(1, 9):
        if n < (1 << (7 * length)) - 1:
            return bytes([((1 << (8 - length)) | (n >> (8 * (length - 1))))]) + \
                n.to_bytes(length, "big")[1:] if length > 1 else bytes([0x80 | n])
    raise ValueError(n)


def el(id_hex, payload):
    return bytes.fromhex(id_hex) + vint_size(len(payload)) + payload


def uint(id_hex, v):
    return el(id_hex, v.to_bytes(max(1, (v.bit_length() + 7) // 8), "big"))


def string(id_hex, s):
    return el(id_hex, s.encode())


def block(track, rel, payload, lacing=0):
    flags = lacing << 1
    return bytes([0x80 | track]) + struct.pack(">hB", rel, flags) + payload


def mkv(codec, blocks, scale=1000000, default_duration=None, encoding=None, private=None):
    """blocks: list of (cluster_tc, rel, payload, duration or None, lacing)."""
    head = el("1A45DFA3", uint("4286", 1) + uint("42F7", 1) + uint("42F2", 4) +
              uint("42F3", 8) + string("4282", "matroska") + uint("4287", 4) + uint("4285", 2))
    info = el("1549A966", uint("2AD7B1", scale) + string("4D80", "gen") + string("5741", "gen"))
    te = uint("D7", 1) + uint("73C5", 1) + uint("83", 17) + string("86", codec) + string("22B59C", "und")
    if default_duration:
        te += uint("23E383", default_duration)
    if private:
        te += el("63A2", private)
    if encoding:
        algo, settings = encoding
        comp = uint("4254", algo) + (el("4255", settings) if settings else b"")
        te += el("6D80", el("6240", uint("5031", 0) + uint("5032", 1) + uint("5033", 0) + el("5034", comp)))
    tracks = el("1654AE6B", el("AE", te))
    clusters = b""
    by_tc = {}
    for tc, rel, payload, dur, lacing in blocks:
        by_tc.setdefault(tc, []).append((rel, payload, dur, lacing))
    for tc in sorted(by_tc):
        body = uint("E7", tc)
        for rel, payload, dur, lacing in by_tc[tc]:
            b = block(1, rel, payload, lacing)
            if dur is None:
                body += el("A3", b)  # SimpleBlock, no BlockDuration
            else:
                body += el("A0", el("A1", b) + uint("9B", dur))
        clusters += el("1F43B675", body)
    seg = info + tracks + clusters
    return head + el("18538067", seg)


def xiph(frames):
    out = bytes([len(frames) - 1])
    for f in frames[:-1]:
        n = len(f)
        out += b"\xff" * (n // 255) + bytes([n % 255])
    return out + b"".join(frames)


fixtures = {
    # Plain text: ffmpeg's "text" decoder, no markup interpretation.
    "sub_hand_ascii.mkv": mkv("S_TEXT/ASCII", [
        (0, 1000, b"Plain <i>not markup</i> {\\b1}", 1500, 0),
        (0, 3000, b"Line one\nLine two\r\n  tabs\tand  spaces", 1000, 0),
    ]),
    # SubRip markup in SimpleBlocks (no BlockDuration, no DefaultDuration):
    # ffmpeg writes zero-length cues.
    "sub_hand_noduration.mkv": mkv("S_TEXT/UTF8", [
        (0, 1000, b"<b>No duration</b>", None, 0),
        (0, 2000, b"Second", None, 0),
        (5000, 0, b"Zero BlockDuration", 0, 0),
    ]),
    # DefaultDuration (ns) supplies the length; laced frames split it.
    "sub_hand_defaultdur.mkv": mkv("S_TEXT/UTF8", [
        (0, 1000, b"Default duration", None, 0),
        (0, 3000, b"Explicit wins", 500, 0),
        (0, 5000, xiph([b"Lace A\x00", b"Lace B\x00"]), 1000, 1),
    ], default_duration=2_000_000_000),
    # D_WEBVTT with identifier/settings lines, a malformed frame and an empty
    # payload. ffmpeg drops a malformed frame by failing the whole cluster and
    # resyncing at the next one, so each bad frame sits alone in its cluster.
    "sub_hand_dwebvtt.mkv": mkv("D_WEBVTT/SUBTITLES", [
        (0, 1000, b"cue1\nalign:start position:0%\n<v Bob>Hi</v> &amp; {x}\n\n", 1000, 0),
        (0, 3000, b"\r\nline:0\r\nCRLF header", 1000, 0),
        (5000, 0, b"no header lines", 1000, 0),
        (7000, 0, b"\n\n", 1000, 0),
        (9000, 0, b"\n\n<c.red>last</c>", 1000, 0),
    ]),
    # mkvmerge-style S_TEXT/WEBVTT: frames are only the cue text.
    "sub_hand_stextwebvtt.mkv": mkv("S_TEXT/WEBVTT", [
        (0, 1000, b"\n\nWith header lines", 1000, 0),
        (0, 3000, b"Text only <i>cue</i>", 1000, 0),
    ]),
    # zlib ContentCompression.
    "sub_hand_zlib.mkv": mkv("S_TEXT/UTF8", [
        (0, 1000, zlib.compress(b"<i>zlib</i> compressed"), 1000, 0),
        (0, 3000, zlib.compress(b"Second"), 1000, 0),
    ], encoding=(0, None)),
    # Header stripping: ContentCompSettings is prepended to every frame.
    "sub_hand_headerstrip.mkv": mkv("S_TEXT/UTF8", [
        (0, 1000, b"stripped header", 1000, 0),
        (0, 3000, b"again", 1000, 0),
    ], encoding=(3, b"Prefix: ")),
    # TimecodeScale 100 us with odd tick values exercises ms rounding;
    # timestamps past one hour use the HH: form.
    "sub_hand_scale.mkv": mkv("S_TEXT/UTF8", [
        (0, 15, b"1.5 ms start", 25, 0),
        (36000000, 12345, b"Over an hour", 9999, 0),
    ], scale=100000),
    # Bitmap and unsupported codecs: go-media must refuse them.
    "sub_hand_pgs.mkv": mkv("S_HDMV/PGS", [(0, 0, b"\x00", 1000, 0)]),
    "sub_hand_vobsub.mkv": mkv("S_VOBSUB", [(0, 0, b"\x00", 1000, 0)], private=b"size: 720x480\n"),
    "sub_hand_dvbsub.mkv": mkv("S_DVBSUB", [(0, 0, b"\x00", 1000, 0)], private=b"\x00\x01\x00\x01"),
    "sub_hand_textst.mkv": mkv("S_HDMV/TEXTST", [(0, 0, b"\x00", 1000, 0)]),
    "sub_hand_aribsub.mkv": mkv("S_ARIBSUB", [(0, 0, b"\x00", 1000, 0)], private=b"\x00\x00\x00"),
}

for name, data in fixtures.items():
    with open(name, "wb") as f:
        f.write(data)
