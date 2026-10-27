if (new URLSearchParams(window.location.search).has('capture')) {
  document.body.classList.add('capture-mode');
}

/** Opaque placeholder when go-media (or ffmpeg) thumb file is missing. */
const THUMB_MISSING_SRC =
  'data:image/svg+xml,' +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 180"><rect fill="#0d1117" width="320" height="180"/><rect x="118" y="58" width="84" height="64" rx="4" fill="none" stroke="#3a4555" stroke-width="2"/><path d="M128 98l20-22 18 16 16-14 30 28" fill="none" stroke="#3a4555" stroke-width="2"/><text x="160" y="148" text-anchor="middle" fill="#6b7a90" font-family="system-ui,sans-serif" font-size="13">N/A</text></svg>',
  );

function thumbCompareSrc(path) {
  return path || THUMB_MISSING_SRC;
}

function attachThumbImgFallback(img) {
  img.addEventListener('error', () => {
    if (img.dataset.fallbackApplied) return;
    img.dataset.fallbackApplied = '1';
    img.src = THUMB_MISSING_SRC;
    img.classList.add('compare-missing');
  });
}

function compareSliderMarkup({ assetId, goSrc, ffSrc, goMissing, ffMissing, extraClass = '' }) {
  return `
      <div class="compare-slider ${extraClass}" data-asset="${assetId}" title="Drag to compare · Click to enlarge">
        <img class="compare-base${goMissing ? ' compare-missing' : ''}" src="${goSrc}" alt="go-media ${assetId}" />
        <div class="compare-clip">
          <img class="${ffMissing ? 'compare-missing' : ''}" src="${ffSrc}" alt="ffmpeg ${assetId}" />
        </div>
        <div class="compare-handle" aria-hidden="true"></div>
        <span class="compare-label left">ffmpeg</span>
        <span class="compare-label right">go-media</span>
      </div>`;
}

let lightboxReturnFocus = null;

function closeLightbox() {
  const box = document.getElementById('lightbox');
  if (!box) return;
  box.classList.add('hidden');
  const el = lightboxReturnFocus;
  lightboxReturnFocus = null;
  if (el && typeof el.focus === 'function') {
    try {
      el.focus();
    } catch (_) {
      /* ignore */
    }
  }
}

function openThumbCompareLightbox(card, pct) {
  const slider = card.querySelector('.compare-slider');
  if (!slider) return;
  const goImg = slider.querySelector('.compare-base');
  const ffImg = slider.querySelector('.compare-clip img');
  const body = document.getElementById('lightbox-body');
  const box = document.getElementById('lightbox');
  lightboxReturnFocus = document.activeElement;
  body.innerHTML = compareSliderMarkup({
    assetId: card.dataset.assetId || '',
    goSrc: goImg.src,
    ffSrc: ffImg.src,
    goMissing: goImg.classList.contains('compare-missing'),
    ffMissing: ffImg.classList.contains('compare-missing'),
    extraClass: 'lightbox-enlarged',
  });
  body.querySelectorAll('img').forEach(attachThumbImgFallback);
  const enlarged = initCompareSlider(body.querySelector('.compare-slider'));
  if (enlarged) enlarged.setPct(pct);
  const cap = document.getElementById('lightbox-caption');
  if (cap) cap.textContent = card.querySelector('h3')?.textContent?.trim() || '';
  box.classList.remove('hidden');
  box.querySelector('.lightbox-close')?.focus();
}

let chartInstances = [];
let reportData = null;

function destroyCharts() {
  for (const c of chartInstances) c.destroy();
  chartInstances = [];
}

function metricBar(value, max, kind, format, title) {
  const pct = max > 0 ? Math.min(100, (Math.abs(value) / max) * 100) : 0;
  const label = format(value);
  const tip = title || label;
  return `<div class="metric-bar" title="${tip}">
    <div class="metric-bar-track"><div class="metric-bar-fill ${kind}" style="width:${pct}%"></div></div>
    <span class="metric-bar-value">${label}</span>
  </div>`;
}

function timingComparableRow(r) {
  return r.status === 'supported' || r.status === 'mismatch';
}

function timingGateRow(r) {
  return r.check === 'thumbnail_small' || r.check === 'thumbnail_large';
}

const CHART_GO_COLOR = '#5b9bd5';
const CHART_FF_COLOR = '#6b7a90';

function goMsForCompare(r) {
  if (r.goMediaWriteMs > 0) return r.goMediaWriteMs;
  return r.goMediaMs || 0;
}

function medianValues(values) {
  const v = values.filter((n) => n != null && !Number.isNaN(n) && n > 0).sort((a, b) => a - b);
  if (!v.length) return null;
  const mid = Math.floor(v.length / 2);
  return v.length % 2 ? v[mid] : (v[mid - 1] + v[mid]) / 2;
}

function medianMsByCheck(results, checks, pick) {
  return checks.map((c) => {
    const vals = results
      .filter((r) => r.check === c && timingComparableRow(r) && (r.ffmpegMs || 0) > 0)
      .map(pick);
    return medianValues(vals) ?? 0;
  });
}

function sumMsByAsset(results) {
  const go = {};
  const ff = {};
  for (const r of results) {
    if (!timingComparableRow(r) || (r.ffmpegMs || 0) <= 0) continue;
    const id = r.assetId;
    go[id] = (go[id] || 0) + goMsForCompare(r);
    ff[id] = (ff[id] || 0) + r.ffmpegMs;
  }
  return { go, ff };
}

function pairedMsChartOptions(title, extra = {}) {
  return {
    indexAxis: 'y',
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { position: 'bottom', labels: { color: '#9aa8bc', boxWidth: 12 } },
      title: { display: true, text: title, color: '#e7ecf3', font: { size: 13 } },
      ...extra.plugins,
    },
    scales: {
      x: {
        beginAtZero: true,
        grid: { color: '#2a3544' },
        ticks: { color: '#9aa8bc' },
        title: { display: true, text: 'milliseconds', color: '#9aa8bc' },
      },
      y: { grid: { display: false }, ticks: { color: '#e7ecf3' } },
      ...extra.scales,
    },
    ...extra,
  };
}

/** Mean ms for every row that timed ffmpeg/ffprobe (duration, probe, thumbs, …). */
function ffmpegActionAverages(report) {
  const t = report.summary?.timing || {};
  if (t.ffmpegActionRows > 0 && t.avgFfmpegMs >= 0) {
    return { go: t.avgGoMediaMs, ff: t.avgFfmpegMs, n: t.ffmpegActionRows };
  }
  const rows = (report.results || []).filter(
    (r) => timingComparableRow(r) && (r.ffmpegMs || 0) > 0,
  );
  if (!rows.length) return { go: 0, ff: 0, n: 0 };
  let goSum = 0;
  let ffSum = 0;
  for (const r of rows) {
    goSum += goMsForCompare(r);
    ffSum += r.ffmpegMs;
  }
  return { go: goSum / rows.length, ff: ffSum / rows.length, n: rows.length };
}

function compactChartOptions(extra = {}) {
  return {
    responsive: true,
    maintainAspectRatio: false,
    ...extra,
  };
}

function thumbQualityLine(r) {
  return `JPEG PSNR ${r.psnr?.toFixed(1) ?? '—'} SSIM ${r.ssim?.toFixed(3) ?? '—'} · decode PSNR ${r.psnrDecode?.toFixed(1) ?? '—'} SSIM ${r.ssimDecode?.toFixed(3) ?? '—'}`;
}

function thumbStatsLine(r) {
  if (!timingComparableRow(r) || r.timingDeltaPct == null) return thumbQualityLine(r);
  return `${thumbQualityLine(r)} · go ${r.goMediaMs?.toFixed(1) ?? '—'}ms write ${r.goMediaWriteMs?.toFixed(1) ?? '—'}ms ffmpeg ${r.ffmpegMs?.toFixed(1) ?? '—'}ms ${deltaPctCell(r.timingDeltaPct)}`;
}

function deltaPctCell(pct) {
  if (pct == null || Number.isNaN(pct)) return '—';
  const cls = pct <= 0 ? 'delta-good' : 'delta-bad';
  const sign = pct > 0 ? '+' : '';
  const barW = Math.min(100, Math.abs(pct) * 100);
  return `<span class="${cls} delta-pct">
    <span class="delta-bar"><span class="delta-bar-fill ${cls}" style="width:${barW}%"></span></span>
    ${sign}${(pct * 100).toFixed(1)}%
  </span>`;
}

function qualityBar(value, max, higherIsBetter) {
  if (value == null || Number.isNaN(value)) return '—';
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  const ratio = max > 0 ? value / max : 0;
  const cls = higherIsBetter ? (ratio >= 0.95 ? 'delta-good' : ratio >= 0.85 ? 'delta-warn' : 'delta-bad') : (Math.abs(value) <= 0.05 ? 'delta-good' : Math.abs(value) <= 0.2 ? 'delta-warn' : 'delta-bad');
  return `<span class="${cls} quality-cell">
    <span class="delta-bar"><span class="delta-bar-fill ${cls}" style="width:${pct}%"></span></span>
    ${typeof value === 'number' ? value.toFixed(value < 10 ? 3 : 1) : value}
  </span>`;
}

function statusRank(st) {
  const order = { supported: 5, typed_unsupported: 4, skipped: 3, mismatch: 2, error: 1 };
  return order[st] ?? 0;
}

function makeSortable(table, getRows, renderBody) {
  const thead = table.querySelector('thead');
  let sortKey = 'assetId';
  let sortDir = 1;
  thead.addEventListener('click', (e) => {
    const th = e.target.closest('th[data-sort]');
    if (!th) return;
    const key = th.dataset.sort;
    if (sortKey === key) sortDir *= -1;
    else { sortKey = key; sortDir = 1; }
    thead.querySelectorAll('th').forEach((h) => h.classList.remove('sort-asc', 'sort-desc'));
    th.classList.add(sortDir > 0 ? 'sort-asc' : 'sort-desc');
    renderBody(sortKey, sortDir);
  });
  return () => renderBody(sortKey, sortDir);
}

function initLightbox() {
  const box = document.getElementById('lightbox');
  const body = document.getElementById('lightbox-body');
  const cap = document.getElementById('lightbox-caption');
  document.body.addEventListener('click', (e) => {
    const t = e.target;
    if (t.matches('.thumb-compare img, .thumb-pair img')) {
      lightboxReturnFocus = document.activeElement;
      body.innerHTML = `<img class="lightbox-img" src="${t.src}" alt="" />`;
      const img = body.querySelector('img');
      img.alt = t.alt || '';
      cap.textContent = t.alt || '';
      box.classList.remove('hidden');
    }
  });
  box.querySelector('.lightbox-close').addEventListener('click', closeLightbox);
  box.addEventListener('click', (e) => { if (e.target === box) closeLightbox(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !box.classList.contains('hidden')) closeLightbox();
  });
}

const COMPARE_CLICK_DRAG_PX = 8;

function initCompareSlider(sliderOrRoot, { onZoom } = {}) {
  const slider = sliderOrRoot?.classList?.contains('compare-slider')
    ? sliderOrRoot
    : sliderOrRoot?.querySelector('.compare-slider');
  if (!slider) return null;
  const clip = slider.querySelector('.compare-clip');
  const handle = slider.querySelector('.compare-handle');
  let pct = 50;
  let dragStart = null;
  let dragged = false;
  const apply = (p) => {
    pct = Math.max(2, Math.min(98, p));
    clip.style.clipPath = `inset(0 ${100 - pct}% 0 0)`;
    handle.style.left = `${pct}%`;
  };
  apply(pct);
  const move = (clientX) => {
    const rect = slider.getBoundingClientRect();
    apply(((clientX - rect.left) / rect.width) * 100);
  };
  slider.addEventListener('pointerdown', (e) => {
    if (e.button !== 0) return;
    dragStart = { x: e.clientX, y: e.clientY };
    dragged = false;
    slider.setPointerCapture(e.pointerId);
    move(e.clientX);
  });
  slider.addEventListener('pointermove', (e) => {
    if (!slider.hasPointerCapture(e.pointerId)) return;
    if (dragStart) {
      const dx = e.clientX - dragStart.x;
      const dy = e.clientY - dragStart.y;
      if (Math.hypot(dx, dy) > COMPARE_CLICK_DRAG_PX) dragged = true;
    }
    move(e.clientX);
  });
  slider.addEventListener('pointerup', (e) => {
    if (slider.hasPointerCapture(e.pointerId)) slider.releasePointerCapture(e.pointerId);
    if (!dragged && onZoom) onZoom(pct);
    dragStart = null;
  });
  slider.addEventListener('pointercancel', () => {
    dragStart = null;
  });
  return { getPct: () => pct, setPct: apply };
}

const BUILD_MODE_ORDER = ['purego', 'purego_pgo', 'purego_pgo_simd'];
const BUILD_MODE_LABELS = {
  purego: 'Tier 1 — diagnostic (no PGO)',
  purego_pgo: 'Tier 2 — production (PGO, gated)',
  purego_pgo_simd: 'Tier 3 — experiment (PGO + SIMD)',
};
const BUILD_MODE_AXIS_LABELS = {
  purego: 'Tier 1\nno PGO',
  purego_pgo: 'Tier 2\nproduction',
  purego_pgo_simd: 'Tier 3\n+ SIMD',
};

function medianOf(nums) {
  const v = nums.filter((n) => n != null && !Number.isNaN(n)).sort((a, b) => a - b);
  if (!v.length) return null;
  const mid = Math.floor(v.length / 2);
  return v.length % 2 ? v[mid] : (v[mid - 1] + v[mid]) / 2;
}

function aggregateBuildModeByTier(modeRows) {
  return BUILD_MODE_ORDER.map((mode) => {
    const rows = modeRows.filter((r) => r.buildMode === mode);
    return {
      mode,
      n: rows.length,
      goWriteMed: medianOf(rows.map((r) => r.goMediaWriteMs)),
      ffMed: medianOf(rows.map((r) => r.ffmpegMs)),
      deltaMed: medianOf(rows.map((r) => (r.timingDeltaPct != null ? r.timingDeltaPct * 100 : null))),
    };
  });
}

function formatTierSummaryLine(tiers) {
  const parts = tiers
    .filter((t) => t.n > 0 && t.goWriteMed != null)
    .map((t) => {
      const label = BUILD_MODE_LABELS[t.mode] || t.mode;
      const delta =
        t.deltaMed != null ? `, median ${t.deltaMed >= 0 ? '+' : ''}${t.deltaMed.toFixed(0)}% vs ffmpeg` : '';
      return `${label}: ${t.goWriteMed.toFixed(0)} ms go+write${delta} (n=${t.n})`;
    });
  if (!parts.length) return '';
  let line = parts.join(' · ');
  const t1 = tiers.find((t) => t.mode === 'purego');
  const t2 = tiers.find((t) => t.mode === 'purego_pgo');
  const t3 = tiers.find((t) => t.mode === 'purego_pgo_simd');
  if (t1?.goWriteMed && t2?.goWriteMed) {
    const pgoSave = ((t1.goWriteMed - t2.goWriteMed) / t1.goWriteMed) * 100;
    line += ` · PGO (tier 1→2) saves ~${pgoSave.toFixed(0)}% on median go+write`;
  }
  if (t2?.goWriteMed && t3?.goWriteMed) {
    const simdSave = ((t2.goWriteMed - t3.goWriteMed) / t2.goWriteMed) * 100;
    line += ` · SIMD (tier 2→3) saves ~${simdSave.toFixed(0)}% on median go+write`;
  }
  return line;
}

function buildModeRowKey(r) {
  return `${r.assetId}|${r.thumbPreset}`;
}

function parseBuildModeRowKey(k) {
  const sep = k.indexOf('|');
  return { assetId: k.slice(0, sep), thumbPreset: k.slice(sep + 1) };
}

function buildModeChartLayoutOptions(extra = {}) {
  return {
    responsive: true,
    maintainAspectRatio: false,
    layout: { padding: { top: 12, right: 20, bottom: 8, left: 12 } },
    plugins: {
      legend: {
        position: 'bottom',
        labels: { color: '#e7ecf3', padding: 14, boxWidth: 14 },
      },
      ...extra.plugins,
    },
    ...extra,
  };
}

function setBuildModeChartWrapHeights(labelCount) {
  const h = `${Math.max(320, labelCount * 22 + 120)}px`;
  for (const id of ['chart-build-modes', 'chart-build-modes-ms']) {
    const wrap = document.getElementById(id)?.closest('.chart-wrap');
    if (wrap) wrap.style.height = h;
  }
}

function renderBuildModeTierSummaryCharts(modeRows) {
  const canvasDelta = document.getElementById('chart-tier-median-delta');
  const canvasMs = document.getElementById('chart-tier-median-ms');
  const summaryEl = document.getElementById('buildmode-tier-summary');
  const rowWrap = document.getElementById('buildmode-tier-summary-charts');
  if (!canvasDelta || !canvasMs) return;

  const tiers = aggregateBuildModeByTier(modeRows);
  if (summaryEl) summaryEl.textContent = formatTierSummaryLine(tiers);

  if (!modeRows.length) {
    rowWrap?.classList.add('hidden');
    return;
  }
  rowWrap?.classList.remove('hidden');

  const axisLabels = BUILD_MODE_ORDER.map((m) => BUILD_MODE_AXIS_LABELS[m] || m);
  const deltaVals = tiers.map((t) => t.deltaMed);
  const msVals = tiers.map((t) => t.goWriteMed);
  const msFF = tiers[0]?.ffMed;

  chartInstances.push(new Chart(canvasDelta, {
    type: 'bar',
    data: {
      labels: axisLabels,
      datasets: [{
        label: 'Median Δ% vs ffmpeg',
        data: deltaVals,
        backgroundColor: deltaVals.map((d) => (d != null && d <= 0 ? '#3dd68c' : '#f07178')), // Δ%: green=faster
        borderRadius: 4,
      }],
    },
    options: {
      plugins: {
        legend: { display: false },
        title: {
          display: true,
          text: 'Median thumbnail Δ% vs ffmpeg (all fixtures)',
          color: '#e7ecf3',
          font: { size: 14 },
        },
      },
      scales: {
        y: {
          ticks: { color: '#9aa8bc', callback: (v) => v + '%' },
          grid: { color: '#2a3544' },
          title: { display: true, text: 'vs ffmpeg (negative = faster)', color: '#9aa8bc' },
        },
        x: { ticks: { color: '#e7ecf3' }, grid: { display: false } },
      },
    },
  }));

  chartInstances.push(new Chart(canvasMs, {
    type: 'bar',
    data: {
      labels: axisLabels,
      datasets: [
        {
          label: 'go-media decode + write',
          data: msVals,
          backgroundColor: CHART_GO_COLOR,
          borderRadius: 4,
        },
        ...(msFF != null
          ? [{
              label: 'ffmpeg / ffprobe (reference)',
              data: BUILD_MODE_ORDER.map(() => msFF),
              backgroundColor: CHART_FF_COLOR,
              borderRadius: 4,
            }]
          : []),
      ],
    },
    options: {
      plugins: {
        title: {
          display: true,
          text: 'Median thumbnail time (ms, all fixtures)',
          color: '#e7ecf3',
          font: { size: 14 },
        },
      },
      scales: {
        y: {
          beginAtZero: true,
          ticks: { color: '#9aa8bc' },
          grid: { color: '#2a3544' },
          title: { display: true, text: 'milliseconds', color: '#9aa8bc' },
        },
        x: { ticks: { color: '#e7ecf3' }, grid: { display: false } },
      },
    },
  }));
}

function renderBuildModeCharts(modeRows) {
  const canvas = document.getElementById('chart-build-modes');
  const canvasMs = document.getElementById('chart-build-modes-ms');
  const emptyEl = document.getElementById('buildmode-charts-empty');
  if (!canvas || !canvasMs) return;

  renderBuildModeTierSummaryCharts(modeRows);

  if (!modeRows.length) {
    if (emptyEl) emptyEl.classList.remove('hidden');
    canvas.closest('.chart-box')?.classList.add('hidden');
    canvasMs.closest('.chart-box')?.classList.add('hidden');
    return;
  }

  if (emptyEl) emptyEl.classList.add('hidden');
  canvas.closest('.chart-box')?.classList.remove('hidden');
  canvasMs.closest('.chart-box')?.classList.remove('hidden');

  const keys = [...new Set(modeRows.map(buildModeRowKey))].sort();
  const labels = keys.map((k) => {
    const { assetId, thumbPreset } = parseBuildModeRowKey(k);
    return `${assetId} (${thumbPreset})`;
  });
  setBuildModeChartWrapHeights(keys.length);

  const colors = ['#6b7a90', '#5b9bd5', '#3dd68c'];
  const findRow = (k, mode) => {
    const { assetId, thumbPreset } = parseBuildModeRowKey(k);
    return modeRows.find((r) => r.assetId === assetId && r.thumbPreset === thumbPreset && r.buildMode === mode);
  };

  const deltaDatasets = BUILD_MODE_ORDER.map((mode, i) => ({
    label: BUILD_MODE_LABELS[mode] || mode,
    data: keys.map((k) => {
      const row = findRow(k, mode);
      return row ? row.timingDeltaPct * 100 : null;
    }),
    backgroundColor: colors[i],
    borderRadius: 3,
  }));
  chartInstances.push(new Chart(canvas, {
    type: 'bar',
    data: { labels, datasets: deltaDatasets },
    options: buildModeChartLayoutOptions({
      plugins: {
        title: {
          display: true,
          text: 'Δ% vs ffmpeg — every video fixture × preset',
          color: '#e7ecf3',
          font: { size: 14 },
        },
      },
      scales: {
        x: {
          ticks: { color: '#9aa8bc', maxRotation: 45, minRotation: 45, autoSkip: keys.length > 24 },
          grid: { color: '#2a3544' },
        },
        y: {
          ticks: { color: '#9aa8bc', callback: (v) => v + '%' },
          grid: { color: '#2a3544' },
          title: { display: true, text: 'vs ffmpeg (0% = tie, negative = faster)', color: '#9aa8bc' },
        },
      },
    }),
  }));

  const msDatasets = BUILD_MODE_ORDER.map((mode, i) => ({
    label: BUILD_MODE_LABELS[mode] || mode,
    data: keys.map((k) => {
      const row = findRow(k, mode);
      return row ? row.goMediaWriteMs : null;
    }),
    backgroundColor: colors[i],
    borderRadius: 3,
  }));
  const ffPerFixture = keys.map((k) => {
    for (const mode of BUILD_MODE_ORDER) {
      const row = findRow(k, mode);
      if (row?.ffmpegMs > 0) return row.ffmpegMs;
    }
    return null;
  });
  msDatasets.push({
    label: 'ffmpeg / ffprobe (reference)',
    data: ffPerFixture,
    backgroundColor: CHART_FF_COLOR,
    borderRadius: 3,
  });
  chartInstances.push(new Chart(canvasMs, {
    type: 'bar',
    data: { labels, datasets: msDatasets },
    options: buildModeChartLayoutOptions({
      plugins: {
        title: {
          display: true,
          text: 'Decode + write (ms) vs ffmpeg — every fixture × preset',
          color: '#e7ecf3',
          font: { size: 14 },
        },
      },
      scales: {
        x: {
          ticks: { color: '#9aa8bc', maxRotation: 45, minRotation: 45, autoSkip: keys.length > 24 },
          grid: { color: '#2a3544' },
        },
        y: {
          beginAtZero: true,
          ticks: { color: '#9aa8bc' },
          grid: { color: '#2a3544' },
          title: { display: true, text: 'milliseconds', color: '#9aa8bc' },
        },
      },
    }),
  }));
}

function renderCharts(report) {
  destroyCharts();
  const s = report.summary;
  const results = report.results || [];
  const statusLabels = ['supported', 'typed_unsupported', 'mismatch', 'error', 'skipped'];
  const statusColors = ['#3dd68c', '#f5c542', '#f07178', '#f07178', '#6b7a90'];
  const statusData = statusLabels.map((k) => s.byStatus[k] || 0);

  chartInstances.push(new Chart(document.getElementById('chart-status'), {
    type: 'doughnut',
    data: {
      labels: statusLabels,
      datasets: [{ data: statusData, backgroundColor: statusColors, borderWidth: 0 }],
    },
    options: compactChartOptions({
      plugins: {
        legend: { position: 'bottom', labels: { color: '#9aa8bc', boxWidth: 12, padding: 8 } },
        title: { display: true, text: 'All checks by outcome', color: '#e7ecf3', font: { size: 14 } },
      },
    }),
  }));

  const avgMs = ffmpegActionAverages(report);
  chartInstances.push(new Chart(document.getElementById('chart-timing-totals'), {
    type: 'bar',
    data: {
      labels: ['go-media', 'ffmpeg / ffprobe'],
      datasets: [{
        label: 'Average ms per check',
        data: [avgMs.go, avgMs.ff],
        backgroundColor: [CHART_GO_COLOR, CHART_FF_COLOR],
        borderRadius: 4,
      }],
    },
    options: compactChartOptions({
      plugins: {
        legend: { display: false },
        title: {
          display: true,
          text: `Average latency per ffmpeg comparison (n=${avgMs.n})`,
          color: '#e7ecf3',
          font: { size: 14 },
        },
        tooltip: {
          callbacks: {
            label: (ctx) => `${ctx.parsed.y.toFixed(1)} ms`,
          },
        },
      },
      scales: {
        y: {
          beginAtZero: true,
          grid: { color: '#2a3544' },
          ticks: { color: '#9aa8bc' },
          title: { display: true, text: 'milliseconds (mean)', color: '#9aa8bc' },
        },
        x: { grid: { display: false }, ticks: { color: '#e7ecf3' } },
      },
    }),
  }));

  const cap = s.capabilityBreakdown || [];
  const capLabels = cap.map((c) => c.check.replace('thumbnail_', 'thumb '));
  chartInstances.push(new Chart(document.getElementById('chart-capability'), {
    type: 'bar',
    data: {
      labels: capLabels,
      datasets: [
        { label: 'supported', data: cap.map((c) => c.supported), backgroundColor: '#3dd68c', stack: 's' },
        { label: 'other', data: cap.map((c) => c.other), backgroundColor: '#f5c542', stack: 's' },
        { label: 'skipped', data: cap.map((c) => c.skipped), backgroundColor: '#6b7a90', stack: 's' },
      ],
    },
    options: compactChartOptions({
      indexAxis: 'y',
      plugins: {
        title: {
          display: true,
          text: 'Capability by check type',
          color: '#e7ecf3',
          font: { size: 14 },
        },
        legend: { position: 'bottom', labels: { color: '#9aa8bc', boxWidth: 12 } },
      },
      scales: {
        x: { stacked: true, beginAtZero: true, grid: { color: '#2a3544' }, ticks: { color: '#9aa8bc', stepSize: 1 } },
        y: { stacked: true, grid: { display: false }, ticks: { color: '#e7ecf3', font: { size: 11 } } },
      },
    }),
  }));

  const deltas = results
    .filter((r) => r.timingDeltaPct != null && timingComparableRow(r) && timingGateRow(r))
    .map((r) => r.timingDeltaPct * 100);
  const histBins = [-100, -50, -25, -10, 0, 10, 25, 50, 100, 200];
  const histCounts = new Array(histBins.length - 1).fill(0);
  for (const d of deltas) {
    for (let i = 0; i < histBins.length - 1; i++) {
      if (d >= histBins[i] && d < histBins[i + 1]) { histCounts[i]++; break; }
    }
  }
  const histLabels = histBins.slice(0, -1).map((b, i) => `${b}…${histBins[i + 1]}%`);
  chartInstances.push(new Chart(document.getElementById('chart-timing-histogram'), {
    type: 'bar',
    data: {
      labels: histLabels,
      datasets: [{
        label: 'checks',
        data: histCounts,
        backgroundColor: histLabels.map((_, i) => (histBins[i] < 0 ? '#3dd68c' : '#f07178')),
        borderRadius: 3,
      }],
    },
    options: {
      indexAxis: 'y',
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        title: { display: true, text: 'Thumbnail Δ% distribution (gated rows)', color: '#e7ecf3', font: { size: 13 } },
      },
      scales: {
        x: { beginAtZero: true, grid: { color: '#2a3544' }, ticks: { color: '#9aa8bc' } },
        y: { grid: { display: false }, ticks: { color: '#e7ecf3', font: { size: 11 } } },
      },
    },
  }));

  const checks = ['duration', 'probe', 'tags', 'artwork', 'lyrics', 'subtitles', 'thumbnail_small', 'thumbnail_large'];
  const checkLabels = checks.map((c) => c.replace('thumbnail_', 'thumb '));
  const medGoByCheck = medianMsByCheck(results, checks, (r) => goMsForCompare(r));
  const medFfByCheck = medianMsByCheck(results, checks, (r) => r.ffmpegMs);
  chartInstances.push(new Chart(document.getElementById('chart-check-median'), {
    type: 'bar',
    data: {
      labels: checkLabels,
      datasets: [
        { label: 'go-media', data: medGoByCheck, backgroundColor: CHART_GO_COLOR, borderRadius: 3 },
        { label: 'ffmpeg / ffprobe', data: medFfByCheck, backgroundColor: CHART_FF_COLOR, borderRadius: 3 },
      ],
    },
    options: pairedMsChartOptions('Median latency by check type (single harness run)'),
  }));

  const { go: byAssetGo, ff: byAssetFf } = sumMsByAsset(results);
  const assetLabels = Object.keys(byAssetGo).sort();
  const assetGoVals = assetLabels.map((k) => byAssetGo[k] || 0);
  const assetFfVals = assetLabels.map((k) => byAssetFf[k] || 0);
  const wrap = document.getElementById('chart-per-asset').closest('.chart-wrap');
  if (wrap) wrap.style.height = `${Math.max(280, assetLabels.length * 16 + 56)}px`;
  chartInstances.push(new Chart(document.getElementById('chart-per-asset'), {
    type: 'bar',
    data: {
      labels: assetLabels,
      datasets: [
        { label: 'go-media', data: assetGoVals, backgroundColor: CHART_GO_COLOR, borderRadius: 3 },
        { label: 'ffmpeg / ffprobe', data: assetFfVals, backgroundColor: CHART_FF_COLOR, borderRadius: 3 },
      ],
    },
    options: pairedMsChartOptions('Total time per asset (all timed checks)', {
      scales: {
        y: { grid: { display: false }, ticks: { color: '#e7ecf3', font: { size: 10 }, autoSkip: false } },
      },
    }),
  }));
}

function renderReport(report) {
  reportData = report;
  const s = report.summary;
  document.getElementById('meta').textContent =
    `${s.generatedAt} · seed ${s.seedCommit.slice(0, 7)} · ${s.ffmpegVersion}`;
  const t = s.timing || {};
  let timingLine = `Thumbnail gate rows: ${t.comparableRows || 0} · avg Δ ${((t.avgTimingDeltaPct || 0) * 100).toFixed(1)}% · median Δ ${((t.medianTimingDeltaPct || 0) * 100).toFixed(1)}% (vs ffmpeg, in-process)`;
  if (s.previousRun) {
    const pr = s.previousRun;
    timingLine += ` · prev run ${pr.generatedAt} (${pr.timingRegressions} timing regressions, ${pr.statusRegressions} status regressions)`;
  }
  if (s.timingRelaxed?.length) {
    timingLine +=
      ' · MJPEG relaxed gate: ' +
      s.timingRelaxed
        .map(
          (r) =>
            `${r.assetId} ${r.check} Δ${(r.timingDeltaPct * 100).toFixed(0)}% (cap ${(r.timingGateLimitPct * 100).toFixed(0)}%)`,
        )
        .join('; ');
  }
  document.getElementById('timing-meta').textContent = timingLine;
  const gateEl = document.getElementById('gate-build-meta');
  if (gateEl) {
    const gate = s.timingGateBuild || 'amd64_asm_kernels_pgo_in_process';
    const tiers = (s.buildModeTiers || []).join(' · ');
    gateEl.textContent =
      `Production: CGO_ENABLED=0 + -pgo=default.pgo (tier 2 / in-process ${gate}). Gate: expectations.json timingGate. Tiers: ${tiers || 'diagnostic → production (gated) → +SIMD (diagnostic)'}.`;
  }

  const cards = document.getElementById('summary-cards');
  cards.innerHTML = '';
  const statusOrder = ['supported', 'typed_unsupported', 'mismatch', 'error', 'skipped'];
  for (const key of statusOrder) {
    const count = s.byStatus[key] || 0;
    const div = document.createElement('div');
    div.className = 'card';
    div.innerHTML = `<div class="label">${key}</div><div class="value status-${key}">${count}</div>`;
    cards.appendChild(div);
  }
  const extra = document.createElement('div');
  extra.className = 'card';
  extra.innerHTML = `<div class="label">avg Δ timing</div><div class="value ${(t.avgTimingDeltaPct || 0) <= 0 ? 'delta-good' : 'delta-bad'}">${((t.avgTimingDeltaPct || 0) * 100).toFixed(1)}%</div>`;
  cards.appendChild(extra);

  const checks = ['duration', 'probe', 'tags', 'artwork', 'lyrics', 'subtitles', 'thumbnail_small', 'thumbnail_large'];
  const byAsset = {};
  for (const r of report.results) {
    if (!byAsset[r.assetId]) byAsset[r.assetId] = {};
    byAsset[r.assetId][r.check] = r;
  }

  const matrixFilter = document.getElementById('matrix-filter');
  const renderMatrix = () => {
    const q = (matrixFilter.value || '').toLowerCase();
    const matrixBody = document.querySelector('#matrix-table tbody');
    matrixBody.innerHTML = '';
    for (const a of report.assets) {
      if (q && !a.id.toLowerCase().includes(q) && !(a.description || '').toLowerCase().includes(q)) continue;
      const tr = document.createElement('tr');
      tr.innerHTML = `<td><strong>${a.id}</strong><br><span class="muted">${a.description}</span></td>`;
      for (const c of checks) {
        const r = (byAsset[a.id] || {})[c];
        const st = r ? r.status : 'skipped';
        tr.innerHTML += `<td class="status-${st}" data-sort-val="${statusRank(st)}">${st}</td>`;
      }
      matrixBody.appendChild(tr);
    }
  };
  matrixFilter.addEventListener('input', renderMatrix);
  renderMatrix();

  const maxMs = Math.max(1, ...report.results.map((r) => Math.max(r.goMediaMs || 0, r.ffmpegMs || 0)));
  const maxPsnr = Math.max(1, ...report.results.map((r) => r.psnr || 0));

  const filterAsset = document.getElementById('filter-asset');
  const filterCheck = document.getElementById('filter-check');
  const filterStatus = document.getElementById('filter-status');
  const filterDelta = document.getElementById('filter-delta');
  for (const c of checks) {
    filterCheck.append(new Option(c, c));
  }
  for (const st of statusOrder) {
    filterStatus.append(new Option(st, st));
  }

  const filteredResults = () => report.results.filter((r) => {
    const aq = (filterAsset.value || '').toLowerCase();
    if (aq && !r.assetId.toLowerCase().includes(aq)) return false;
    if (filterCheck.value && r.check !== filterCheck.value) return false;
    if (filterStatus.value && r.status !== filterStatus.value) return false;
    if (filterDelta.value === 'faster' && (r.timingDeltaPct == null || r.timingDeltaPct > 0)) return false;
    if (filterDelta.value === 'slower' && (r.timingDeltaPct == null || r.timingDeltaPct <= 0)) return false;
    if (filterDelta.value === 'regression' && !r.timingRegression) return false;
    return true;
  });

  const sortVal = (r, key) => {
    switch (key) {
      case 'assetId': return r.assetId;
      case 'check': return r.check;
      case 'status': return statusRank(r.status);
      case 'goMediaMs': return r.goMediaMs ?? -1;
      case 'ffmpegMs': return r.ffmpegMs ?? -1;
      case 'timingDeltaPct': return r.timingDeltaPct ?? 999;
      case 'durationDeltaSec': return Math.abs(r.durationDeltaSec ?? 0);
      case 'psnr': return r.psnr ?? -1;
      case 'ssim': return r.ssim ?? -1;
      default: return '';
    }
  };

  const renderResults = (sortKey, sortDir) => {
    const rows = filteredResults().slice().sort((a, b) => {
      const va = sortVal(a, sortKey);
      const vb = sortVal(b, sortKey);
      if (va < vb) return -sortDir;
      if (va > vb) return sortDir;
      return 0;
    });
    const tbody = document.querySelector('#results-table tbody');
    tbody.innerHTML = rows.map((r) => {
      const reg = r.timingRegression ? ' <span class="reg-badge">prev slower</span>' : '';
      const stReg = r.statusRegression ? ' <span class="reg-badge bad">status</span>' : '';
      return `<tr>
        <td>${r.assetId}</td>
        <td>${r.check}</td>
        <td class="status-${r.status}">${r.status}${stReg}</td>
        <td class="metric-col">${timingComparableRow(r) ? metricBar(r.goMediaMs || 0, maxMs, 'go', (v) => `${v.toFixed(2)}`, `go-media ${r.goMediaMs?.toFixed(2)} ms`) + reg : '—'}</td>
        <td class="metric-col">${timingComparableRow(r) && r.ffmpegMs ? metricBar(r.ffmpegMs, maxMs, 'ff', (v) => `${v.toFixed(2)}`, `ffmpeg ${r.ffmpegMs.toFixed(2)} ms`) : '—'}</td>
        <td>${timingGateRow(r) && timingComparableRow(r) ? deltaPctCell(r.timingDeltaPct) : '—'}</td>
        <td>${qualityBar(r.durationDeltaSec, 1, false)}</td>
        <td>${r.psnr ? qualityBar(r.psnr, maxPsnr, true) : '—'}</td>
        <td>${r.ssim ? qualityBar(r.ssim, 1, true) : '—'}</td>
        <td class="detail-cell">${r.detail || (r.fieldDiffs || []).join('; ')}</td>
      </tr>`;
    }).join('');
  };

  const refreshResults = makeSortable(document.getElementById('results-table'), filteredResults, renderResults);
  [filterAsset, filterCheck, filterStatus, filterDelta].forEach((el) => el.addEventListener('input', refreshResults));
  refreshResults();

  const grid = document.getElementById('thumb-grid');
  grid.innerHTML = '';
  for (const r of report.results) {
    if (r.check !== 'thumbnail_small' && r.check !== 'thumbnail_large') continue;
    if (!r.thumbFFPath && !r.thumbGoPath) continue;
    const card = document.createElement('div');
    card.className = 'thumb-card';
    card.dataset.assetId = r.assetId;
    card.dataset.thumbPreset = r.thumbPreset || (r.check === 'thumbnail_large' ? 'large' : 'small');
    const preset = card.dataset.thumbPreset;
    const ffMissing = !r.thumbFFPath;
    const goMissing = !r.thumbGoPath;
    card.innerHTML = `
      <h3>${r.assetId} (${preset}) <span class="status-${r.status}">${r.status}</span></h3>
      ${compareSliderMarkup({
        assetId: r.assetId,
        goSrc: thumbCompareSrc(r.thumbGoPath),
        ffSrc: thumbCompareSrc(r.thumbFFPath),
        goMissing,
        ffMissing,
      })}
      <p>${thumbStatsLine(r)}</p>`;
    grid.appendChild(card);
    card.querySelectorAll('.compare-slider img').forEach(attachThumbImgFallback);
    const previewSlider = card.querySelector('.compare-slider');
    initCompareSlider(previewSlider, {
      onZoom: (pct) => openThumbCompareLightbox(card, pct),
    });
  }

  const modeRows = report.buildModeTimings || [];
  const modeTbody = document.querySelector('#buildmode-table tbody');
  if (modeTbody) {
    modeTbody.innerHTML = '';
    const byKey = {};
    for (const r of modeRows) {
      const k = `${r.assetId}:${r.thumbPreset}`;
      if (!byKey[k]) byKey[k] = {};
      byKey[k][r.buildMode] = r;
    }
    const sorted = [...modeRows].sort(
      (a, b) =>
        (a.assetId + a.thumbPreset).localeCompare(b.assetId + b.thumbPreset) ||
        (a.tier || 0) - (b.tier || 0),
    );
    for (const r of sorted) {
      const tr = document.createElement('tr');
      const gated = r.timingGateApplicable !== false && r.buildMode !== 'purego_pgo_simd';
      const deltaClass = gated ? (r.timingGatePass ? 'delta-good' : 'delta-bad') : '';
      const pctClass = gated ? deltaClass : '';
      const prevMode = BUILD_MODE_ORDER[(r.tier || 1) - 2];
      let inc = '—';
      if (prevMode) {
        const prev = byKey[`${r.assetId}:${r.thumbPreset}`]?.[prevMode];
        if (prev && prev.goMediaWriteMs > 0) {
          inc = `${(((r.goMediaWriteMs - prev.goMediaWriteMs) / prev.goMediaWriteMs) * 100).toFixed(0)}%`;
        }
      }
      const gateCell = gated
        ? (r.timingGatePass ? 'pass' : 'fail')
        : 'n/a';
      if (r.buildMode === 'purego_pgo') {
        tr.classList.add('production-tier');
      }
      tr.innerHTML = `<td>${r.tier ?? '—'}</td><td>${r.assetId}</td><td>${r.thumbPreset}</td><td>${BUILD_MODE_LABELS[r.buildMode] || r.buildMode}</td>
        <td>${r.goMediaWriteMs?.toFixed(1) ?? '—'}</td><td>${r.ffmpegMs?.toFixed(1) ?? '—'}</td>
        <td class="${pctClass}">${(r.timingDeltaPct * 100).toFixed(0)}%</td>
        <td>${inc}</td>
        <td class="${deltaClass}">${gateCell}</td>`;
      modeTbody.appendChild(tr);
    }
  }
  const scalerRows = report.scalerBenchmarks || [];
  const renderScalerBody = (sortKey, sortDir) => {
    const tbody = document.querySelector('#scaler-table tbody');
    if (!tbody) return;
    const rows = [...scalerRows];
    rows.sort((a, b) => {
      const av = a[sortKey] ?? '';
      const bv = b[sortKey] ?? '';
      if (typeof av === 'number' && typeof bv === 'number') {
        return (av - bv) * sortDir;
      }
      return String(av).localeCompare(String(bv)) * sortDir;
    });
    tbody.innerHTML = '';
    for (const r of rows) {
      const tr = document.createElement('tr');
      const deltaClass = r.deltaPct <= 0.1 ? 'delta-good' : 'delta-bad';
      tr.innerHTML = `<td>${r.assetId}</td><td>${r.preset}</td><td>${r.scaler}</td>
        <td>${r.goMediaMs?.toFixed(1) ?? '—'}</td><td>${r.goWriteMs?.toFixed(1) ?? '—'}</td>
        <td>${r.ffmpegMs?.toFixed(1) ?? '—'}</td>
        <td class="${deltaClass}">${r.deltaPct != null ? (r.deltaPct * 100).toFixed(0) + '%' : '—'}</td>
        <td>${r.psnr?.toFixed(1) ?? '—'}</td><td>${r.ssim?.toFixed(3) ?? '—'}</td>`;
      tbody.appendChild(tr);
    }
  };
  const scalerTable = document.getElementById('scaler-table');
  if (scalerTable) {
    makeSortable(scalerTable, () => scalerRows, renderScalerBody)();
  }
  renderCharts(report);
  renderBuildModeCharts(modeRows);
}

async function loadReport() {
  const res = await fetch('data/report.json');
  const report = await res.json();
  renderReport(report);
}

initLightbox();
loadReport().catch((e) => {
  document.getElementById('meta').textContent = 'Failed to load report: ' + e;
});
