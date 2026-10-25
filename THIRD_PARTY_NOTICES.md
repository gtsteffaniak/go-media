# Third-party notices

## H.264 intra decode (`internal/h264`)

In-tree intra-only AVC decoder for MP4/MOV thumbnails. SPS/PPS/slice header parsing uses **`github.com/Eyevinn/mp4ff/avc`** (MIT). Decode logic is intended to live in **`internal/h264/cleanroom`** (ITU-T H.264, validated against ffmpeg). Until clean-room lands, do not treat relocated hi264 packages as third-party-original work — see **`docs/FOLLOWUP_H264_CLEANROOM.md`**.
