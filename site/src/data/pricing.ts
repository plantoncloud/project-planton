/**
 * THE single source of pricing truth for the website.
 *
 * Every number a visitor can see -- plan cards, the landing page, the value
 * matrix, FAQ copy -- must read from here. The pricing page and the landing
 * page once disagreed publicly ($19 vs $20 per seat, and a 10x drift on the
 * automation-minute rate) precisely because these constants lived inside one
 * UI component while other components re-authored them. The law applies to
 * prose too: a sentence that names a price references a constant.
 *
 * Prices are market-aware: each market carries its own SET prices -- an
 * India price is a real India number, never an FX conversion of the US one.
 * Adding a market is one entry in MARKETS; the display layer follows.
 */

export type MarketId = 'us' | 'in';

export interface EnterpriseTierDisplay {
  name: string;
  /** Market-native display string (e.g. "$60K", "₹30 lakh"). */
  perYear: string;
  seatCeiling: number;
  support: string;
}

export interface LicenseSizeDisplay {
  /** Exact yearly print (e.g. "$3,000", "₹2,00,000") -- the one precise appearance near a charge surface. */
  perYear: string;
  /** Compact form for prose ranges (e.g. "$3K", "₹2L"). */
  perYearCompact: string;
  /**
   * The plan card's headline price, in the market's own idiom: India reads
   * lakh at a glance ("₹2L", matching the enterprise band's "₹30L"), while
   * "$3,000" is already the US price-tag form. The exact figure still
   * prints once in the card's sub line -- headline ergonomics never remove
   * the precise number from the page.
   */
  perYearCard: string;
  seatCeiling: number;
}

export interface Market {
  id: MarketId;
  /** Currency code shown on the market control. */
  currency: string;
  /** Region name shown beside the control. */
  region: string;
  symbol: string;
  teamSeatMonthly: number;
  /** Per seat per YEAR (two months free vs monthly). */
  teamSeatAnnual: number;
  /** Smallest purchasable AI credit pack. */
  creditPackStart: number;
  /** Self-hosted license sizes, smallest first (order mirrors SELF_HOSTED_LICENSE_SEAT_CEILINGS). */
  licenses: LicenseSizeDisplay[];
  enterprise: EnterpriseTierDisplay[];
}

export const MARKETS: Record<MarketId, Market> = {
  us: {
    id: 'us',
    currency: 'USD',
    region: 'United States',
    symbol: '$',
    teamSeatMonthly: 20,
    teamSeatAnnual: 192,
    creditPackStart: 10,
    licenses: [
      { perYear: '$3,000', perYearCompact: '$3K', perYearCard: '$3,000', seatCeiling: 10 },
      { perYear: '$8,000', perYearCompact: '$8K', perYearCard: '$8,000', seatCeiling: 25 },
    ],
    enterprise: [
      {
        name: 'Enterprise Standard',
        perYear: '$60K',
        seatCeiling: 100,
        support: 'Named business-hours support',
      },
      {
        name: 'Enterprise Plus',
        perYear: '$120K',
        seatCeiling: 250,
        support: '24×7 support with SLA',
      },
    ],
  },
  in: {
    id: 'in',
    currency: 'INR',
    region: 'India',
    symbol: '₹',
    teamSeatMonthly: 999,
    teamSeatAnnual: 9990,
    creditPackStart: 499,
    licenses: [
      { perYear: '₹2,00,000', perYearCompact: '₹2L', perYearCard: '₹2L', seatCeiling: 10 },
      { perYear: '₹5,00,000', perYearCompact: '₹5L', perYearCard: '₹5L', seatCeiling: 25 },
    ],
    enterprise: [
      {
        name: 'Enterprise Standard',
        perYear: '₹30L',
        seatCeiling: 100,
        support: 'Named business-hours support',
      },
      {
        name: 'Enterprise Plus',
        perYear: '₹60L',
        seatCeiling: 250,
        support: '24×7 support with SLA',
      },
    ],
  },
};

export const DEFAULT_MARKET_ID: MarketId = 'us';

/**
 * Best-effort market detection from the browser's two locality signals: the
 * locale's region tag (e.g. "en-IN") and the IANA timezone (India has a
 * single timezone, so Asia/Kolkata is a strong signal; Asia/Calcutta is its
 * legacy alias). Either signal saying India means India.
 *
 * The site is a static export -- no server ever sees the request, so there
 * is no geo-IP to read and detection is client-side by design. The detected
 * market is a GATE, not just a default (founder direction 2026-08-20): INR
 * prices render only for visitors detected in India; everyone else sees USD
 * with no market control at all. Display-only either way -- the actual
 * charge is always fixed server-side from the catalog's regional prices at
 * checkout.
 */
export const detectMarket = (locale?: string, timeZone?: string): MarketId => {
  if (timeZone === 'Asia/Kolkata' || timeZone === 'Asia/Calcutta') return 'in';
  if (!locale) return DEFAULT_MARKET_ID;
  try {
    const region = new Intl.Locale(locale).region;
    return region === 'IN' ? 'in' : DEFAULT_MARKET_ID;
  } catch {
    return DEFAULT_MARKET_ID;
  }
};

// The free tier's ONE bound (founder-decided 2026-08-13): seats. Everything
// else -- environments, cloud account connections, resources, services,
// automation minutes -- is deliberately unlimited on every plan (the meters
// competitors cap; environments cost the customer's cloud, not ours). The
// free tier never asks for a card and never bills -- at its cap it pauses
// new invites; nothing running is touched. A future tightening for NEW
// organizations would be a catalog edit that grandfathers existing ones.
export const FREE_TIER_SEATS = 3;

/**
 * Self-hosted community edition seat cap. Decided 2026-08-17 (it briefly
 * shipped as "unlimited", which made the paid licenses unsellable — a guest
 * read it live and asked "why would I even pay you?"). The sixth member
 * is the structural walk into the licensed sizes.
 */
export const COMMUNITY_SEAT_LIMIT = 5;

// Self-hosted license structure (market-invariant): the two sizes differ by
// seat ceiling only — identical features. Prices live in each MARKETS entry
// (`licenses`), set per market like every other number here. The community
// edition underneath stays free forever.
export const SELF_HOSTED_LICENSE_SEAT_CEILINGS = [10, 25] as const;

// Free full-experience evaluation on your own cluster: every capability
// unlocked for this many days; expiry steps down gently, never bricks.
export const EVALUATION_DAYS = 30;

// Where a self-hosted license purchase starts (the console's public
// buy page; no account required).
export const BUY_LICENSE_URL = 'https://planton.ai/license/buy';

// Where the free evaluation key is claimed: the same public page, its
// evaluation section (email in, key by email, once per address).
export const EVALUATION_URL = `${BUY_LICENSE_URL}#evaluation`;

// The self-serve ceiling: below this seat count nobody needs to talk to
// sales -- both license sizes are card-and-email purchases.
export const SELF_SERVE_SEAT_CEILING = 25;

