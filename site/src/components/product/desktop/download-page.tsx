'use client';

import { FC, useEffect, useState } from 'react';
import { DESKTOP_PLATFORMS, DOWNLOADS_LATEST, type DesktopPlatformId } from '@/data/desktop-download';
import { DESKTOP_RELEASE } from '@/data/desktop-release';
import { DownloadHero } from './download-hero';
import { DownloadAfterInstall } from './download-after-install';
import { DownloadVerify } from './download-verify';
import { useDetectedPlatform } from './use-detected-platform';

// The version the page was built with is the floor; once the downloads bucket
// permits a cross-origin read, the browser refreshes it so the badge never
// lags a release by a site deploy. Silent on failure: the badge simply keeps
// the build-time value, or stays absent.
function useLatestVersion(): string | null {
  const [version, setVersion] = useState<string | null>(DESKTOP_RELEASE.version);
  useEffect(() => {
    const controller = new AbortController();
    fetch(`${DOWNLOADS_LATEST}/version.txt`, { signal: controller.signal, cache: 'no-store' })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(String(r.status)))))
      .then((text) => {
        const fresh = text.trim();
        if (/^v\d+\.\d+\.\d+/.test(fresh)) setVersion(fresh);
      })
      .catch(() => {});
    return () => controller.abort();
  }, []);
  return version;
}

// One platform selection for the whole page. The hero's tabs change it, and
// every section below follows: the install steps, the CLI note, and the
// verification commands are all the selected platform's, never macOS's by
// default. A platform with no installer gets its door in the hero and none of
// the "after you install" story it cannot yet begin.
export const DownloadPage: FC = () => {
  const detected = useDetectedPlatform();
  const [override, setOverride] = useState<DesktopPlatformId | null>(null);
  const version = useLatestVersion();

  // Detected wins until the person picks; a phone (null) and the prerender
  // (undefined) both fall back to the first listed platform.
  const selectedId: DesktopPlatformId = override ?? detected ?? DESKTOP_PLATFORMS[0].id;
  const platform = DESKTOP_PLATFORMS.find((p) => p.id === selectedId) ?? DESKTOP_PLATFORMS[0];

  return (
    <>
      <DownloadHero platform={platform} onSelect={setOverride} version={version} />
      {platform.available && (
        <>
          <DownloadAfterInstall platform={platform} />
          <DownloadVerify platform={platform} version={version} />
        </>
      )}
    </>
  );
};
