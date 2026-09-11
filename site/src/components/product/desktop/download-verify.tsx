'use client';

import { FC } from 'react';
import Link from 'next/link';
import { Typography } from '@mui/material';
import {
  Section,
  Card,
  FeatureTitle,
  BodyText,
  Divider,
  Grid,
} from '@/components/landing-page/v3-2026-01-02-1000/shared';
import { DESKTOP_BREW_UPGRADE_COMMAND, desktopChecksumsUrl, type DesktopPlatform } from '@/data/desktop-download';
import { CommandBlock } from './command-block';

interface DownloadVerifyProps {
  platform: DesktopPlatform;
  version: string | null;
}

export const DownloadVerify: FC<DownloadVerifyProps> = ({ platform, version }) => {
  const checksumsUrl = version ? desktopChecksumsUrl(version) : null;
  return (
    <>
      <Section className="!py-0">
        <Divider />
      </Section>
      <Section>
        <Grid cols={2} className="max-w-5xl mx-auto">
          <Card hover={false}>
            <FeatureTitle className="mb-2">Verify the Download on {platform.name}</FeatureTitle>
            <BodyText className="mb-4">
              Hash the file you downloaded and compare it with{' '}
              {checksumsUrl ? (
                <a href={checksumsUrl} className="underline decoration-[#3a3a3a] hover:text-[#ededed]">
                  the release&apos;s checksums
                </a>
              ) : (
                'the release\u2019s checksums'
              )}
              .
              {platform.id === 'macos' &&
                ' The next two lines ask Gatekeeper and the notarization ticket directly; a signed, stapled build passes both.'}
            </BodyText>
            <CommandBlock commands={platform.verifyCommands} label={`Copy the ${platform.name} verification commands`} />
          </Card>

          <Card hover={false}>
            <FeatureTitle className="mb-2">Updates</FeatureTitle>
            <BodyText>
              The app checks for updates and installs them with one click.
              {platform.id === 'macos' && (
                <>
                  {' '}
                  On Homebrew, <code className="font-mono text-[#ededed]">{DESKTOP_BREW_UPGRADE_COMMAND}</code> does the
                  same.
                </>
              )}{' '}
              Updates are signed by Planton and verified before they are applied.
            </BodyText>
          </Card>
        </Grid>

        <Typography className="text-center text-sm text-[#666] mt-12 max-w-2xl mx-auto">
          Want the platform without the app?{' '}
          <Link href="/docs/cli" className="underline decoration-[#3a3a3a] hover:text-[#a0a0a0]">
            Install the CLI on its own
          </Link>
          , or{' '}
          <Link href="/docs/self-hosting" className="underline decoration-[#3a3a3a] hover:text-[#a0a0a0]">
            run the self-hosted edition on your own cluster
          </Link>
          .
        </Typography>
      </Section>
    </>
  );
};
