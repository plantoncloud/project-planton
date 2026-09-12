'use client';

import { FC } from 'react';
import Link from 'next/link';
import { Box, Typography } from '@mui/material';
import {
  Section,
  SectionTitle,
  Card,
  BodyText,
  PrimaryButton,
  SecondaryButton,
  Divider,
} from '@/components/landing-page/v3-2026-01-02-1000/shared';
import { RelatedModules } from '@/components/product/shared';
import { COMMUNITY_SEAT_LIMIT, FREE_TIER_SEATS } from '@/data/pricing';
import { DESKTOP_DOWNLOAD_PATH } from '@/data/desktop-download';

// Why it is free, said as a business model rather than a badge. The seat
// counts read pricing constants; prices themselves stay on the pricing page,
// where the market gate decides which currency a visitor sees.
const WhyFree: FC = () => (
  <Section id="why-free">
    <Box className="max-w-2xl mx-auto">
      <SectionTitle className="text-center">Why It Is Free</SectionTitle>
      <BodyText className="mt-5 !text-base">
        Planton Desktop is free for individuals, including commercial use. Not a trial, not a tier with the good parts
        removed: the full catalog, the wizards, the IaC pipelines, service CI/CD, and secrets management are the core
        product on every edition.
      </BodyText>
      <BodyText className="mt-4 !text-base">
        Here is the business model, stated plainly. Planton makes its money when a team adopts it: hosted seats beyond
        the free {FREE_TIER_SEATS}, self-hosted licenses beyond the community edition&apos;s {COMMUNITY_SEAT_LIMIT}{' '}
        seats, and prepaid AI credits. The solo developer is not a funnel stage. You are the person this was built to
        delight, and if you one day bring a team, nothing you built gets redone.
      </BodyText>
      <Link href="/pricing" className="inline-block mt-5 text-sm text-[#a0a0a0] underline decoration-[#3a3a3a] hover:text-[#ededed]">
        See Full Pricing
      </Link>
    </Box>
  </Section>
);

export const LandingCTA: FC = () => (
  <>
    <WhyFree />
    <RelatedModules modules={['cli', 'infra-hub', 'service-hub']} />
    <Section className="!py-0">
      <Divider />
    </Section>
    <Section>
      <Card hover={false} className="!p-8 md:!p-12 text-center max-w-3xl mx-auto">
        <Typography component="h2" className="text-2xl md:text-3xl font-semibold text-[#ededed] mb-4">
          Install It. Ask It Something.
        </Typography>
        <BodyText className="!text-base mx-auto max-w-xl mb-8">
          Download, open, pick Run on This Computer, and ask for what you need in your own words. Or install the skills
          and ask from your editor.
        </BodyText>
        <Box className="flex flex-col sm:flex-row sm:flex-wrap gap-3 justify-center">
          <Link href={DESKTOP_DOWNLOAD_PATH}>
            <PrimaryButton className="!whitespace-nowrap">Download Planton Desktop</PrimaryButton>
          </Link>
          <Link href="/docs/ci-cd/on-your-laptop">
            <SecondaryButton className="!whitespace-nowrap">Read the CI/CD Guide</SecondaryButton>
          </Link>
          <Link href="/docs/coding-agents">
            <SecondaryButton className="!whitespace-nowrap">Connect Your Agent</SecondaryButton>
          </Link>
        </Box>
      </Card>
    </Section>
  </>
);
