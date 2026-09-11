/**
 * Design-review captures of the built static export.
 *
 * A page is not done when it builds; it is done when an independent review of
 * its pixels finds nothing above minor. This script produces those pixels the
 * same way every time: it serves out/ itself (no external server to start),
 * opens each scene in headless Chromium at the review widths, and writes one
 * full-page PNG per scene. Fixed viewports, a fixed user agent per scene, and
 * a settled network mean any pixel difference between two runs is a change in
 * the page -- which is what makes a before/after grade honest.
 *
 * Scenes that depend on the browser's platform also ASSERT the page did the
 * right thing: a Windows user agent must promote the Windows card, and so on.
 * A failed assertion fails the run, so the harness is a test as well as a
 * camera.
 *
 * Usage (after `make build`):
 *   node scripts/capture-pages.mjs --out /path/to/captures
 *   node scripts/capture-pages.mjs --out ... --only download-1280,download-windows
 *   node scripts/capture-pages.mjs --out ... --tag before        # <scene>-before-dark.png
 *   node scripts/capture-pages.mjs --out ... --publish-og        # also refresh public/_site/images/og/*.png
 *   node scripts/capture-pages.mjs --out ... --base http://localhost:4175   # an already-running server
 *
 * The website has one theme (dark), so every file is <scene>-dark.png; a
 * reviewer asking for the light pair is told the surface has none.
 */

import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const puppeteer = require('puppeteer');

const args = process.argv.slice(2);
const arg = (name, fallback) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 ? args[i + 1] : fallback;
};
const flag = (name) => args.includes(`--${name}`);

const OUT_DIR = arg('out');
if (!OUT_DIR) {
  console.error('usage: capture-pages.mjs --out <dir> [--only a,b] [--tag before] [--publish-og] [--base URL]');
  process.exit(2);
}
const ONLY = arg('only', '')
  .split(',')
  .map((s) => s.trim())
  .filter(Boolean);
const TAG = arg('tag', '');
const PUBLISH_OG = flag('publish-og');

const siteRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const exportDir = path.join(siteRoot, 'out');
const ogDir = path.join(siteRoot, 'public/_site/images/og');

// The frozen user-agent strings browsers actually send; the page's detector
// reads these when Client Hints are absent (Firefox, Safari) and the hint
// platform when present (Chromium). Both paths are exercised below.
const UA = {
  mac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
  windows: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36',
  linux: 'Mozilla/5.0 (X11; Linux x86_64; rv:129.0) Gecko/20100101 Firefox/129.0',
  iphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1',
};

/**
 * One row per scene. `expectTab` is the platform tab the page must have
 * selected on load (asserted); `og` marks a 1200x630 first-screen capture
 * that is also the page's Open Graph image when --publish-og is given.
 */
const SCENES = [
  { name: 'download-1680', route: '/download', width: 1680, ua: UA.mac, expectTab: 'macOS' },
  { name: 'download-1280', route: '/download', width: 1280, ua: UA.mac, expectTab: 'macOS' },
  { name: 'download-windows', route: '/download', width: 1280, ua: UA.windows, uaPlatform: 'Windows', expectTab: 'Windows' },
  { name: 'download-linux', route: '/download', width: 1280, ua: UA.linux, expectTab: 'Linux' },
  { name: 'download-phone', route: '/download', width: 390, ua: UA.iphone, expectTab: 'macOS' },
  { name: 'download-og', route: '/download', width: 1200, height: 630, ua: UA.mac, og: 'download.png' },
];

// ---------------------------------------------------------------------------
// A static server for out/: enough of one to render the export the way GitHub
// Pages does (route -> route.html or route/index.html; /_site/_next assets).
// ---------------------------------------------------------------------------
const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.woff2': 'font/woff2',
  '.woff': 'font/woff',
  '.ico': 'image/x-icon',
  '.txt': 'text/plain',
  '.xml': 'application/xml',
};

function resolveFile(urlPath) {
  const clean = decodeURIComponent(urlPath.split('?')[0]).replace(/\/+$/, '') || '/';
  const candidates =
    clean === '/'
      ? [path.join(exportDir, 'index.html')]
      : [path.join(exportDir, clean), path.join(exportDir, `${clean}.html`), path.join(exportDir, clean, 'index.html')];
  for (const c of candidates) {
    if (c.startsWith(exportDir) && fs.existsSync(c) && fs.statSync(c).isFile()) return c;
  }
  return null;
}

function serveExport() {
  if (!fs.existsSync(exportDir)) {
    console.error(`no static export at ${exportDir}; run \`make build\` first`);
    process.exit(2);
  }
  const server = http.createServer((req, res) => {
    const file = resolveFile(req.url ?? '/');
    if (!file) {
      res.writeHead(404);
      res.end('not found');
      return;
    }
    res.writeHead(200, { 'content-type': MIME[path.extname(file)] ?? 'application/octet-stream' });
    fs.createReadStream(file).pipe(res);
  });
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => resolve({ server, base: `http://127.0.0.1:${server.address().port}` }));
  });
}

// ---------------------------------------------------------------------------

async function capture(browser, base, scene) {
  const page = await browser.newPage();
  await page.setViewport({ width: scene.width, height: scene.height ?? 900, deviceScaleFactor: 1 });
  await page.setUserAgent(scene.ua);
  if (scene.uaPlatform) {
    // Chromium exposes navigator.userAgentData; give the page the hint a real
    // Chromium on that platform would send, so the Client Hints path is tested.
    await page.evaluateOnNewDocument((platform) => {
      Object.defineProperty(navigator, 'userAgentData', { value: { platform }, configurable: true });
    }, scene.uaPlatform);
  }
  await page.emulateMediaFeatures([
    { name: 'prefers-color-scheme', value: 'dark' },
    { name: 'prefers-reduced-motion', value: 'reduce' },
  ]);
  await page.goto(base + scene.route, { waitUntil: 'networkidle0', timeout: 45000 });
  if (scene.og) {
    // A share image is a poster, not a viewport: no site chrome, no clipped
    // card, the page's own headline block centred in the 1200x630 frame.
    await page.addStyleTag({
      content: `
        header { display: none !important; }
        main { padding-top: 0 !important; }
        main section:first-of-type { min-height: 630px; display: flex; align-items: center; padding: 0 !important; }
        main section:first-of-type .mb-10 { margin-bottom: 0 !important; }
        main section:first-of-type h1 { font-size: 64px !important; line-height: 1.1 !important; margin-bottom: 24px !important; }
        main section:first-of-type h1 + p { font-size: 24px !important; line-height: 1.4 !important; max-width: 820px !important; }
        main section:first-of-type h1 + p + p { font-size: 16px !important; margin-top: 24px !important; }
        main section:first-of-type a { text-decoration: none !important; }
        [role="tablist"], div:has(> [role="tabpanel"]) { display: none !important; }
      `,
    });
  }
  await new Promise((r) => setTimeout(r, 600));

  let verdict = null;
  if (scene.expectTab) {
    const selected = await page.$eval('[role="tab"][aria-selected="true"]', (el) => el.textContent?.trim()).catch(() => null);
    verdict = selected === scene.expectTab ? `ok (${selected})` : `FAIL expected ${scene.expectTab}, got ${selected}`;
  }

  const suffix = TAG ? `-${TAG}` : '';
  const file = path.join(OUT_DIR, `${scene.name}${suffix}-dark.png`);
  await page.screenshot({ path: file, fullPage: !scene.og });
  if (scene.og && PUBLISH_OG) {
    fs.mkdirSync(ogDir, { recursive: true });
    fs.copyFileSync(file, path.join(ogDir, scene.og));
  }
  await page.close();
  return { file, verdict };
}

async function main() {
  const scenes = ONLY.length ? SCENES.filter((s) => ONLY.includes(s.name)) : SCENES;
  if (!scenes.length) {
    console.error(`no scenes match --only ${ONLY.join(',')}; known: ${SCENES.map((s) => s.name).join(', ')}`);
    process.exit(2);
  }
  fs.mkdirSync(OUT_DIR, { recursive: true });

  const external = arg('base');
  const served = external ? null : await serveExport();
  const base = external ?? served.base;
  const browser = await puppeteer.launch({ headless: true, args: ['--no-sandbox'] });

  let failed = 0;
  try {
    for (const scene of scenes) {
      const { file, verdict } = await capture(browser, base, scene);
      if (verdict?.startsWith('FAIL')) failed += 1;
      console.log(`${path.relative(process.cwd(), file)}${verdict ? `  ${verdict}` : ''}`);
    }
  } finally {
    await browser.close();
    served?.server.close();
  }
  if (failed) {
    console.error(`${failed} scene assertion(s) failed`);
    process.exit(1);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
