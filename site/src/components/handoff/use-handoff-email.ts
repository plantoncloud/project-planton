'use client';

import { useSyncExternalStore } from 'react';
import { HANDOFF_EMAIL_STORAGE_KEY, looksLikeEmail } from './handoff-email';

// The capture script has already run (synchronously, in the head) by the
// time any component renders, and nothing writes the key afterwards, so the
// store never changes within a page's lifetime: a no-op subscription and a
// direct read keep hydration honest (server snapshot = nothing handed off),
// the same discipline the market hook uses for the browser's locality.
const emptySubscribe = () => () => {};

const clientSnapshot = (): string => {
  try {
    const stored = window.sessionStorage.getItem(HANDOFF_EMAIL_STORAGE_KEY) ?? '';
    return looksLikeEmail(stored) ? stored : '';
  } catch {
    // Storage can be unavailable (privacy modes); the handoff is a
    // convenience, and its absence is simply an empty form.
    return '';
  }
};

const serverSnapshot = (): string => '';

/** The address a console handed to this tab, or '' when nobody did. */
export const useHandoffEmail = (): string =>
  useSyncExternalStore(emptySubscribe, clientSnapshot, serverSnapshot);
