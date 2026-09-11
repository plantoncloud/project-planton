'use client';

import { FC } from 'react';
import { Box, Typography } from '@mui/material';
import {
  Section,
  SectionTitle,
  SectionSubtitle,
  Card,
  FeatureTitle,
  BodyText,
  Grid,
} from '@/components/landing-page/v3-2026-01-02-1000/shared';
import { CodeTabs } from '@/components/product/shared';
import type { CodeTab } from '@/components/product/shared';
import { AGENT_SKILLS_INSTALL_COMMAND, type DesktopPlatform } from '@/data/desktop-download';

// What happens after the download finishes -- the part download pages skip.
// Three beats a person will actually see, then the three follow-ons as tabs
// whose code is only what they type; the teaching sits under the block.
const beats = [
  {
    title: 'Pick Where Planton Runs',
    body:
      'First launch asks one question: this computer, planton.ai, or a self-hosted deployment. Choose Run on This Computer. No account is asked for, then or later.',
  },
  {
    title: 'Watch It Set Itself Up',
    body:
      'The app downloads its runtime once: a Java runtime, Postgres, Temporal, a cache, and the control plane. A few hundred megabytes with a real progress bar, verified against the release\u2019s checksums. Later upgrades fetch only what changed.',
  },
  {
    title: 'Your Clouds Are Already There',
    body:
      'Planton finds the AWS, Google Cloud, Azure, and Kubernetes sign-ins on your machine and offers each as a ready connection. Then ask for what you need.',
  },
];

const cliInstallNote = (platform: DesktopPlatform): string =>
  platform.id === 'macos'
    ? 'The Homebrew cask installs the CLI with the app. From the disk image, Planton offers to install it for you and shows the one PATH line if ~/.local/bin is not on your PATH yet.'
    : 'Planton offers to install the CLI for you on first launch and shows the one PATH line if ~/.local/bin is not on your PATH yet.';

const followOns = (platform: DesktopPlatform): CodeTab[] => [
  {
    label: 'Coding Agent',
    code: AGENT_SKILLS_INSTALL_COMMAND,
    description:
      'Installs the Planton skills into Cursor, Claude Code, Codex, and any other agent on the machine. Then ask in your editor, in your own words: "I need a Postgres database for this service in dev." The agent composes the manifest from the real schema, validates it offline, and applies it through the platform on this laptop, with one confirmation from you.',
  },
  {
    label: 'CLI',
    code: `planton --version
planton explain aws-vpc
planton validate -f infrastructure/`,
    description: `${cliInstallNote(platform)} Schema lookups and validation work offline, with no account.`,
  },
  {
    label: 'Build Locally (Optional)',
    code: 'planton local build-cluster status',
    description:
      'Only needed to build your own services on this machine; everything else on this page runs without Docker. Say yes to "Build on this machine?" once, with Docker Desktop running. Planton creates a small build cluster inside it, sleeps it after ten idle minutes, and wakes it on your next push.',
  },
];

export const DownloadAfterInstall: FC<{ platform: DesktopPlatform }> = ({ platform }) => (
  <Section id="after-you-install">
    <Box className="max-w-5xl mx-auto">
      <Box className="text-center mb-10">
        <SectionTitle>After You Install</SectionTitle>
        <SectionSubtitle className="mx-auto">One launch does the rest.</SectionSubtitle>
      </Box>

      <Grid cols={3} className="mb-12">
        {beats.map((beat, i) => (
          <Card key={beat.title} hover={false}>
            <Typography className="text-xs font-mono text-[#666] mb-3">{String(i + 1).padStart(2, '0')}</Typography>
            <FeatureTitle className="mb-2">{beat.title}</FeatureTitle>
            <BodyText>{beat.body}</BodyText>
          </Card>
        ))}
      </Grid>

      <Box className="text-center mb-6">
        <Typography component="h3" className="text-base md:text-lg font-semibold text-[#ededed]">
          Then, From Wherever You Work
        </Typography>
        <Typography className="text-sm text-[#a0a0a0] mt-2 max-w-2xl mx-auto">
          Ask from your editor, from the terminal, or build your services on this machine. Each is a minute.
        </Typography>
      </Box>
      <CodeTabs tabs={followOns(platform)} compact className="max-w-3xl mx-auto" />
    </Box>
  </Section>
);
