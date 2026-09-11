/**
 * THE single source of truth for where Planton Desktop is downloaded from.
 *
 * Every installer link on the site -- the install page, the landing page's
 * download button, the header link -- reads from here, the same law
 * src/data/pricing.ts applies to prices and src/data/positioning.ts to what
 * Planton is. A download URL typed anywhere else is a future dead link.
 *
 * The URLs point at the `latest` channel aliases on the downloads CDN. The
 * desktop release pipeline republishes these aliases on every stable release
 * (stripping the version from the per-version filename), so they are
 * version-free and always serve the current installer. That is what lets a
 * static site link to "the latest build" without knowing a version number:
 * the site never has to be rebuilt for a release.
 *
 * What the CDN also publishes, for the pieces that DO want a version:
 *   desktop/latest/version.txt        -> "v0.0.56" (plain text)
 *   desktop/<tag>/checksums.txt       -> the release's integrity contract
 * scripts/generate-desktop-release.mjs reads the pointer at build time and
 * writes src/generated/desktop-release.json; the page renders the version
 * badge and the checksums link only when that file carries a version.
 *
 * Per-platform `available` is a deliberate hand switch, not a computed fact:
 * it is flipped off when a published installer is known to be wrong (the
 * Windows alias served a cache-warm build stamped 0.0.0-warm on 2026-09-11)
 * and back on once a release has proven the alias -- download it, read its
 * version resource, then flip. The site must never offer a link it knows is
 * broken.
 */

export type DesktopPlatformId = 'macos' | 'windows' | 'linux';

export interface DesktopArtifact {
  /** Button label, Title Case ("Download for macOS", "Download .deb"). */
  label: string;
  href: string;
  /** Matches the artifact key the build-time release file reports sizes under. */
  key: 'macos-dmg' | 'windows-setup-exe' | 'linux-appimage' | 'linux-deb';
  /** The one artifact a platform card leads with; the rest render as secondary. */
  primary?: boolean;
}

export interface DesktopPlatform {
  id: DesktopPlatformId;
  /** Human name as it appears in "Download for {name}". */
  name: string;
  /** Renders the platform card at all. See the header comment before flipping. */
  available: boolean;
  artifacts: readonly DesktopArtifact[];
  /** The one-phrase floor a sentence can name ("macOS 12 or newer"). */
  minimum: string;
  /** Short facts under the button: architecture, minimum OS, package form. */
  facts: readonly string[];
  /**
   * What happens after the download finishes, as steps. A step that is a
   * command a person types carries it separately, so the page can render it
   * in the code register with a copy affordance instead of as prose.
   */
  installSteps: readonly { text: string; command?: string }[];
  /**
   * How to verify the downloaded file on this platform. The first line is
   * always the hash to compare against the release's checksums.txt; macOS
   * adds the Gatekeeper and notarization checks the signed build passes.
   */
  verifyCommands: readonly string[];
}

/**
 * Where the desktop pages live on the website. Both sit under /features
 * because the edge currently routes the short paths (/desktop, /download) to
 * the console; those short paths are the intended final homes, and moving
 * back is a folder rename plus these two constants. Every site component
 * reads them from here; the shell package's navigation keeps literal hrefs
 * like every other entry because it cannot import from src/.
 */
export const DESKTOP_LANDING_PATH = '/features/desktop';
export const DESKTOP_DOWNLOAD_PATH = '/features/desktop/download';

export const DOWNLOADS_BASE = 'https://downloads.planton.app/desktop';

/** The version-free alias directory the release pipeline republishes on every stable release. */
export const DOWNLOADS_LATEST = `${DOWNLOADS_BASE}/latest`;

/** The Homebrew cask. It declares the `planton` CLI formula as a dependency, so one command installs both. */
export const DESKTOP_BREW_COMMAND = 'brew install plantonhq/tap/planton-desktop';
export const DESKTOP_BREW_UPGRADE_COMMAND = 'brew upgrade planton-desktop';

/** The one command that installs the Planton skills into every coding agent on the machine. */
export const AGENT_SKILLS_INSTALL_COMMAND = 'npx skills add plantonhq/skills';

export const DESKTOP_PLATFORMS: readonly DesktopPlatform[] = [
  {
    id: 'macos',
    name: 'macOS',
    available: true,
    minimum: 'macOS 12 or newer',
    artifacts: [
      {
        label: 'Download for macOS',
        href: `${DOWNLOADS_LATEST}/planton-desktop-universal-macos.dmg`,
        key: 'macos-dmg',
        primary: true,
      },
    ],
    facts: ['Universal: Apple Silicon and Intel', 'macOS 12 or newer', 'Signed and notarized by Apple'],
    installSteps: [
      { text: 'Open the disk image and drag Planton to Applications. The app is signed and notarized, so it opens without a warning.' },
    ],
    verifyCommands: [
      'shasum -a 256 ~/Downloads/planton-desktop-universal-macos.dmg',
      'spctl --assess --type open --context context:primary-signature -v ~/Downloads/planton-desktop-universal-macos.dmg',
      'xcrun stapler validate /Applications/Planton.app',
    ],
  },
  {
    id: 'windows',
    name: 'Windows',
    minimum: 'Windows 10 or newer',
    // Off until a release republishes a real installer under the alias; the
    // one there now is a cache-warm build (see the header comment).
    available: false,
    artifacts: [
      {
        label: 'Download for Windows',
        href: `${DOWNLOADS_LATEST}/planton-desktop-windows-x64-setup.exe`,
        key: 'windows-setup-exe',
        primary: true,
      },
    ],
    facts: ['Installer for x64', 'Windows 10 or newer'],
    installSteps: [
      {
        text: 'Windows SmartScreen may show "Windows protected your PC" because the installer is not yet code-signed. Choose More info, then Run anyway. Updates are signed by Planton and verified by the app before they apply.',
      },
    ],
    verifyCommands: ['certutil -hashfile planton-desktop-windows-x64-setup.exe SHA256'],
  },
  {
    id: 'linux',
    name: 'Linux',
    available: true,
    minimum: 'glibc 2.35 or newer',
    artifacts: [
      {
        label: 'Download AppImage',
        href: `${DOWNLOADS_LATEST}/planton-desktop-linux-amd64.AppImage`,
        key: 'linux-appimage',
        primary: true,
      },
      {
        label: 'Download .deb',
        href: `${DOWNLOADS_LATEST}/planton-desktop-linux-amd64.deb`,
        key: 'linux-deb',
      },
    ],
    facts: [
      'x86_64 only; arm64 builds are not published yet',
      'glibc 2.35 or newer: Ubuntu 22.04, Debian 12, Fedora 38, RHEL 9, or newer',
    ],
    installSteps: [
      { text: 'For the AppImage, make it executable and run it.', command: 'chmod +x planton-desktop-linux-amd64.AppImage' },
      { text: 'On Debian and Ubuntu, install the .deb with your package manager.', command: 'sudo apt install ./planton-desktop-linux-amd64.deb' },
    ],
    verifyCommands: ['sha256sum planton-desktop-linux-amd64.AppImage', 'sha256sum planton-desktop-linux-amd64.deb'],
  },
];

export const DESKTOP_PLATFORM_BY_ID: Readonly<Record<DesktopPlatformId, DesktopPlatform>> = Object.fromEntries(
  DESKTOP_PLATFORMS.map((p) => [p.id, p]),
) as Record<DesktopPlatformId, DesktopPlatform>;

/** Where a release's checksums live; the version carries its `v` prefix exactly as the pointer prints it. */
export const desktopChecksumsUrl = (version: string): string => `${DOWNLOADS_BASE}/${version}/checksums.txt`;

/**
 * Best-effort platform detection from the two signals a browser offers: the
 * User-Agent Client Hints platform (Chromium: "macOS", "Windows", "Linux")
 * and the user-agent string everyone still sends. Client Hints win when
 * present because the UA string is frozen and lies on purpose.
 *
 * The site is a static export -- no server ever sees the request -- so
 * detection is client-side by design, exactly like pricing's detectMarket.
 * The result only PROMOTES a platform's card; every platform stays listed and
 * a manual switch is always shown, because a person downloading for another
 * machine is common. iPhones and iPads return null: there is no desktop app
 * for them, and promoting macOS would be a guess dressed as a fact.
 */
export const detectDesktopPlatform = (userAgent: string, uaPlatform?: string | null): DesktopPlatformId | null => {
  const hint = (uaPlatform ?? '').toLowerCase();
  if (hint.includes('mac')) return 'macos';
  if (hint.includes('win')) return 'windows';
  if (hint.includes('linux') || hint.includes('chrome os') || hint.includes('chromeos')) return 'linux';

  const ua = userAgent.toLowerCase();
  if (/iphone|ipad|ipod|android/.test(ua)) return null;
  if (ua.includes('mac os') || ua.includes('macintosh')) return 'macos';
  if (ua.includes('windows')) return 'windows';
  if (ua.includes('linux') || ua.includes('x11') || ua.includes('cros')) return 'linux';
  return null;
};
