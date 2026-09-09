import type { Metadata } from 'next';
import { Inter } from 'next/font/google';
import './globals.css';
import { ThemeProvider } from '@/providers/theme';
import { GoogleAnalytics } from '@next/third-parties/google';
import { HANDOFF_CAPTURE_SCRIPT } from '@/components/handoff';

const inter = Inter({
  weight: ['300', '400', '500', '600', '700'],
  subsets: ['latin'],
  display: 'swap',
  variable: '--font-inter',
});

const siteDescription =
  'Planton is a DevOps automation platform that provides self-service infrastructure deployment across AWS, GCP, and Azure, combined with built-in CI/CD for backend services. No ops team required. No vendor lock-in.';

export const metadata: Metadata = {
  metadataBase: new URL('https://planton.ai'),
  applicationName: 'Planton',
  icons: { icon: '/favicon.ico' },
  title: 'Planton — Deploy Production Infrastructure in Minutes, Not Weeks',
  description: siteDescription,
  openGraph: {
    siteName: 'Planton',
    type: 'website',
    url: 'https://planton.ai',
    title: 'Planton — Deploy Production Infrastructure in Minutes, Not Weeks',
    description: siteDescription,
  },
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="h-screen">
      {process.env.NODE_ENV === 'production' && <GoogleAnalytics gaId="G-VWZNWQPEJ0" />}

      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: 'try{if(window.location.hostname==="planton.ai"&&window.location.pathname==="/"&&!window.location.search&&/planton_logged_in=/.test(document.cookie))window.location.replace("/dashboard")}catch(e){}',
          }}
        />
        {/* A console's email handoff (`#email=`) is captured into session
            storage and stripped from the address bar here, synchronously,
            before the analytics script below can record a page URL. */}
        <script dangerouslySetInnerHTML={{ __html: HANDOFF_CAPTURE_SCRIPT }} />
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Material+Symbols+Rounded:opsz,wght,FILL,GRAD@20..48,100..700,0..1,-50..200&display=optional"
        />
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              '@context': 'https://schema.org',
              '@type': 'WebApplication',
              name: 'Planton',
              url: 'https://planton.ai',
              description: siteDescription,
              applicationCategory: 'DeveloperApplication',
              operatingSystem: 'Web',
              offers: {
                '@type': 'Offer',
                price: '0',
                priceCurrency: 'USD',
              },
              publisher: {
                '@type': 'Organization',
                name: 'Planton Cloud, Inc.',
                url: 'https://planton.ai',
              },
            }),
          }}
        />
      </head>
      <body className={`${inter.variable} antialiased h-screen bg-cover bg-center bg-[#0a0a0a]`}>
        <ThemeProvider>
          {children}
        </ThemeProvider>
      </body>
    </html>
  );
}
