#!/usr/bin/env node
/**
 * Browser screenshots for docs/comparison/*.png from test/compare/report_site.
 *
 * Usage: node capture-comparison-docs.mjs <report-url-with-capture-query> <docs-dir>
 */
import { chromium } from 'playwright';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';

const [baseUrl, docsDir] = process.argv.slice(2);
if (!baseUrl || !docsDir) {
  console.error('usage: node capture-comparison-docs.mjs <url> <docs-dir>');
  process.exit(1);
}

const url = baseUrl.includes('capture=1') ? baseUrl : `${baseUrl}${baseUrl.includes('?') ? '&' : '?'}capture=1`;

await mkdir(docsDir, { recursive: true });

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
await page.goto(url, { waitUntil: 'networkidle' });
await page.waitForSelector('#summary-cards .card');
await page.waitForSelector('#matrix-table tbody tr');
await page.waitForSelector('.thumb-card[data-asset-id="mjpeg_mp4"] .compare-slider img');
await page.waitForSelector('#chart-status');

await page.evaluate(() => window.scrollTo(0, 0));
const dashClip = await page.evaluate(() => {
  const header = document.querySelector('header');
  const cards = document.querySelector('.cards');
  const charts = document.getElementById('charts-summary');
  const matrixSection = document.querySelector('#matrix-table')?.closest('section');
  const rects = [header, cards, charts, matrixSection].filter(Boolean).map((el) => el.getBoundingClientRect());
  const top = Math.min(...rects.map((r) => r.top));
  const left = Math.min(...rects.map((r) => r.left));
  const right = Math.max(...rects.map((r) => r.right));
  const bottom = Math.max(...rects.map((r) => r.bottom));
  return {
    x: Math.max(0, left),
    y: Math.max(0, top),
    width: right - left,
    height: bottom - top,
  };
});
await page.screenshot({ path: join(docsDir, 'report-dashboard.png'), clip: dashClip });

await page.locator('#charts-summary').scrollIntoViewIfNeeded();
await page.waitForTimeout(500);
await page.locator('#charts-metrics').screenshot({ path: join(docsDir, 'report-charts.png') });

const mjpegSlider = page.locator('.thumb-card[data-asset-id="mjpeg_mp4"] .compare-slider');
await mjpegSlider.scrollIntoViewIfNeeded();
await mjpegSlider.screenshot({ path: join(docsDir, 'mjpeg-thumbnail-side-by-side.png') });

const vp8Slider = page.locator('.thumb-card[data-asset-id="webm_vp8"] .compare-slider');
await vp8Slider.scrollIntoViewIfNeeded();
await vp8Slider.evaluate((el) => {
  const clip = el.querySelector('.compare-clip');
  const handle = el.querySelector('.compare-handle');
  if (clip) clip.style.clipPath = 'inset(0 100% 0 0)';
  if (handle) handle.style.left = '0%';
});
await vp8Slider.screenshot({ path: join(docsDir, 'vp8-ffmpeg-frame.png') });

await browser.close();
