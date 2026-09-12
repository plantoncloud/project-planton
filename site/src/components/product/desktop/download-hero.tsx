'use client';

import { FC } from 'react';
import Link from 'next/link';
import { Box, Typography } from '@mui/material';
import { sendGAEvent } from '@next/third-parties/google';
import {
  Section,
  SectionTitle,
  SectionSubtitle,
  Badge,
  PrimaryButton,
  SecondaryButton,
  Card,
  BodyText,
} from '@/components/landing-page/v3-2026-01-02-1000/shared';
import {
  DESKTOP_BREW_COMMAND,
  DESKTOP_PLATFORMS,
  desktopChecksumsUrl,
  type DesktopArtifact,
  type DesktopPlatform,
  type DesktopPlatformId,
} from '@/data/desktop-download';
import { DESKTOP_RELEASE, formatDownloadSize } from '@/data/desktop-release';
import { CopyCommand } from './copy-command';

interface DownloadHeroProps {
  platform: DesktopPlatform;
  onSelect: (id: DesktopPlatformId) => void;
  version: string | null;
}

const recordDownload = (platform: DesktopPlatformId, artifact: DesktopArtifact, version: string | null) => {
  sendGAEvent('event', 'desktop_download', { platform, artifact: artifact.key, version: version ?? 'unknown' });
};

const artifactSize = (artifact: DesktopArtifact): string | null => {
  const size = DESKTOP_RELEASE.artifacts[artifact.key];
  return size ? formatDownloadSize(size.bytes) : null;
};

const PlatformTabs: FC<{ selected: DesktopPlatformId; onSelect: (id: DesktopPlatformId) => void }> = ({
  selected,
  onSelect,
}) => (
  <Box role="tablist" aria-label="Platform" className="flex justify-center gap-2 mb-6">
    {DESKTOP_PLATFORMS.map((platform) => {
      const active = platform.id === selected;
      return (
        <button
          key={platform.id}
          role="tab"
          type="button"
          aria-selected={active}
          aria-controls={`platform-${platform.id}`}
          onClick={() => onSelect(platform.id)}
          className={`px-4 py-2 rounded-lg border text-sm font-medium transition-colors duration-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#ededed] ${
            active
              ? 'border-[#ededed] bg-[#1a1a1a] text-[#ededed]'
              : 'border-[#2a2a2a] bg-transparent text-[#a0a0a0] hover:border-[#3a3a3a] hover:text-[#ededed]'
          }`}
        >
          {platform.name}
        </button>
      );
    })}
  </Box>
);

// The door for a platform whose installer is not available right now. It says
// why, and it names the two real ways forward with a link each -- never a
// dead end under a card that offers nothing.
const UnavailableCard: FC<{ platform: DesktopPlatform; onSelect: (id: DesktopPlatformId) => void }> = ({
  platform,
  onSelect,
}) => {
  const alternatives = DESKTOP_PLATFORMS.filter((p) => p.available);
  return (
    <Card hover={false} className="max-w-2xl mx-auto !p-8 md:!p-10">
      <Box id={`platform-${platform.id}`} role="tabpanel" className="flex flex-col items-center text-center">
        <Typography component="h2" className="text-lg font-semibold text-[#ededed] mb-2">
          The {platform.name} Installer Returns with the Next Release
        </Typography>
        <BodyText className="!text-base max-w-lg">
          The build currently published for {platform.name} is not one we would ask you to run, so the link is
          off until the next release replaces it. Two ways forward today:
        </BodyText>
        <Box className="flex flex-col sm:flex-row flex-wrap gap-3 justify-center mt-6">
          <Link href="/docs/cli">
            <PrimaryButton className="!whitespace-nowrap">Install the CLI on WSL</PrimaryButton>
          </Link>
          {alternatives.map((alt) => (
            <SecondaryButton key={alt.id} className="!whitespace-nowrap" onClick={() => onSelect(alt.id)}>
              Switch to {alt.name}
            </SecondaryButton>
          ))}
        </Box>
        <Typography className="text-xs text-[#666] mt-5 max-w-md">
          The CLI runs the same schema lookups, validation, and deploys from a terminal; the desktop app adds
          the assistant, the canvas, and the local instance.
        </Typography>
      </Box>
    </Card>
  );
};

const PlatformCard: FC<{ platform: DesktopPlatform; version: string | null }> = ({ platform, version }) => {
  const [primary, ...secondary] = [
    ...platform.artifacts.filter((a) => a.primary),
    ...platform.artifacts.filter((a) => !a.primary),
  ];
  const primarySize = artifactSize(primary);

  return (
    <Card hover={false} className="max-w-2xl mx-auto !p-8 md:!p-10">
      <Box id={`platform-${platform.id}`} role="tabpanel" className="flex flex-col items-center text-center">
        <PrimaryButton
          href={primary.href}
          onClick={() => recordDownload(platform.id, primary, version)}
          className="!px-8 !py-3 !text-base"
        >
          {primary.label}
          {primarySize && <span className="ml-2 text-[#666] font-normal">{primarySize}</span>}
        </PrimaryButton>

        {secondary.length > 0 && (
          <Box className="flex flex-wrap justify-center gap-3 mt-3">
            {secondary.map((artifact) => {
              const size = artifactSize(artifact);
              return (
                <SecondaryButton
                  key={artifact.key}
                  href={artifact.href}
                  onClick={() => recordDownload(platform.id, artifact, version)}
                >
                  {artifact.label}
                  {size && <span className="ml-2 text-[#666] font-normal">{size}</span>}
                </SecondaryButton>
              );
            })}
          </Box>
        )}

        <Typography
          component="ul"
          className="flex flex-wrap justify-center gap-x-4 gap-y-1 mt-5 text-xs text-[#a0a0a0] list-none p-0"
        >
          {platform.facts.map((fact) => (
            <li key={fact}>{fact}</li>
          ))}
        </Typography>

        {platform.id === 'macos' && (
          <Box className="mt-6 flex flex-col items-center gap-2">
            <Typography className="text-xs text-[#666]">
              Or with Homebrew, which also installs the <code className="font-mono text-[#a0a0a0]">planton</code> CLI
            </Typography>
            <CopyCommand command={DESKTOP_BREW_COMMAND} label="Copy the Homebrew command" />
          </Box>
        )}

        <Box className="mt-6 flex flex-col items-center gap-3 max-w-lg">
          {platform.installSteps.map((step) => (
            <Box key={step.text} className="flex flex-col items-center gap-2">
              <BodyText className="!text-[13px] !text-[#a0a0a0]">{step.text}</BodyText>
              {step.command && <CopyCommand command={step.command} label={`Copy: ${step.command}`} />}
            </Box>
          ))}
        </Box>
      </Box>
    </Card>
  );
};

export const DownloadHero: FC<DownloadHeroProps> = ({ platform, onSelect, version }) => {
  const checksumsUrl = version ? desktopChecksumsUrl(version) : null;

  return (
    <Section className="pt-24 md:pt-32">
      <Box className="max-w-4xl mx-auto">
        <Box className="text-center mb-10">
          <Badge variant="success" className="mb-6">
            Free Forever
          </Badge>
          <SectionTitle component="h1" className="!text-3xl md:!text-5xl !leading-tight mb-4">
            Download Planton Desktop
          </SectionTitle>
          <SectionSubtitle className="mx-auto !max-w-2xl !text-base md:!text-lg">
            Free for individuals, including commercial use. No account, no sign-up. Pick your platform; the rest is
            one launch.
          </SectionSubtitle>
          {version && (
            <Typography className="text-xs text-[#666] mt-4">
              Latest release <span className="font-mono text-[#a0a0a0]">{version}</span>
              {checksumsUrl && (
                <>
                  {' · '}
                  <a href={checksumsUrl} className="underline decoration-[#3a3a3a] hover:text-[#a0a0a0]">
                    Checksums
                  </a>
                </>
              )}
            </Typography>
          )}
        </Box>

        <PlatformTabs selected={platform.id} onSelect={onSelect} />
        {platform.available ? (
          <PlatformCard platform={platform} version={version} />
        ) : (
          <UnavailableCard platform={platform} onSelect={onSelect} />
        )}
      </Box>
    </Section>
  );
};
