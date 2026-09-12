'use client';

import { FC } from 'react';
import { Box, Typography } from '@mui/material';
import {
  Section,
  SectionTitle,
  SectionSubtitle,
  FeatureTitle,
  BodyText,
  Card,
  Grid,
  Divider,
} from '@/components/landing-page/v3-2026-01-02-1000/shared';
import { MetricsStrip, ScrollReveal } from '@/components/product/shared';
import type { MetricItem } from '@/components/product/shared';
import { POSITIONING } from '@/data/positioning';
import { PLATFORM_STATS } from '@/data/platform-stats';
import { AGENT_SKILLS_INSTALL_COMMAND } from '@/data/desktop-download';
import { CopyCommand } from './copy-command';
import { LANDING_SCREENSHOTS } from './landing-screenshots';
import { Screenshot } from './landing-hero';

// ---------------------------------------------------------------------------
// The two ways a person already does this, and the honest third way.
// ---------------------------------------------------------------------------

const TwoWays: FC = () => (
  <Section id="two-ways">
    <Box className="max-w-5xl mx-auto">
      <Box className="text-center mb-10">
        <SectionTitle>You Already Have Two Ways to Do This</SectionTitle>
      </Box>
      <Grid cols={2} className="mb-8">
        <Card hover={false}>
          <FeatureTitle className="mb-1">A Platform That Hides the Cloud</FeatureTitle>
          <Typography className="text-sm text-[#a0a0a0] mb-3">Convenience, in exchange for control.</Typography>
          <BodyText>
            Your app runs in someone else&apos;s account, under IAM you did not write, on a bill you cannot itemize.
            When you outgrow it, you start over.
          </BodyText>
        </Card>
        <Card hover={false}>
          <FeatureTitle className="mb-1">Your Agent and the Cloud CLI</FeatureTitle>
          <Typography className="text-sm text-[#a0a0a0] mb-3">Control, in exchange for a record.</Typography>
          <BodyText>
            The agent writes the Terraform from memory and you learn at apply time what it got wrong. Permissions
            nobody derived. A cost you find out about next month. A database password that went through the chat to
            reach the shell. No pipeline. The second environment is a second conversation.
          </BodyText>
        </Card>
      </Grid>
      <Box className="text-center max-w-3xl mx-auto">
        <Typography className="text-base md:text-lg text-[#ededed] font-medium">
          Planton is a platform too. It just runs on your machine, in your account, and gives the agent rails you can
          inspect.
        </Typography>
        <Typography className="text-sm text-[#666] mt-3">{POSITIONING.desktop.concession}</Typography>
      </Box>
    </Box>
  </Section>
);

// ---------------------------------------------------------------------------
// Two ways to ask, one craft: the coding agent leads, the assistant beside it.
// ---------------------------------------------------------------------------

const TwoWaysToAsk: FC = () => (
  <Section id="two-ways-to-ask">
    <Box className="max-w-5xl mx-auto">
      <Box className="text-center mb-10">
        <SectionTitle>Two Ways to Ask. One Craft.</SectionTitle>
        <SectionSubtitle className="mx-auto">
          Ask from inside the tool you already use, or ask the assistant built in. Both run the same skills.
        </SectionSubtitle>
      </Box>
      <Grid cols={2} gap="lg">
        <Card hover={false} className="!p-6 md:!p-8">
          <FeatureTitle className="mb-3">Your Coding Agent</FeatureTitle>
          <BodyText className="mb-4">
            Install the Planton skills once. From then on Cursor, Claude Code, or Codex composes infrastructure
            grounded in the real schema, validates it offline, and applies it through the platform on your laptop,
            while you stay in your editor.
          </BodyText>
          <CopyCommand command={AGENT_SKILLS_INSTALL_COMMAND} label="Copy the skills install command" className="mb-6" />
          <Typography className="text-sm font-semibold text-[#ededed] mb-3">What Planton Adds to the Agent You Already Use</Typography>
          <Box component="ul" className="flex flex-col gap-2.5 list-none p-0 m-0">
            {POSITIONING.desktop.whatItAdds.map((point) => (
              <Box component="li" key={point} className="flex items-start gap-3">
                <Box className="mt-[7px] w-1.5 h-1.5 rounded-full bg-[#3a3a3a] shrink-0" aria-hidden />
                <BodyText className="!text-[13px]">{point}</BodyText>
              </Box>
            ))}
          </Box>
        </Card>
        <Card hover={false} className="!p-6 md:!p-8">
          <FeatureTitle className="mb-3">The Built-In Assistant</FeatureTitle>
          <BodyText className="mb-4">
            Open the app and ask. The Planton Assistant converses with zero keys and zero accounts; Planton funds it.
          </BodyText>
          <BodyText className="mb-4">
            Prefer to keep everything on your machine? Bring your own Anthropic or Cursor key in the engine chooser,
            and the conversation and the model traffic never leave the laptop.
          </BodyText>
          <Screenshot shot={LANDING_SCREENSHOTS.chooser} className="mt-6" />
        </Card>
      </Grid>
      <Typography className="text-center text-sm text-[#a0a0a0] mt-8 max-w-3xl mx-auto">
        Your agent and the platform&apos;s own assistant share one craft: the same skills, the same catalog, the same
        rules about what never happens without your say-so.
      </Typography>
    </Box>
  </Section>
);

// ---------------------------------------------------------------------------
// What actually runs on the machine.
// ---------------------------------------------------------------------------

const runs = [
  {
    title: 'The Control Plane and One Runner',
    body:
      'The same control plane that serves planton.ai, running as a local process, with a single runner that is ready the moment the app is. No slug to name, no registration, no credentials to hand over.',
  },
  {
    title: 'Postgres, Temporal, and a Cache, as Native Processes',
    body: 'Downloaded once on first launch, verified, and supervised by the app. Docker is not required.',
  },
  {
    title: 'OpenTofu and Pulumi as Your Tools',
    body:
      'Planton honors the tofu or pulumi already on your PATH. Only when nothing usable exists does it install one, through a version manager you already use or from the tool\u2019s official distribution, checksum-verified.',
  },
  {
    title: 'Your Data Stays Here',
    body:
      'Secrets encrypted in the local database with the key in your OS keychain. State on your disk. Logs on your disk. Local data never leaves the laptop.',
  },
];

const WhatRuns: FC = () => (
  <Section id="what-runs">
    <Box className="max-w-5xl mx-auto">
      <Box className="text-center mb-10">
        <SectionTitle>What Actually Runs on Your Machine</SectionTitle>
        <SectionSubtitle className="mx-auto">A full platform as ordinary processes. No containers to babysit.</SectionSubtitle>
      </Box>
      <Grid cols={4} gap="sm">
        {runs.map((tile) => (
          <Card key={tile.title} hover={false}>
            <FeatureTitle className="mb-2 !text-base">{tile.title}</FeatureTitle>
            <BodyText className="!text-[13px]">{tile.body}</BodyText>
          </Card>
        ))}
      </Grid>
    </Box>
  </Section>
);

// ---------------------------------------------------------------------------
// Infra Hub on the laptop (the hub's one analogy lives only here).
// ---------------------------------------------------------------------------

const infraPoints = [
  {
    title: 'Your Clouds, Found, Not Pasted',
    body:
      'Planton reads the cloud logins already on your machine (AWS profiles, gcloud configurations, az subscriptions, kubeconfig contexts) and turns each into a ready connection. Cloudflare and DigitalOcean tokens are detected from your environment and verified against the provider before they are offered. You never paste a key into a form.',
  },
  {
    title: `${PLATFORM_STATS.DEPLOYMENT_MODULE_COUNT} Resource Kinds Across ${PLATFORM_STATS.CLOUD_PROVIDER_COUNT} Providers`,
    body: 'The full catalog, seeded into your local instance at first boot.',
  },
  {
    title: 'A Map of Everything You Run',
    body:
      'Accounts, environments, projects, resources, and the services that span them: one living picture, with drill-down to each project\u2019s diagram.',
  },
];

const InfraHub: FC = () => (
  <Section id="infra-hub">
    <Box className="max-w-5xl mx-auto">
      <Box className="text-center mb-10">
        <Typography className="text-xs font-semibold uppercase tracking-wider text-[#666] mb-3">
          {POSITIONING.infraHub.analogy}
        </Typography>
        <SectionTitle>{POSITIONING.infraHub.name} on Your Laptop</SectionTitle>
        <SectionSubtitle className="mx-auto !max-w-3xl">{POSITIONING.infraHub.line}</SectionSubtitle>
      </Box>
      <ScrollReveal>
        <Screenshot shot={LANDING_SCREENSHOTS.studio} className="max-w-4xl mx-auto mb-10" />
      </ScrollReveal>
      <Grid cols={3}>
        {infraPoints.map((point) => (
          <Card key={point.title} hover={false}>
            <FeatureTitle className="mb-2 !text-base">{point.title}</FeatureTitle>
            <BodyText className="!text-[13px]">{point.body}</BodyText>
          </Card>
        ))}
      </Grid>
    </Box>
  </Section>
);

// ---------------------------------------------------------------------------
// Service Hub on the laptop: the measured record, and the one Docker note.
// ---------------------------------------------------------------------------

// Measured on an M-series Mac: push at 07:33:26, run at 07:34:01 (including
// waking the stopped build cluster), a 30 s build, a 34 s deploy; the idle
// build cluster sampled at roughly 800 MiB. Every figure here is a measured
// number, never a projection.
const serviceMetrics: MetricItem[] = [
  { value: '35 s', label: 'push to run start' },
  { value: '30 s', label: 'build' },
  { value: '34 s', label: 'deploy' },
  { value: '~800 MiB', label: 'build cluster at rest' },
];

const servicePoints = [
  {
    title: 'No Public URL, No GitHub App',
    body:
      'A laptop cannot receive a webhook, so the local instance asks GitHub whether the branches it watches have moved, with your own gh sign-in and conditional requests that cost nothing while nothing changed. Each push becomes the same run a webhook would have started.',
  },
  {
    title: 'Builds in a Pod on Your Machine',
    body:
      'The build runs on a small build cluster the app sets up inside Docker Desktop the first time you say yes. It sleeps after ten idle minutes and wakes on your next push. Your sign-in enters the build once and is deleted when the run ends.',
  },
  {
    title: 'Deploys Where You Point It',
    body:
      'The image goes to the cluster or cloud you connected through the same modules hosted Planton uses, with the same rollout verification; the result lands on the commit as a status your pull request shows.',
  },
];

const ServiceHub: FC = () => (
  <>
    <Section id="service-hub" className="!pb-6">
      <Box className="max-w-5xl mx-auto text-center">
        <Typography className="text-xs font-semibold uppercase tracking-wider text-[#666] mb-3">
          {POSITIONING.serviceHub.analogy}
        </Typography>
        <SectionTitle>{POSITIONING.serviceHub.name} on Your Laptop</SectionTitle>
        <SectionSubtitle className="mx-auto !max-w-3xl">
          Push to GitHub. Your laptop notices, builds the commit in a pod, deploys it to the cluster or cloud you
          connected, and puts a status on the commit.
        </SectionSubtitle>
      </Box>
    </Section>
    <MetricsStrip metrics={serviceMetrics} />
    <Section className="!pt-6">
      <Box className="max-w-5xl mx-auto">
        <Typography className="text-center text-xs text-[#666] mb-10 max-w-2xl mx-auto">
          Measured on an M-series Mac, end to end, unattended: pushed at 07:33:26, the run started at 07:34:01 (waking the
          stopped build cluster), then a 30 s build and a 34 s deploy. A running, verified deployment inside two minutes
          of the push.
        </Typography>
        <Grid cols={3}>
          {servicePoints.map((point) => (
            <Card key={point.title} hover={false}>
              <FeatureTitle className="mb-2 !text-base">{point.title}</FeatureTitle>
              <BodyText className="!text-[13px]">{point.body}</BodyText>
            </Card>
          ))}
        </Grid>
        <BodyText className="text-center mt-8 !text-[13px] !text-[#a0a0a0] max-w-2xl mx-auto">
          Building on your own machine needs Docker Desktop. Planton detects it and never installs it. Everything else on
          this page runs without Docker.
        </BodyText>
      </Box>
    </Section>
  </>
);

// ---------------------------------------------------------------------------
// The same Planton everywhere.
// ---------------------------------------------------------------------------

const Everywhere: FC = () => (
  <>
    <Section className="!py-0">
      <Divider />
    </Section>
    <Section id="everywhere">
      <Box className="max-w-3xl mx-auto text-center">
        <SectionTitle>The Same Planton Everywhere</SectionTitle>
        <BodyText className="mt-5 !text-base !text-[#ededed]">
          This laptop, planton.ai, or your company&apos;s cluster: pick where in the instance switcher. One data model,
          one set of code paths; only which capabilities run differs. Your laptop can also deploy into your team&apos;s
          Planton, with your cloud credentials still never leaving your machine.
        </BodyText>
        <Typography className="text-sm text-[#666] mt-6">{POSITIONING.umbrella.sentence}</Typography>
      </Box>
    </Section>
  </>
);

export const LandingCapabilities: FC = () => (
  <>
    <TwoWays />
    <TwoWaysToAsk />
    <WhatRuns />
    <InfraHub />
    <ServiceHub />
    <Everywhere />
  </>
);
