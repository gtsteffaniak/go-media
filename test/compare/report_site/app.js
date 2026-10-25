async function loadReport() {
  const res = await fetch('data/report.json');
  const report = await res.json();
  const s = report.summary;
  document.getElementById('meta').textContent =
    `${s.generatedAt} · seed ${s.seedCommit.slice(0, 7)} · ${s.ffmpegVersion}`;

  const cards = document.getElementById('summary-cards');
  const statusOrder = ['supported', 'typed_unsupported', 'mismatch', 'error', 'skipped'];
  for (const key of statusOrder) {
    const count = s.byStatus[key] || 0;
    const div = document.createElement('div');
    div.className = 'card';
    div.innerHTML = `<div class="label">${key}</div><div class="value status-${key}">${count}</div>`;
    cards.appendChild(div);
  }

  const checks = ['duration', 'probe', 'tags', 'artwork', 'lyrics', 'subtitles', 'thumbnail'];
  const byAsset = {};
  for (const r of report.results) {
    if (!byAsset[r.assetId]) byAsset[r.assetId] = {};
    byAsset[r.assetId][r.check] = r;
  }

  const matrixBody = document.querySelector('#matrix-table tbody');
  for (const a of report.assets) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td><strong>${a.id}</strong><br><span style="color:#9aa8bc">${a.description}</span></td>`;
    for (const c of checks) {
      const r = (byAsset[a.id] || {})[c];
      const st = r ? r.status : 'skipped';
      tr.innerHTML += `<td class="status-${st}">${st}</td>`;
    }
    matrixBody.appendChild(tr);
  }

  const tbody = document.querySelector('#results-table tbody');
  for (const r of report.results) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${r.assetId}</td>
      <td>${r.check}</td>
      <td class="status-${r.status}">${r.status}</td>
      <td>${r.goMediaMs?.toFixed(2) ?? ''}</td>
      <td>${r.ffmpegMs ? r.ffmpegMs.toFixed(2) : ''}</td>
      <td>${r.durationDeltaSec != null ? r.durationDeltaSec.toFixed(3) : ''}</td>
      <td>${r.psnr ? r.psnr.toFixed(1) : ''}</td>
      <td>${r.ssim ? r.ssim.toFixed(3) : ''}</td>
      <td>${r.detail || (r.fieldDiffs || []).join('; ')}</td>`;
    tbody.appendChild(tr);
  }

  const grid = document.getElementById('thumb-grid');
  for (const r of report.results) {
    if (r.check !== 'thumbnail' || !r.thumbGoPath) continue;
    const card = document.createElement('div');
    card.className = 'thumb-card';
    card.innerHTML = `
      <h3>${r.assetId} <span class="status-${r.status}">${r.status}</span></h3>
      <div class="thumb-pair">
        <figure><img src="${r.thumbFFPath}" alt="ffmpeg" /><figcaption>ffmpeg</figcaption></figure>
        <figure><img src="${r.thumbGoPath}" alt="go-media" /><figcaption>go-media</figcaption></figure>
      </div>
      <p>PSNR ${r.psnr?.toFixed(1) ?? '—'} · SSIM ${r.ssim?.toFixed(3) ?? '—'} · ${r.detail || ''}</p>`;
    grid.appendChild(card);
  }
}

loadReport().catch((e) => {
  document.getElementById('meta').textContent = 'Failed to load report: ' + e;
});
