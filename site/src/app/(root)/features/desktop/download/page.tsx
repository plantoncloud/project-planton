import { Metadata } from 'next';
import { Box } from '@mui/material';
import { DownloadPage as DownloadPageContent } from '@/components/product/desktop';
import { DESKTOP_DOWNLOAD_PATH, DESKTOP_PLATFORMS, DOWNLOADS_LATEST } from '@/data/desktop-download';
import { DESKTOP_RELEASE } from '@/data/desktop-release';

const description =
  'Download Planton Desktop for macOS, Windows, or Linux. The whole Planton platform runs on your laptop and deploys to your own cloud. Free for individuals, including commercial use. No account required.';

export const metadata: Metadata = {
  title: 'Download Planton Desktop | Planton',
  description,
  alternates: { canonical: `https://planton.ai${DESKTOP_DOWNLOAD_PATH}` },
  openGraph: {
    title: 'Download Planton Desktop',
    description,
    url: `https://planton.ai${DESKTOP_DOWNLOAD_PATH}`,
    images: [{ url: '/_site/images/og/download.png', width: 1200, height: 630, alt: 'Download Planton Desktop' }],
  },
  twitter: { card: 'summary_large_image' },
};

// Structured data so a search result for "planton desktop download" carries
// the platforms, the price (none), and the direct download URL. Built from the
// same data module as the buttons, so it cannot name a platform the page does
// not offer.
const softwareApplication = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareApplication',
  name: 'Planton Desktop',
  applicationCategory: 'DeveloperApplication',
  operatingSystem: DESKTOP_PLATFORMS.filter((p) => p.available)
    .map((p) => p.name)
    .join(', '),
  ...(DESKTOP_RELEASE.version ? { softwareVersion: DESKTOP_RELEASE.version.replace(/^v/, '') } : {}),
  downloadUrl: `${DOWNLOADS_LATEST}/`,
  offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
  url: `https://planton.ai${DESKTOP_DOWNLOAD_PATH}`,
  description,
};

export default function DownloadPage() {
  return (
    <Box>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(softwareApplication) }} />
      <DownloadPageContent />
    </Box>
  );
}
