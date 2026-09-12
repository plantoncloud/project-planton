import { Metadata } from 'next';
import { Box } from '@mui/material';
import { LandingHero, LandingCapabilities, LandingCTA } from '@/components/product/desktop';
import { DESKTOP_LANDING_PATH } from '@/data/desktop-download';
import { POSITIONING } from '@/data/positioning';

const description = `${POSITIONING.desktop.line} The whole Planton platform runs on your laptop and deploys to your own cloud with the logins already on your machine. No account. Free for individuals, including commercial use.`;

export const metadata: Metadata = {
  title: 'Planton Desktop | Planton',
  description,
  alternates: { canonical: `https://planton.ai${DESKTOP_LANDING_PATH}` },
  openGraph: {
    title: 'Planton Desktop: the whole platform on your laptop, free',
    description,
    url: `https://planton.ai${DESKTOP_LANDING_PATH}`,
    images: [{ url: '/_site/images/og/desktop.png', width: 1200, height: 630, alt: 'Planton Desktop' }],
  },
  twitter: { card: 'summary_large_image' },
};

export default function DesktopPage() {
  return (
    <Box>
      <LandingHero />
      <LandingCapabilities />
      <LandingCTA />
    </Box>
  );
}
