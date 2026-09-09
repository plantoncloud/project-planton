'use client';

import { Box, Typography } from '@mui/material';
import Link from 'next/link';
import { FC } from 'react';
import {
  Section,
  SectionTitle,
  SectionSubtitle,
  Grid,
  Card,
  FeatureTitle,
  BodyText,
  Badge,
  SecondaryButton,
} from './shared';
import { COMMUNITY_SEAT_LIMIT, FREE_TIER_SEATS, MARKETS, SELF_SERVE_SEAT_CEILING } from '@/data/pricing';

/**
 * The distribution strip. Numbers read from src/data/pricing.ts — a
 * sentence that names a price references a constant, never a literal.
 * The desktop tile is copy-only by decision (2026-08-17): no download
 * link until the desktop release hold lifts.
 */

export const ThreeWaysToRun: FC = () => {
  const ways = [
    {
      badge: 'Free Forever',
      title: 'Desktop App',
      description:
        'Runs on your laptop, deploys with the cloud logins already on your machine. Free forever, including commercial use — no account required.',
    },
    {
      badge: `Free for ${FREE_TIER_SEATS} Seats`,
      title: 'Hosted on Planton.ai',
      description: `For teams that do not want to run anything. Free tier never bills; the Team plan is $${MARKETS.us.teamSeatMonthly}/seat per month, self-serve with a card.`,
    },
    {
      badge: 'Free Community Edition',
      title: 'Self-Hosted',
      description: `The full core product on your own Kubernetes cluster, free for up to ${COMMUNITY_SEAT_LIMIT} seats, runs offline. Paid licenses unlock more seats, bought with a card and an email.`,
    },
  ];

  return (
    <Section id="three-ways">
      <Box className="text-center mb-10">
        <SectionTitle>Three Ways To Run It</SectionTitle>
        <SectionSubtitle className="mx-auto">
          Same platform, same catalog — on your laptop, on Planton.ai, or in
          your own cluster.
        </SectionSubtitle>
      </Box>

      <Grid cols={3} className="max-w-5xl mx-auto mb-8">
        {ways.map((way) => (
          <Card key={way.title} className="text-center">
            <Box className="mb-3">
              <Badge variant="success">{way.badge}</Badge>
            </Box>
            <FeatureTitle className="mb-2">{way.title}</FeatureTitle>
            <BodyText>{way.description}</BodyText>
          </Card>
        ))}
      </Grid>

      <Box className="text-center">
        <Link href="/pricing">
          <SecondaryButton>See Full Pricing</SecondaryButton>
        </Link>
        <Typography className="text-xs text-[#555] mt-3">
          At {SELF_SERVE_SEAT_CEILING} seats or fewer, everything is self-serve. Free tiers pause instead
          of billing.
        </Typography>
      </Box>
    </Section>
  );
};
