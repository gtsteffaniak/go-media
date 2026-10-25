# Clean-room H.264 intra decoder

Decode-only Main/High intra path for thumbnails. Implementation follows **ITU-T H.264**; **ffmpeg** and **openh264** are behavioral references only.

**Do not** import, vendor, or relocate **Eyevinn/hi264**. The sibling packages (`intradec`, `cabac`, …) are hi264 relocations pending deletion once this tree passes parity tests.

See `docs/FOLLOWUP_H264_CLEANROOM.md`.
