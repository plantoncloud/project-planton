'use client';

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
import { Section, SectionTitle, SectionSubtitle } from '@/components/landing-page/v3-2026-01-02-1000/shared';
import { ReactNode } from 'react';
import { PLATFORM_STATS } from '@/data/platform-stats';
import { DESKTOP_LANDING_PATH } from '@/data/desktop-download';

interface ProductModuleCardProps {
  icon: ReactNode;
  title: string;
  pain: string;
  solution: string;
  href: string;
}

const ProductModuleCard = ({ icon, title, pain, solution, href }: ProductModuleCardProps) => (
  <Link href={href} className="group">
    <Box
      className="
        rounded-xl bg-[#151515] border border-[#2a2a2a]
        hover:border-[#3a3a3a] hover:bg-[#1a1a1a]
        transition-all duration-300
        p-6 h-full flex flex-col
      "
    >
      <Box className="w-10 h-10 rounded-lg bg-white/10 flex items-center justify-center mb-4 text-white group-hover:bg-white/15 transition-colors">
        {icon}
      </Box>
      <Typography className="text-lg font-semibold text-white mb-2">{title}</Typography>
      <Typography className="text-sm text-[#666] mb-3 leading-relaxed">{pain}</Typography>
      <Typography className="text-sm text-[#b0b0b0] leading-relaxed flex-1">{solution}</Typography>
      <Typography className="text-sm font-medium text-white mt-4 group-hover:translate-x-1 transition-transform">
        Learn more &rarr;
      </Typography>
    </Box>
  </Link>
);

const modules: ProductModuleCardProps[] = [
  {
    icon: <InfraHubIcon />,
    title: 'Infra Hub',
    pain: 'Developers wait days for infrastructure. Ops is the bottleneck.',
    solution: `Deploy ${PLATFORM_STATS.DEPLOYMENT_MODULE_COUNT} cloud resource types across any provider in minutes. Infra Charts, dependency-aware pipelines, and auditable stack jobs.`,
    href: '/features/infra-hub',
  },
  {
    icon: <ServiceHubIcon />,
    title: 'Service Hub',
    pain: 'Shipping a backend service means wrestling with Dockerfiles, Helm charts, and CI pipelines.',
    solution: 'Connect your Git repo. Push code. Planton builds and deploys to K8s, ECS, Cloud Run, or Workers. Vercel for Backend, In Your Own Cloud.',
    href: '/features/service-hub',
  },
  {
    icon: <RunnerIcon />,
    title: 'Runner',
    pain: 'SaaS platforms want your credentials. On-prem tools need you to manage everything.',
    solution: 'A single binary in your cloud. Planton orchestrates, Runner executes. Your credentials never leave your account.',
    href: '/features/runner',
  },
  {
    icon: <SecurityIcon />,
    title: 'Security',
    pain: 'Secrets sprawl. No audit trail. Compliance means stitching together five different tools.',
    solution: 'Secrets management with multi-backend support, IAM with fine-grained RBAC, full audit trails, and a zero-trust architecture built into every layer.',
    href: '/features/security',
  },
  {
    icon: <AgentFleetIcon />,
    title: 'Agent Fleet',
    pain: 'Your DevOps team is firefighting, not building. Pipeline failures at 2 AM. Tribal knowledge in one person\'s head.',
    solution: 'Specialized AI agents that understand your infrastructure. Purpose-built for pipeline troubleshooting, security hardening, drift detection, and more.',
    href: '/features/agent-fleet',
  },
  {
    icon: <CliIcon />,
    title: 'CLI',
    pain: 'Context-switching between terminal, console, and CI. Different tools for different operations.',
    solution: 'Everything Planton does, from your terminal. kubectl-inspired commands, manifest-driven workflows, real-time stack job streaming.',
    href: '/features/cli',
  },
  {
    icon: <DesktopIcon />,
    title: 'Planton Desktop',
    pain: 'Your coding agent can already create cloud infrastructure -- and leave you with resources you cannot explain, permissions nobody derived, and a password that went through the chat.',
    solution: 'The whole platform on your laptop, deploying to your own cloud with the logins already on your machine. Verifiable, recorded, reusable. Free for individuals, including commercial use.',
    href: DESKTOP_LANDING_PATH,
  },
  {
    icon: <OpenSourceIcon />,
    title: 'Open Source',
    pain: 'Your infrastructure definitions locked inside a vendor platform. No exit path.',
    solution: `Planton open source: protobuf-defined APIs for ${PLATFORM_STATS.DEPLOYMENT_MODULE_COUNT} resource types. Pulumi and Terraform modules. Export your manifests and run them anywhere.`,
    href: '/features/open-source',
  },
];

export const ProductModulesGrid = () => {
  return (
    <Section>
      <Box className="text-center mb-10">
        <SectionTitle>Everything you need to deploy and operate infrastructure</SectionTitle>
        <SectionSubtitle className="mx-auto">
          Seven modules that work together to replace your patchwork of DevOps tools with a single, integrated platform.
        </SectionSubtitle>
      </Box>
      <Box className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {modules.map((module) => (
          <ProductModuleCard key={module.title} {...module} />
        ))}
      </Box>
    </Section>
  );
};
