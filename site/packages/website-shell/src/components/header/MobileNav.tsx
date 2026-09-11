'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Divider, Stack, Typography } from '@mui/material';
import { DensityMedium, Close } from '@mui/icons-material';
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
import { ShellDrawer } from './styled';
import { MenuAccordion } from './MenuAccordion';
import { MegaMenuItem } from './MegaMenuItem';
import { MobileAuthButtons } from './AuthButtons';
import { MobileDownloadLink } from './DownloadLink';
import { DiscordButton } from '../shared/DiscordButton';
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

const dividerSx = { borderColor: '#232323' } as const;

export function MobileNav() {
  const [open, setOpen] = useState(false);
  const [expandedPanel, setExpandedPanel] = useState<string | false>(false);

  const handlePanelChange = (panel: string) => (_: React.SyntheticEvent, isExpanded: boolean) => {
    setExpandedPanel(isExpanded ? panel : false);
  };

  const toggleDrawer = () => setOpen((prev) => !prev);

  return (
    <>
      <DensityMedium
        fontSize="small"
        onClick={toggleDrawer}
        sx={{ position: 'relative', zIndex: 10, cursor: 'pointer' }}
      />

      <ShellDrawer open={open} onClose={toggleDrawer}>
        <Stack
          sx={{ gap: 3.5 }}
          onClick={(e) => {
            if ((e.target as HTMLElement).closest('a')) setOpen(false);
          }}
        >
          <Stack direction="row" sx={{ alignItems: 'center', gap: 4, justifyContent: 'space-between' }}>
            <WebsiteLogo />
            <Close onClick={toggleDrawer} sx={{ cursor: 'pointer' }} />
          </Stack>

          <Stack sx={{ gap: 4 }}>
            {/* Product */}
            <MenuAccordion
              expanded={expandedPanel === 'product'}
              title="Product"
              onChange={handlePanelChange('product')}
            >
              <Stack sx={{ gap: 3 }}>
                {withIcons(menuProduct, productIcons).map((item) => (
                  <MegaMenuItem key={item.label} {...item} />
                ))}
                <Divider sx={{ ...dividerSx, mt: -1.5 }} />
                <Stack sx={{ gap: 2 }}>
                  <Typography sx={{ fontSize: '0.875rem', fontWeight: 400 }}>Explore</Typography>
                  {menuExplorer.map((item) => (
                    <MegaMenuItem key={item.label} {...item} />
                  ))}
                  <Divider sx={{ ...dividerSx, mt: -0.5 }} />
                </Stack>
              </Stack>
            </MenuAccordion>

            {/* Solutions */}
            <MenuAccordion
              expanded={expandedPanel === 'solutions'}
              title="Solutions"
              onChange={handlePanelChange('solutions')}
            >
              <Stack sx={{ gap: 3 }}>
                <Stack sx={{ gap: 2 }}>
                  <Typography sx={{ fontSize: '0.875rem', fontWeight: 400 }}>By Use Case</Typography>
                  {menuByUseCases.map((item) => (
                    <MegaMenuItem key={item.label} {...item} />
                  ))}
                  <Divider sx={{ ...dividerSx, mt: -0.5 }} />
                </Stack>
                <Stack sx={{ gap: 2 }}>
                  <Typography sx={{ fontSize: '0.875rem', fontWeight: 400 }}>By Size</Typography>
                  {menuBySize.map((item) => (
                    <MegaMenuItem key={item.label} {...item} />
                  ))}
                  <Divider sx={{ ...dividerSx, mt: -0.5 }} />
                </Stack>
                <Stack sx={{ gap: 2 }}>
                  <Typography sx={{ fontSize: '0.875rem', fontWeight: 400 }}>By Role</Typography>
                  {menuByRole.map((item) => (
                    <MegaMenuItem key={item.label} {...item} />
                  ))}
                  <Divider sx={{ ...dividerSx, mt: -0.5 }} />
                </Stack>
              </Stack>
            </MenuAccordion>

            {/* Resources */}
            <MenuAccordion
              expanded={expandedPanel === 'resources'}
              title="Resources"
              onChange={handlePanelChange('resources')}
            >
              <Stack sx={{ gap: 2.5 }}>
                {withIcons(menuResources, resourceIcons).map((item) => (
                  <MegaMenuItem key={item.label} {...item} />
                ))}
                <Divider sx={{ ...dividerSx, mt: -1 }} />
              </Stack>
            </MenuAccordion>

            {/* Pricing */}
            <Link href="/pricing" style={{ textDecoration: 'none', color: 'inherit' }}>
              <Typography sx={{ color: 'text.secondary', fontSize: '1rem', fontWeight: 600 }}>
                Pricing
              </Typography>
            </Link>

            <Divider sx={dividerSx} />

            {/* Discord + Auth */}
            <Stack sx={{ gap: 1.5 }}>
              <DiscordButton sx={{ color: 'text.secondary', width: '100%', justifyContent: 'center' }} />
              <MobileDownloadLink />
              <MobileAuthButtons />
            </Stack>
          </Stack>
        </Stack>
      </ShellDrawer>
    </>
  );
}
