#!/usr/bin/env bash
# Print three-tier thumbnail timings from report_site/data/report.json (ms + speed vs ffmpeg, report-site style).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
JSON="$ROOT/test/compare/report_site/data/report.json"
if [[ ! -f "$JSON" ]]; then
  echo "missing $JSON — run make compare first" >&2
  exit 1
fi
python3 << PY
import json
with open("$JSON") as f:
    r = json.load(f)
rows = r.get("buildModeTimings") or []
modes = ["purego", "purego_pgo", "purego_pgo_simd"]
labels = {"purego": "T1 no PGO", "purego_pgo": "T2 production (+PGO)", "purego_pgo_simd": "T3 +SIMD (diag)"}

def format_speed_compare(pct, digits=1):
    """Match report_site/app.js formatSpeedCompare (timingDeltaPct is a fraction)."""
    if pct is None:
        return "—"
    denom = 1 + pct
    if denom <= 0:
        return "—"
    if pct <= 0:
        factor = 1 / denom
        kind = "faster"
    else:
        factor = denom
        kind = "slower"
    return f"{factor:.{digits}f}× {kind}"

by = {}
for row in rows:
    k = (row["assetId"], row.get("thumbPreset") or row.get("check", ""))
    by.setdefault(k, {})[row["buildMode"]] = row
keys = sorted(by.keys())
print(f"{'asset':<28} {'preset':<8} {'ffmpeg':>8}  " + "  ".join(f"{labels[m]:>24}" for m in modes))
for asset, preset in keys:
    ff = 0.0
    for m in modes:
        if m in by[(asset, preset)]:
            ff = by[(asset, preset)][m].get("ffmpegMs") or ff
    cols = []
    for m in modes:
        row = by[(asset, preset)].get(m)
        if not row:
            cols.append(f"{'—':>24}")
        else:
            ms = row["goMediaWriteMs"]
            speed = format_speed_compare(row.get("timingDeltaPct"))
            cols.append(f"{ms:5.0f}ms {speed}".rjust(24))
    print(f"{asset:<28} {preset:<8} {ff:7.0f}ms  " + "  ".join(cols))
PY
