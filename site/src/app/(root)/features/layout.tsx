'use client';

import NextLink from 'next/link';
import { usePathname } from 'next/navigation';
import { Box, Link, Stack, styled } from '@mui/material';
import './styles.css';

const Container = styled('div')({});

const paths: Record<string, string> = {
  Product: '/features',
  'Infra Hub': '/features/infra-hub',
  'Service Hub': '/features/service-hub',
  'Cloud Catalog': '/features/cloud-catalog',
  Runner: '/features/runner',
  Security: '/features/security',
  'Agent Fleet': '/features/agent-fleet',
  CLI: '/features/cli',
  'Planton Desktop': '/features/desktop',
  'Open Source': '/features/open-source',
};

// A tab is active on its own path and on any page nested under it (the
// desktop's install page lives under /features/desktop). The index tab is
// exact only; otherwise it would light on every product page.
const isActiveTab = (pathname: string, href: string) =>
  pathname === href || (href !== '/features' && pathname.startsWith(`${href}/`));

export default function Layout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <Container>
      <Stack className="hidden md:flex w-full h-[48px] gap-6 sticky top-[70px] left-0 z-10 flex-row items-center py-2 px-[32px] bg-[#0a0a0a]/90 backdrop-blur-md border-b border-[#2a2a2a]/30 overflow-x-auto">
        {Object.entries(paths).map(([key, value]) => (
          <Link
            component={NextLink}
            key={key}
            href={value}
            className={`text-sm font-medium whitespace-nowrap ${
              isActiveTab(pathname, value) ? 'text-white' : 'text-[#666] hover:text-[#a0a0a0]'
            } transition-colors`}
          >
            {key}
          </Link>
        ))}
      </Stack>
      <Box className="h-[100%]">{children}</Box>
    </Container>
  );
}
