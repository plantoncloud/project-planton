/**
 * THE single source of positioning truth for the website.
 *
 * The product positioning follows a three-level rule, and this file exists
 * so the rule cannot drift the way the last one did (the "Vercel for
 * backend" line was coined for Service Hub, escaped its scope, and spent
 * six months describing the whole product):
 *
 *   Level 1 — Planton, the umbrella. Never uses an analogy.
 *   Level 2 — each hub gets exactly one analogy, and the analogy never
 *             escapes its hub. Infra Hub is "Cursor for Cloud
 *             Infrastructure"; Service Hub is "Vercel for Backend, In
 *             Your Own Cloud".
 *
 * Any sentence on the site that states what Planton IS must read from
 * here — the same law src/data/pricing.ts applies to prices.
 *
 * Vocabulary: "Infra Chart" is the product noun; "template" is the plain
 * word that explains it. Pair them on first mention, and never capitalize
 * "Template" as if it were a product name.
 */

export const POSITIONING = {
  /** Level 1 — the umbrella. No analogy, ever. */
  umbrella: {
    tagline: 'The Self-Service Cloud Platform',
    sentence:
      'Planton turns your own cloud account into a self-service platform. AI designs the infrastructure, verifies the cost and permissions before anything is created, and publishes it as templates your whole team can deploy. Your services then ship onto that infrastructure straight from Git.',
    /**
     * The mission, in the founder's words (2026-09-11). Platforms that hide
     * the cloud trade control for convenience; Planton's bet is that you can
     * keep the account, the policy, the state, and the bill and still have
     * the convenience. It is the stance behind every page, and it is a
     * contrast with hidden-cloud platforms -- NOT the argument a page makes
     * to an engineer who already has a coding agent and a cloud CLI, because
     * that engineer already has both convenience and control. For them the
     * argument is what Planton adds to the agent (see `desktop`).
     */
    mission: 'Convenience without losing control.',
  },

  /**
   * The desktop distribution. Free for individuals, including commercial
   * use, because Planton makes its money when a team adopts it -- the page
   * says so as a business model, not a badge. The argument leads with what
   * Planton adds to the coding agent the reader already uses; control is the
   * second beat; competitors are described, never named; and the page
   * concedes that for a one-off bucket the agent alone wins.
   */
  desktop: {
    name: 'Planton Desktop',
    line: 'Your coding agent can already create cloud infrastructure. Planton makes it verifiable, recorded, and reusable — in your account, on your laptop, free.',
    whatItAdds: [
      'A typed catalog, not memory: schema lookups and validation run offline, so a wrong field fails before it touches your cloud.',
      'Verified before created: the monthly cost with its coverage stated, and the least-privilege permission policy derived from what is composed.',
      'A record, not a transcript: every deploy is a stack job with a live log and a revision history; state sits under a path you can list.',
      'Secrets the agent never reads: encrypted locally, key in your OS keychain, resolved on the runner at the moment of use.',
      'Push-to-deploy from your laptop: it watches GitHub, builds in a pod, deploys to your cloud, and writes the status back.',
      'What you built becomes a template: an Infra Chart redeploys into the next environment; agent commands do not compose.',
      'When you become a team, nothing is redone: the same manifests and model on planton.ai or your own cluster.',
    ],
    concession: 'For a one-off bucket, the agent alone wins. Planton earns its place on anything you will still be running in a month.',
  },

  /** Level 2 — one analogy per hub, scoped to that hub only. */
  infraHub: {
    name: 'Infra Hub',
    analogy: 'Cursor for Cloud Infrastructure',
    line: 'Describe what you need, watch it compose on a live canvas, see the cloud bill and the IAM policy before anything is created, deploy, and publish it as an Infra Chart — a template your team reuses.',
  },
  serviceHub: {
    name: 'Service Hub',
    analogy: 'Vercel for Backend, In Your Own Cloud',
    line: 'Connect a Git repository and every push becomes a running deployment — no pipeline YAML, no Dockerfile required, with results written back into GitHub checks and deployments.',
  },
} as const;
