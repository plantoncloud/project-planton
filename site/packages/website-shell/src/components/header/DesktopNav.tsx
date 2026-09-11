'use client';

import Link from 'next/link';
import { Stack, Typography } from '@mui/material';
import {
  Hub as InfraHubIcon,
  RocketLaunch as ServiceHubIcon,
  SyncAlt as RunnerIcon,
  Shield as SecurityIcon,
  Psychology as AgentFleetIcon,
  Terminal as CliIcon,
  Code as OpenSourceIcon,
  Laptop as DesktopAppIcon,
  Assignment as CatalogIcon,
  MenuBook as DocsIcon,
  School as TutorialsIcon,
  Article as BlogIcon,
  NewReleases as ChangelogIcon,
  Explore as TourIcon,
  PlayCircle as DemoIcon,
} from '@mui/icons-material';
import { WebsiteLogo } from '../WebsiteLogo';
import { MegaMenu } from './MegaMenu';
import {
  menuProduct,
  menuExplorer,
  menuByUseCases,
  menuBySize,
  menuByRole,
  menuResources,
} from '../../data/navigation';

const iconSx = { fontSize: { xs: 16, md: 24 } } as const;

const productIcons: Record<string, React.ReactNode> = {
  // Keyed by the menu label exactly as navigation.ts spells it; a key that
  // drifts from the label renders the item without its icon.
  'Infra Hub': <InfraHubIcon sx={iconSx} />,
  'Service Hub': <ServiceHubIcon sx={iconSx} />,
  'Cloud Catalog': <CatalogIcon sx={iconSx} />,
  Runner: <RunnerIcon sx={iconSx} />,
  Security: <SecurityIcon sx={iconSx} />,
  'Agent Fleet': <AgentFleetIcon sx={iconSx} />,
  CLI: <CliIcon sx={iconSx} />,
  'Planton Desktop': <DesktopAppIcon sx={iconSx} />,
  'Open Source': <OpenSourceIcon sx={iconSx} />,
};

const resourceIcons: Record<string, React.ReactNode> = {
  Docs: <DocsIcon sx={iconSx} />,
  Tutorials: <TutorialsIcon sx={iconSx} />,
  Blog: <BlogIcon sx={iconSx} />,
  Changelog: <ChangelogIcon sx={iconSx} />,
  Tour: <TourIcon sx={iconSx} />,
  Demo: <DemoIcon sx={iconSx} />,
};

const withIcons = (items: typeof menuProduct, icons: Record<string, React.ReactNode>) =>
  items.map((item) => ({ ...item, icon: icons[item.label] ?? item.icon }));

export function DesktopNav() {
  return (
    <Stack
      direction="row"
      sx={{
        display: { xs: 'none', md: 'flex' },
        alignItems: 'center',
        gap: 3,
        fontSize: '0.875rem',
      }}
    >
      <WebsiteLogo />
      <MegaMenu
        title="Product"
        leftMenu={[{ items: withIcons(menuProduct, productIcons) }]}
        rightMenu={[{ title: 'Explore', items: menuExplorer }]}
        leftWidth={320}
      />
      <MegaMenu
        title="Solutions"
        leftMenu={[
          { title: 'By Use Case', items: menuByUseCases },
          { title: 'By Size', items: menuBySize },
        ]}
        rightMenu={[{ title: 'By Role', items: menuByRole }]}
        footerMenu={{ label: 'View all Solutions', href: '/solutions' }}
      />
      <MegaMenu
        title="Resources"
        leftMenu={[{ items: withIcons(menuResources, resourceIcons) }]}
        leftWidth={300}
      />
      <Link href="/pricing" style={{ textDecoration: 'none', color: 'inherit' }}>
        <Typography
          sx={{
            fontSize: '0.875rem',
            fontWeight: 500,
            color: 'grey.100',
            transition: 'color 150ms ease',
            '&:hover': { color: '#fff' },
          }}
        >
          Pricing
        </Typography>
      </Link>
    </Stack>
  );
}
