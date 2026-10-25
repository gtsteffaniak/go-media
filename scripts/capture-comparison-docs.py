#!/usr/bin/env python3
"""Serve the comparison report and capture docs/comparison/*.png via Playwright.

Run after `make compare`. Installs Node deps on first use (scripts/package.json).
"""
from __future__ import annotations

import http.server
import os
import socket
import subprocess
import sys
import threading
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPORT = ROOT / "test/compare/report_site"
DOCS = ROOT / "docs/comparison"
SCRIPTS = ROOT / "scripts"


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def serve_report(port: int) -> http.server.HTTPServer:
    os.chdir(REPORT)
    handler = http.server.SimpleHTTPRequestHandler
    httpd = http.server.HTTPServer(("127.0.0.1", port), handler)
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    return httpd


def ensure_playwright() -> None:
    node_modules = SCRIPTS / "node_modules"
    if not node_modules.is_dir():
        subprocess.run(["npm", "install", "--no-save"], cwd=SCRIPTS, check=True)
    subprocess.run(["npx", "playwright", "install", "chromium"], cwd=SCRIPTS, check=True)


def main() -> int:
    if not (REPORT / "data/report.json").is_file():
        print("missing report; run: make compare", file=sys.stderr)
        return 1

    ensure_playwright()

    port = free_port()
    httpd = serve_report(port)
    time.sleep(0.3)
    url = f"http://127.0.0.1:{port}/index.html?capture=1"
    try:
        subprocess.run(
            ["node", str(SCRIPTS / "capture-comparison-docs.mjs"), url, str(DOCS)],
            check=True,
            cwd=SCRIPTS,
        )
    finally:
        httpd.shutdown()

    for name in (
        "report-dashboard.png",
        "report-charts.png",
        "mjpeg-thumbnail-side-by-side.png",
        "vp8-ffmpeg-frame.png",
    ):
        path = DOCS / name
        if not path.is_file():
            print(f"missing {path}", file=sys.stderr)
            return 1
        print(f"wrote {path} ({path.stat().st_size} bytes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
