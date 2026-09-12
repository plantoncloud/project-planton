'use client';

import { useSyncExternalStore } from 'react';
import { detectDesktopPlatform, type DesktopPlatformId } from '@/data/desktop-download';

// The browser's platform is only readable in the browser. A static export
// renders the install page once with no browser at all (the server snapshot),
// and the real answer arrives on mount -- an external store read, not state
// set from an effect, so the first client render and the prerendered HTML
// agree and React never has to repair a mismatch. Same idiom as the
// deep-link bridge's fragment read and the shell's signed-in cookie.
//
// The snapshot is never subscribed to a change: a browser does not change
// operating system while a page is open.
const subscribeToNothing = () => () => {};

interface NavigatorWithClientHints extends Navigator {
  userAgentData?: { platform?: string };
}

function readPlatform(): DesktopPlatformId | null {
  const nav = navigator as NavigatorWithClientHints;
  return detectDesktopPlatform(nav.userAgent, nav.userAgentData?.platform);
}

/**
 * `undefined` while prerendering (platform not yet known), then the detected
 * platform id, or `null` for a device with no desktop build (a phone).
 */
export function useDetectedPlatform(): DesktopPlatformId | null | undefined {
  return useSyncExternalStore(subscribeToNothing, readPlatform, () => undefined);
}
