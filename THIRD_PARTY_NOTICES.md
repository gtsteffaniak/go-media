# Third-party notices

## H.264 intra decode (`internal/h264`)

In-tree intra-only AVC decoder for MP4/MOV thumbnails. SPS/PPS/slice parsing uses **`github.com/Eyevinn/mp4ff/avc`** (MIT). Decode logic is maintained in **`internal/h264`** and validated against ffmpeg (PSNR/SSIM). Design references include ITU-T H.264, FFmpeg `h264`, and openh264; no GPL/LGPL source is copied.
