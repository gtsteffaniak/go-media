#!/usr/bin/env python3
"""Derives hand-patched tx3g fixtures from sub_ff_tx3g.mp4 (made by ffmpeg).

ffmpeg's mp4 muxer always writes a plain edit list and a default style with
no face flags, so these variants patch the file to exercise what ffmpeg's
demuxer and mov_text decoder do with other values:

  sub_hand_tx3g_elst.mp4  leading empty edit of 2 s before the media
  sub_hand_tx3g_bold.mp4  default style flags bold+underline

and builds sub_hand_wvtt.mp4, an ISO/IEC 14496-30 WebVTT track (ffmpeg
cannot read or write wvtt, so its expected output is hand-written).
Run from this directory after testdata/gen-subtitles.sh made the base file.
"""
import struct


def boxes(d, off, end):
    while off + 8 <= end:
        size, typ = struct.unpack(">I4s", d[off:off + 8])
        yield off, size, typ.decode("latin1")
        off += size


def find(d, path, off=0, end=None):
    end = len(d) if end is None else end
    for o, s, t in boxes(d, off, end):
        if t == path[0]:
            if len(path) == 1:
                return o, s
            hdr = 8 + {"stsd": 8}.get(t, 0)
            return find(d, path[1:], o + hdr, o + s)
    raise KeyError(path)


base = bytearray(open("sub_ff_tx3g.mp4", "rb").read())

# Bold (1) + underline (4) default style: tx3g entry body offset 16+22+2.
d = bytearray(base)
o, _ = find(d, ["moov", "trak", "mdia", "minf", "stbl", "stsd", "tx3g"])
d[o + 16 + 24] = 0x05
open("sub_hand_tx3g_bold.mp4", "wb").write(d)

# Edit list: [empty 2000 ms] [media_time 0, rest]. ffmpeg's mov demuxer
# delays the track by 2 s but also reports a 2 s start_time, which the CLI
# subtracts, so the cues keep their stts times. The moov sits after mdat, so
# growing it moves no chunk.
d = bytearray(base)
mo, ms = find(d, ["moov"])
to, ts = find(d, ["moov", "trak"])
eo, es = find(d, ["moov", "trak", "edts"])
lo, ls = find(d, ["moov", "trak", "edts", "elst"])
seg = struct.unpack(">I", d[lo + 16:lo + 20])[0]
elst = struct.pack(">I4sII", 8 + 8 + 24, b"elst", 0, 2) + \
    struct.pack(">IiHH", 2000, -1, 1, 0) + struct.pack(">IiHH", seg, 0, 1, 0)
grow = len(elst) - ls
d[lo:lo + ls] = elst
for off in (eo, to, mo):
    struct.pack_into(">I", d, off, struct.unpack(">I", d[off:off + 4])[0] + grow)
open("sub_hand_tx3g_elst.mp4", "wb").write(d)


# wvtt: ftyp, mdat with samples, moov with one wvtt track (timescale 1000).
def box(t, body):
    return struct.pack(">I4s", 8 + len(body), t.encode()) + body


def full(t, v, f, body):
    return box(t, struct.pack(">I", (v << 24) | f) + body)


def vttc(text, iden=None, sttg=None):
    body = b""
    if iden:
        body += box("iden", iden)
    if sttg:
        body += box("sttg", sttg)
    return box("vttc", body + box("payl", text))


samples = [
    (1000, box("vtte", b"")),
    (1500, vttc(b"<i>Hello</i> &amp; {x}", b"intro", b"align:start position:0%")),
    (500, box("vtte", b"")),
    (1000, vttc(b"Two cues", None, None) + vttc(b"at once\n")),
    (3600000, box("vtte", b"")),
    (1234, vttc(b"Past an hour")),
]
ftyp = box("ftyp", b"isom\x00\x00\x02\x00isomiso6")
mdat_body = b"".join(s for _, s in samples)
mdat_off = len(ftyp) + 8
stts = full("stts", 0, 0, struct.pack(">I", len(samples)) + b"".join(struct.pack(">II", 1, d) for d, _ in samples))
stsc = full("stsc", 0, 0, struct.pack(">IIII", 1, 1, len(samples), 1))
stsz = full("stsz", 0, 0, struct.pack(">II", 0, len(samples)) + b"".join(struct.pack(">I", len(s)) for _, s in samples))
stco = full("stco", 0, 0, struct.pack(">II", 1, mdat_off))
wvtt = box("wvtt", b"\x00" * 6 + b"\x00\x01" + box("vttC", b"WEBVTT"))
stsd = full("stsd", 0, 0, struct.pack(">I", 1) + wvtt)
stbl = box("stbl", stsd + stts + stsc + stsz + stco)
total = sum(d for d, _ in samples)
minf = box("minf", full("sthd", 0, 0, b"") + box("dinf", full("dref", 0, 0, struct.pack(">I", 1) + full("url ", 0, 1, b""))) + stbl)
hdlr = full("hdlr", 0, 0, b"\x00" * 4 + b"text" + b"\x00" * 12 + b"\x00")
mdhd = full("mdhd", 0, 0, struct.pack(">IIIIHH", 0, 0, 1000, total, 0x55c4, 0))
mdia = box("mdia", mdhd + hdlr + minf)
tkhd = full("tkhd", 0, 3, struct.pack(">IIIII", 0, 0, 1, 0, total) + b"\x00" * 8 + b"\x00" * 8 +
            struct.pack(">9I", 0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000) + b"\x00" * 8)
trak = box("trak", tkhd + mdia)
mvhd = full("mvhd", 0, 0, struct.pack(">IIII", 0, 0, 1000, total) + struct.pack(">IH", 0x10000, 0x100) + b"\x00" * 10 +
            struct.pack(">9I", 0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000) + b"\x00" * 24 + struct.pack(">I", 2))
open("sub_hand_wvtt.mp4", "wb").write(ftyp + box("mdat", mdat_body) + box("moov", mvhd + trak))
