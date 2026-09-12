'use client';

import { FC, ReactNode } from 'react';
import Link from 'next/link';
import { Box, Typography } from '@mui/material';
import {
  Hub as InfraHubIcon,
  RocketLaunch as ServiceHubIcon,
  PlayCircleOutline as RunnerIcon,
  Shield as SecurityIcon,
  Psychology as AgentFleetIcon,
  Terminal as CliIcon,
  Laptop as DesktopIcon,
  Code as OpenSourceIcon,
} from '@mui/icons-material';
import { Section } from '@/components/landing-page/v3-2026-01-02-1000/shared';
import { DESKTOP_LANDING_PATH } from '@/data/desktop-download';
import { PLATFORM_STATS } from '@/data/platform-stats';

interface ModuleInfo {
  icon: ReactNode;
  title: string;
  description: string;
  href: string;
}

const allModules: Record<string, ModuleInfo> = {
  'infra-hub': {
    icon: <InfraHubIcon sx={{ fontSize: 20 }} />,
    title: 'Infra Hub',
    description: `Deploy ${PLATFORM_STATS.DEPLOYMENT_MODULE_COUNT} cloud resource types across any provider.`,
    href: '/features/infra-hub',
  },
  'service-hub': {
    icon: <ServiceHubIcon sx={{ fontSize: 20 }} />,
    title: 'Service Hub',
    description: 'Git push to production with built-in CI/CD.',
    href: '/features/service-hub',
  },
  runner: {
    icon: <RunnerIcon sx={{ fontSize: 20 }} />,
    title: 'Runner',
    description: 'Execute in your cloud, orchestrate from ours.',
    href: '/features/runner',
  },
  security: {
    icon: <SecurityIcon sx={{ fontSize: 20 }} />,
    title: 'Security',
    description: 'Secrets, IAM, audit trails, zero-trust architecture.',
    href: '/features/security',
  },
  'agent-fleet': {
    icon: <AgentFleetIcon sx={{ fontSize: 20 }} />,
    title: 'Agent Fleet',
    description: 'AI agents purpose-built for infrastructure.',
    href: '/features/agent-fleet',
  },
  cli: {
    icon: <CliIcon sx={{ fontSize: 20 }} />,
    title: 'CLI',
    description: 'Everything Planton does, from your terminal.',
    href: '/features/cli',
  },
  desktop: {
    icon: <DesktopIcon sx={{ fontSize: 20 }} />,
    title: 'Planton Desktop',
    description: 'The whole platform on your laptop, free forever.',
    href: DESKTOP_LANDING_PATH,
  },
  'open-source': {
    icon: <OpenSourceIcon sx={{ fontSize: 20 }} />,
    title: 'Open Source',
    description: 'Planton open source: portable infrastructure definitions.',
    href: '/features/open-source',
  },
};

interface RelatedModulesProps {
  modules: string[];
}

export const RelatedModules: FC<RelatedModulesProps> = ({ modules }) => {
  const items = modules.map((key) => allModules[key]).filter(Boolean);
  if (items.length === 0) return null;

  return (
    <Section>
      <Typography className="text-xs font-semibold uppercase tracking-wider text-[#555] text-center mb-6">
        Related Modules
      </Typography>
      <Box className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 max-w-3xl mx-auto">
        {items.map((mod) => (
          <Link key={mod.href} href={mod.href} className="group">
            <Box className="rounded-xl bg-[#151515] border border-[#2a2a2a] hover:border-[#3a3a3a] hover:bg-[#1a1a1a] transition-all duration-300 p-4 h-full flex items-start gap-3">
              <Box className="w-9 h-9 rounded-lg bg-white/10 flex items-center justify-center text-white flex-shrink-0 group-hover:bg-white/15 transition-colors">
                {mod.icon}
              </Box>
              <Box>
                <Typography className="text-sm font-semibold text-white mb-0.5">
                  {mod.title}
                </Typography>
                <Typography className="text-xs text-[#666] leading-snug">
                  {mod.description}
                </Typography>
              </Box>
            </Box>
          </Link>
        ))}
      </Box>
    </Section>
  );
};
