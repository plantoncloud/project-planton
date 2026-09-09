/**
 * The email handoff: how a console that already knows who is buying tells
 * the store, without the address ever leaving the person's browser.
 *
 * A self-hosted console's License page opens the pricing page with the
 * viewer's email in the URL FRAGMENT (`/pricing#email=...`). A fragment is
 * never sent to a server -- no CDN log, no origin log -- and the capture
 * script below runs synchronously in the document head, so the address is
 * moved into the tab's session storage and stripped from the address bar
 * before any analytics script has loaded to record a page URL. The pricing
 * page then reads it from storage and decorates its links into the console's
 * public buy page, which accepts `?email=` and opens its form prefilled.
 *
 * Two constants shared by the script and the hook, so the storage contract
 * lives in exactly one place. Only a value shaped like an email is kept.
 */

/** The fragment key the console writes: `#email=<encoded address>`. */
export const HANDOFF_EMAIL_FRAGMENT_KEY = 'email';

/** Where the captured address lives for the rest of the tab's life. */
export const HANDOFF_EMAIL_STORAGE_KEY = 'planton.handoff.email';

/** The query key the console's buy page reads. */
export const HANDOFF_EMAIL_QUERY_KEY = 'email';

/** The same shape test the buy page applies before it accepts an address. */
const EMAIL_SHAPE = /.+@.+\..+/;

/** Longest address the handoff carries (the wire limit for a mailbox address). */
const MAX_EMAIL_LENGTH = 254;

export const looksLikeEmail = (value: string): boolean =>
  value.length > 0 && value.length <= MAX_EMAIL_LENGTH && EMAIL_SHAPE.test(value);

/**
 * The inline script rendered in the document head. Deliberately plain,
 * self-contained JavaScript: it must run before hydration and before any
 * third-party script, and it must never throw into the page.
 */
export const HANDOFF_CAPTURE_SCRIPT =
  'try{' +
  `var m=/^#${HANDOFF_EMAIL_FRAGMENT_KEY}=([^&]+)$/.exec(window.location.hash);` +
  'if(m){' +
  'var e=decodeURIComponent(m[1]);' +
  `if(e.length<=${MAX_EMAIL_LENGTH}&&/${EMAIL_SHAPE.source}/.test(e)){` +
  `window.sessionStorage.setItem(${JSON.stringify(HANDOFF_EMAIL_STORAGE_KEY)},e)}` +
  'window.history.replaceState(null,"",window.location.pathname+window.location.search)' +
  '}' +
  '}catch(err){}';

/**
 * Decorate a store URL with the handed-off address as `?email=`, keeping any
 * fragment the URL already carries (the evaluation link targets a section).
 * A value that is not shaped like an email leaves the URL untouched.
 */
export const withHandoffEmail = (url: string, email: string): string => {
  const address = email.trim();
  if (!looksLikeEmail(address)) return url;
  const hashAt = url.indexOf('#');
  const base = hashAt === -1 ? url : url.slice(0, hashAt);
  const fragment = hashAt === -1 ? '' : url.slice(hashAt);
  const separator = base.includes('?') ? '&' : '?';
  return `${base}${separator}${HANDOFF_EMAIL_QUERY_KEY}=${encodeURIComponent(address)}${fragment}`;
};
