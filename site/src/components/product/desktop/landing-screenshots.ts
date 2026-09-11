/**
 * The desktop landing page's screenshots, by the scene each one shows.
 *
 * Every image is a real capture of the app -- its own window chrome, taken
 * at 1440x900 on a Mac -- processed through the site's image pipeline and
 * served from the assets bucket under `landing/` (the media rule's path for
 * landing assets). A scene whose capture does not exist yet is `null`, and
 * the section renders without an image: a real frame or nothing, never a
 * placeholder pretending to be the product.
 *
 * Adding a screenshot: drop the reviewed PNG in content/assets/_inbox/, run
 * `make process-images CONTEXT=landing DESC=<scene>`, `make sync-assets`,
 * then paste the resulting URL here. Review every frame for identifiers
 * (account ids, emails, the workspace name, hostnames) before it leaves the
 * machine; the bucket is public.
 */

export interface LandingScreenshot {
  src: string;
  alt: string;
  /** Capture dimensions; the image scales to its container. */
  width: number;
  height: number;
}

export type LandingScene = 'home' | 'chooser' | 'studio';

export const LANDING_SCREENSHOTS: Record<LandingScene, LandingScreenshot | null> = {
  /** The local instance's home: one prompt, "What do you want to build?" */
  home: null,
  /** The instance chooser: this computer, planton.ai, or a self-hosted deployment. */
  chooser: null,
  /** Planton Studio's canvas with the cost odometer and the Runner Policy panel. */
  studio: null,
};
