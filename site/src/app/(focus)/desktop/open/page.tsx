'use client';

import { useEffect, useSyncExternalStore } from 'react';
import { Box, Typography } from '@mui/material';

// The one public page a Planton Desktop deep link can hide behind.
//
// Some places only accept http(s) links -- a GitHub commit status's Details
// button, for one -- while a run that happened on a developer's own laptop
// lives only in that laptop's Planton Desktop. This page bridges the two:
// the link carries the desktop's route in its fragment
// (https://planton.ai/desktop/open#/acme/service/storefront/runs/<id>), the
// fragment never reaches any server, and the page hands the route to the
// installed app through its planton-desktop:// URL scheme. Anyone without
// the app -- a reviewer on another machine -- reads what the link was
// instead of a dead end.
const DESKTOP_SCHEME = 'planton-desktop://';

function routeFromFragment(hash: string): string | null {
  const route = hash.startsWith('#') ? hash.slice(1) : hash;
  if (!route.startsWith('/') || route.startsWith('//')) return null;
  return route;
}

// The fragment is only readable in the browser; a static export renders this
// page once with no fragment (the server snapshot), and the browser reads the
// real one on mount -- an external store, not state set from an effect.
function subscribeToHash(onChange: () => void) {
  window.addEventListener('hashchange', onChange);
  return () => window.removeEventListener('hashchange', onChange);
}

function useFragmentRoute(): string | null | undefined {
  const hash = useSyncExternalStore(
    subscribeToHash,
    () => window.location.hash,
    () => undefined,
  );
  return hash === undefined ? undefined : routeFromFragment(hash);
}

export default function OpenInDesktopPage() {
  const route = useFragmentRoute();
  const state = route === null ? 'no-route' : 'opening';

  useEffect(() => {
    if (!route) return;
    // The scheme's path is the console's own path minus its leading slash.
    window.location.href = DESKTOP_SCHEME + route.slice(1);
  }, [route]);

  return (
    <Box className="min-h-screen flex items-center justify-center bg-[#0a0a0a] px-6">
      <Box className="max-w-xl text-center">
        <Typography component="h1" className="text-white text-2xl font-semibold mb-3">
          {state === 'opening' ? 'Opening in Planton Desktop…' : 'Nothing to open'}
        </Typography>
        {state === 'opening' ? (
          <>
            <Typography className="text-[#a0a0a0] text-base mb-6">
              This run happened on a developer&apos;s own laptop, in the Planton Desktop app that built it. If the
              app is installed on this machine, it is opening now.
            </Typography>
            <Typography className="text-[#666] text-sm">
              Nothing happened? Planton Desktop is not installed here, or the run belongs to someone else&apos;s
              laptop. The status you clicked already carries the outcome; the full run — its logs, its stages,
              its deployments — lives in that developer&apos;s desktop.{' '}
              <a href="/download" className="underline decoration-[#3a3a3a] hover:text-[#a0a0a0]">
                Get Planton Desktop
              </a>
              .
            </Typography>
            <Typography className="text-[#444] text-xs mt-6 font-mono break-all">{route ?? ''}</Typography>
          </>
        ) : (
          <Typography className="text-[#a0a0a0] text-base">
            This link carries no destination. A Planton Desktop link looks like{' '}
            <span className="font-mono text-[#ccc]">planton.ai/desktop/open#/your-org/service/…</span>.
          </Typography>
        )}
      </Box>
    </Box>
  );
}
