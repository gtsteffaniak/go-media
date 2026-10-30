// Package mpeg4 decodes MPEG-4 Part 2 (ISO/IEC 14496-2) Simple and
// Advanced Simple Profile I-VOPs for thumbnails.
//
// It parses VOS, visual object, user data and VOL headers from the esds
// DecoderSpecificInfo and in-band, then decodes one intra VOP: intra DC
// VLC or TCOEF DC, DC/AC prediction with ffmpeg's video-packet edge
// rules, H.263 and MPEG quantisation (with custom intra matrices), zigzag
// and alternate scans, field DCT for interlaced VOLs, resync markers and
// header extension, and an IDCT that matches ffmpeg's simple_idct.
//
// P-, B- and S-VOPs are not decoded. Binary and grayscale shape, sprites
// and GMC, data partitioning, reversible VLC, NEWPRED, reduced resolution,
// scalability, complexity estimation, not_8_bit, Studio Profile and the
// short video header return an errs.UnsupportedProfile error naming the
// feature. Header sizes are bounded by MaxWidth, MaxHeight and MaxPixels.
package mpeg4
