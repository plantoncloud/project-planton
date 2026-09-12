import type { ReactNode } from 'react';

export interface MenuItem {
  label: string;
  subLabel?: string;
  href: string;
  icon?: ReactNode;
}

export interface MenuSection {
  title?: string;
  items: MenuItem[];
}

export interface FooterGroup {
  id: string;
  title: string;
  items: { title: string; url: string }[];
}

// ---------------------------------------------------------------------------
// Header — Product mega-menu
// ---------------------------------------------------------------------------

export const menuProduct: MenuItem[] = [
  { label: 'Infra Hub', subLabel: 'Deploy any infrastructure, any cloud', href: '/features/infra-hub' },
  { label: 'Service Hub', subLabel: 'Ship code from Git to production', href: '/features/service-hub' },
  { label: 'Cloud Catalog', subLabel: 'Browse and deploy infrastructure modules', href: '/features/cloud-catalog' },
  { label: 'Runner', subLabel: 'Execute in your cloud, orchestrate from ours', href: '/features/runner' },
  { label: 'Security', subLabel: 'Secrets, IAM, and audit - built into every layer', href: '/features/security' },
  { label: 'Agent Fleet', subLabel: 'AI agents, purpose-built for infrastructure', href: '/features/agent-fleet' },
  { label: 'CLI', subLabel: 'Command your cloud from the terminal', href: '/features/cli' },
  { label: 'Desktop', subLabel: 'The whole platform on your laptop, free forever', href: '/features/desktop' },
  { label: 'Open Source', subLabel: 'The open-source core of Planton', href: '/features/open-source' },
];

export const menuExplorer: MenuItem[] = [
  { label: 'All Product', href: '/features' },
  { label: 'Documentation', href: '/docs' },
  { label: 'Tutorials', href: '/tutorials' },
  { label: 'Blog', href: '/blog' },
  { label: 'Changelog', href: '/changelog' },
];

// ---------------------------------------------------------------------------
// Header — Solutions mega-menu
// ---------------------------------------------------------------------------

export const menuByUseCases: MenuItem[] = [
  { label: 'Internal Developer Platform', href: '/solutions/by-use-case/internal-developer-platform' },
  { label: 'Multi-Cloud', href: '/solutions/by-use-case/multi-cloud' },
  { label: 'Self-Hosted DevOps', href: '/solutions/by-use-case/self-hosted-devops' },
];

export const menuBySize: MenuItem[] = [
  { label: 'Startups', href: '/solutions/by-size/startups' },
  { label: 'Growing Teams', href: '/solutions/by-size/growing-teams' },
  { label: 'Enterprises', href: '/solutions/by-size/enterprises' },
];

export const menuByRole: MenuItem[] = [
  { label: 'Developer', href: '/solutions/by-role/developers' },
  { label: 'Platform Engineer', href: '/solutions/by-role/platform-engineers' },
  { label: 'Startup Founder', href: '/solutions/by-role/startup-founders' },
  { label: 'Engineering Leader', href: '/solutions/by-role/engineering-leader' },
];

// ---------------------------------------------------------------------------
// Header — Resources mega-menu
// ---------------------------------------------------------------------------

export const menuResources: MenuItem[] = [
  { label: 'Docs', subLabel: 'Guides, references, and platform overview', href: '/docs' },
  { label: 'Tutorials', subLabel: 'Step-by-step deployment walkthroughs', href: '/tutorials' },
  { label: 'Blog', subLabel: 'Product updates and engineering insights', href: '/blog' },
  { label: 'Changelog', subLabel: 'What shipped in every release', href: '/changelog' },
  { label: 'Tour', subLabel: 'Interactive walkthrough of the console', href: '/tour' },
  { label: 'Demo', subLabel: 'See Planton in action', href: '/demo' },
];

// ---------------------------------------------------------------------------
// Footer link groups — canonical URLs (aligned with header)
// ---------------------------------------------------------------------------

export const footerGroups: FooterGroup[] = [
  {
    title: 'Product',
    id: 'product',
    items: [
      { title: 'Infra Hub', url: '/features/infra-hub' },
      { title: 'Service Hub', url: '/features/service-hub' },
      { title: 'Cloud Catalog', url: '/features/cloud-catalog' },
      { title: 'Runner', url: '/features/runner' },
      { title: 'Security', url: '/features/security' },
      { title: 'Agent Fleet', url: '/features/agent-fleet' },
      { title: 'CLI', url: '/features/cli' },
    ],
  },
  {
    title: 'Open Source',
    id: 'open_source',
    items: [
      { title: 'Planton open source', url: '/features/open-source' },
      { title: 'Infra Charts', url: 'https://github.com/plantonhq/planton/tree/main/charts' },
    ],
  },
  {
    title: 'GET STARTED',
    id: 'get_started',
    items: [
      { title: 'Download Planton Desktop', url: '/features/desktop/download' },
      { title: 'Sign Up', url: '/signup' },
      { title: 'Pricing', url: '/pricing' },
      { title: 'Book a Demo', url: '/book-demo' },
    ],
  },
  {
    title: 'Resources',
    id: 'resources',
    items: [
      { title: 'Documentation', url: '/docs' },
      { title: 'Tutorials', url: '/tutorials' },
      { title: 'Blog', url: '/blog' },
      { title: 'Changelog', url: '/changelog' },
    ],
  },
  {
    title: 'Explore',
    id: 'explore',
    items: [
      { title: 'All Product', url: '/features' },
      { title: 'Solutions', url: '/solutions' },
      { title: 'Tour', url: '/tour' },
    ],
  },
];

export const footerTermsLinks = [
  { title: 'Status', url: '/' },
  { title: 'Privacy', url: '/privacy' },
  { title: 'Terms', url: '/terms' },
  { title: 'Refunds', url: '/refund-policy' },
];

export const DISCORD_URL = 'https://discord.gg/pwcSapdQAp';
