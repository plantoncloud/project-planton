/**
 * Pre-build script: generates src/generated/desktop-release.json
 *
 * The install page links to version-free installer aliases, so it never needs
 * a version to work. It DOES want one for two small things -- the "Latest:
 * v0.0.56" badge and the link to that release's checksums -- and it wants the
 * installers' sizes so a person on a slow connection knows what they are in
 * for. Both live on the downloads CDN, which sends no CORS headers, so a
 * browser cannot read them; this script reads them at build time instead
 * (Node has no CORS) and bakes the answer into a JSON file the page imports.
 *
 * The file is generated, never committed (src/generated/ is gitignored), and
 * written by both `yarn build` and `yarn dev` so an import of it always
 * resolves. Every failure is soft: an unreachable CDN yields a file with
 * `version: null` and no sizes, and the page simply renders no badge, no
 * checksums link, and no sizes. A release pointer being briefly unreadable
 * must never fail the site build.
 *
 * The artifact list is imported from src/data/desktop-download.ts -- the one
 * source of download URLs -- via Node's type stripping, so this script cannot
 * drift from the page it feeds.
 *
 * Usage:  node --experimental-strip-types scripts/generate-desktop-release.mjs
 * Called: automatically before `next build` and `next dev` via package.json
 */

import fs from 'node:fs';
import path from 'node:path';
import { DESKTOP_PLATFORMS, DOWNLOADS_LATEST, desktopChecksumsUrl } from '../src/data/desktop-download.ts';

const OUTPUT_DIR = path.join(process.cwd(), 'src/generated');
const OUTPUT_PATH = path.join(OUTPUT_DIR, 'desktop-release.json');
const VERSION_URL = `${DOWNLOADS_LATEST}/version.txt`;
const TIMEOUT_MS = 8000;

// A stable tag ("v0.0.56") or a per-surface desktop tag ("v0.0.34-desktop.20260827.7");
// anything else is not a version we want to print on a public page.
const TAG_SHAPE = /^v\d+\.\d+\.\d+(-[0-9a-z-]+\.\d{8}\.\d+)?$/;

const fetchWithTimeout = (url, init = {}) =>
  fetch(url, { ...init, signal: AbortSignal.timeout(TIMEOUT_MS), redirect: 'follow' });

async function readLatestVersion() {
  const response = await fetchWithTimeout(VERSION_URL, { cache: 'no-store' });
  if (!response.ok) throw new Error(`${VERSION_URL} answered ${response.status}`);
  const version = (await response.text()).trim();
  if (!TAG_SHAPE.test(version)) throw new Error(`${VERSION_URL} carries an unexpected value: ${JSON.stringify(version)}`);
  return version;
}

async function readArtifactSize(href) {
  const response = await fetchWithTimeout(href, { method: 'HEAD' });
  if (!response.ok) throw new Error(`${href} answered ${response.status}`);
  const length = Number(response.headers.get('content-length'));
  return Number.isFinite(length) && length > 0 ? length : null;
}

function write(output) {
  fs.mkdirSync(OUTPUT_DIR, { recursive: true });
  fs.writeFileSync(OUTPUT_PATH, JSON.stringify(output, null, 2) + '\n');
}

async function main() {
  const output = { version: null, checksumsUrl: null, artifacts: {}, generatedAt: new Date().toISOString() };

  try {
    output.version = await readLatestVersion();
    output.checksumsUrl = desktopChecksumsUrl(output.version);
  } catch (err) {
    console.warn(`[desktop-release.json] Could not read the latest version; the page renders without a badge. ${err.message}`);
  }

  const artifacts = DESKTOP_PLATFORMS.filter((p) => p.available).flatMap((p) => p.artifacts);
  await Promise.all(
    artifacts.map(async (artifact) => {
      try {
        const bytes = await readArtifactSize(artifact.href);
        if (bytes) output.artifacts[artifact.key] = { bytes };
      } catch (err) {
        console.warn(`[desktop-release.json] No size for ${artifact.key}: ${err.message}`);
      }
    }),
  );

  write(output);
  console.log(
    `[desktop-release.json] version=${output.version ?? 'unknown'} sizes=${Object.keys(output.artifacts).length}/${artifacts.length}`,
  );
}

main().catch((err) => {
  // The script's own failure is the one thing that must not stop the build.
  console.warn(`[desktop-release.json] Unexpected failure; writing an empty file. ${err.message}`);
  write({ version: null, checksumsUrl: null, artifacts: {}, generatedAt: new Date().toISOString() });
});
