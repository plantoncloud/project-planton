'use client';

import { FC } from 'react';
import Link from 'next/link';
import Image from 'next/image';
import { Box, Typography } from '@mui/material';
import {
  Section,
  SectionTitle,
  SectionSubtitle,
  Badge,
  PrimaryButton,
} from '@/components/landing-page/v3-2026-01-02-1000/shared';
import { POSITIONING } from '@/data/positioning';
import { DESKTOP_BREW_COMMAND, DESKTOP_DOWNLOAD_PATH, DESKTOP_PLATFORMS, DESKTOP_PLATFORM_BY_ID } from '@/data/desktop-download';
import { CopyCommand } from './copy-command';
import { LANDING_SCREENSHOTS, type LandingScreenshot } from './landing-screenshots';
import { useDetectedPlatform } from './use-detected-platform';

// A real capture of the app in its own window chrome, framed only by the
// kit's border. When the scene has no capture yet the section simply has no
// image -- a placeholder would be a picture of nothing.
export const Screenshot: FC<{ shot: LandingScreenshot | null; className?: string; priority?: boolean }> = ({
  shot,
  className = '',
  priority = false,
}) =>
  shot ? (
    <Box className={`rounded-xl border border-[#2a2a2a] overflow-hidden bg-[#111] ${className}`}>
      <Image
        src={shot.src}
        alt={shot.alt}
        width={shot.width}
        height={shot.height}
        priority={priority}
        className="w-full h-auto block"
      />
    </Box>
  ) : null;

// The platforms line states exactly what the install page offers today, from
// the same data, so the two surfaces can never disagree about Windows.
const platformsLine = (): string => {
  const on = DESKTOP_PLATFORMS.filter((p) => p.available).map((p) => (p.minimum.startsWith(p.name) ? p.minimum : `${p.name} with ${p.minimum}`));
  const off = DESKTOP_PLATFORMS.filter((p) => !p.available).map((p) => p.name);
  const offClause = off.length ? ` ${off.join(' and ')} returns with the next release.` : '';
  return `Today for ${on.join(' and ')}.${offClause} Signed and notarized on macOS.`;
};

export const LandingHero: FC = () => {
  const detected = useDetectedPlatform();
  const platform = detected ? DESKTOP_PLATFORM_BY_ID[detected] : null;
  const ctaLabel = platform?.available ? `Download for ${platform.name}` : 'Download Planton Desktop';
  const shot = LANDING_SCREENSHOTS.home;

  return (
    <Section className="pt-24 md:pt-32">
      <Box className="max-w-5xl mx-auto">
        <Box className={`text-center ${shot ? 'mb-10' : ''}`}>
          <Badge variant="success" className="mb-6">
            Free Forever
          </Badge>
          <SectionTitle component="h1" className="!text-3xl md:!text-5xl !leading-tight mb-5 max-w-4xl mx-auto">
            {POSITIONING.desktop.line}
          </SectionTitle>
          <SectionSubtitle className="mx-auto !max-w-3xl !text-base md:!text-lg">
            The whole Planton platform runs on your laptop and deploys to your cloud with the logins already on your
            machine. No account. Free for individuals, including commercial use.
          </SectionSubtitle>

          <Box className="flex flex-col items-center gap-4 mt-8">
            <Link href={DESKTOP_DOWNLOAD_PATH}>
              <PrimaryButton className="!px-8 !py-3 !text-base">{ctaLabel}</PrimaryButton>
            </Link>
            <Box className="hidden sm:block">
              <CopyCommand command={DESKTOP_BREW_COMMAND} label="Copy the Homebrew command" />
            </Box>
            <Typography className="text-xs text-[#666] max-w-xl">{platformsLine()}</Typography>
          </Box>
        </Box>

        <Screenshot shot={shot} priority className="max-w-4xl mx-auto" />
      </Box>
    </Section>
  );
};
