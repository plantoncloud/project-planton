'use client';
import { FC, ReactNode, useState } from 'react';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Stack,
  Typography,
} from '@mui/material';
import { Add } from '@mui/icons-material';
import { TypoB2Regular, TypoH2 } from '@/components/common';
import Link from 'next/link';
import { EVALUATION_DAYS, EVALUATION_URL, FREE_TIER_SEATS } from '@/data/pricing';
import { useHandoffEmail, withHandoffEmail } from '@/components/handoff';

interface IFaq {
  title: string;
  description: ReactNode;
}

/**
 * The evaluation door inside the FAQ. Its own component because the answer
 * list below is module-level data: a console that handed this tab the
 * buyer's email decorates the link so the claim form opens prefilled.
 */
const EvaluationClaimLink: FC = () => {
  const handoffEmail = useHandoffEmail();
  return (
    <Link className="text-white underline" href={withHandoffEmail(EVALUATION_URL, handoffEmail)}>
      Claim your evaluation key
    </Link>
  );
};

/**
 * Every answer here words a product behavior or a published policy --
 * the downgrade laws, the license fairness ladder, the tax posture, the
 * refund policy. Nothing is invented for marketing.
 */
const faqs: IFaq[] = [
  {
    title: 'What happens if I stop paying?',
    description: (
      <Stack gap={2}>
        <TypoB2Regular className="text-text-secondary">
          Nothing is deleted and nobody is kicked out. Limits apply to new
          things only: everything you deployed keeps running, extra members
          stay but new invites pause. Reading, deleting, and fixing your
          payment are never blocked.
        </TypoB2Regular>
        <TypoB2Regular className="text-text-secondary">
          The moment payment is fixed, everything opens back up by itself —
          no support ticket needed.
        </TypoB2Regular>
      </Stack>
    ),
  },
  {
    title: 'What happens when a license on my infrastructure expires?',
    description: (
      <Stack gap={2}>
        <TypoB2Regular className="text-text-secondary">
          Your deployment never breaks. You get a 30-day grace period with
          full features and clear warnings, and after that the install steps
          down gently to the free community edition. Everything you deployed
          keeps running; everything you built stays yours.
        </TypoB2Regular>
        <TypoB2Regular className="text-text-secondary">
          Renewing at any point — even after expiry — restores the licensed
          capabilities instantly, with no reinstall.
        </TypoB2Regular>
      </Stack>
    ),
  },
  {
    title: 'Do I need a credit card for the free tier?',
    description: (
      <Box>
        <TypoB2Regular className="text-text-secondary">
          No. The free tier has no card on file, so there is nothing to bill —
          at its limits (for example, {FREE_TIER_SEATS} seats) it simply
          pauses new additions. Upgrading is a choice you make, never a
          surprise on a statement.
        </TypoB2Regular>
      </Box>
    ),
  },
  {
    title: 'How does the 30-day evaluation work on my own cluster?',
    description: (
      <Box>
        <TypoB2Regular className="text-text-secondary">
          You get a self-serve evaluation key — no card, no sales call — that
          unlocks every capability on your own cluster for {EVALUATION_DAYS}{' '}
          days. When it ends, the install steps down gently to the community
          edition through the same never-breaks ladder a paid license uses.
          Nothing you built is ever locked or lost.{' '}
          <EvaluationClaimLink />{' '}
          — enter your email and it arrives in your inbox.
        </TypoB2Regular>
      </Box>
    ),
  },
  {
    title: 'How do prices work outside the US?',
    description: (
      <Stack gap={2}>
        <TypoB2Regular className="text-text-secondary">
          Prices are set for each market, never converted from the US number —
          an Indian rupee price is a real India price, not a converted dollar
          sticker plus an exchange fee. Use the market control at the top of
          this page to view any market&apos;s prices.
        </TypoB2Regular>
        <TypoB2Regular className="text-text-secondary">
          Tax follows each market&apos;s convention: in the US and Canada,
          sales tax is added at checkout based on your location; everywhere
          else, the sticker price already includes tax such as GST or VAT.
          Businesses can enter a VAT/GST ID at checkout for the correct
          business treatment and a compliant invoice.
        </TypoB2Regular>
      </Stack>
    ),
  },
  {
    title: 'How do AI credits work?',
    description: (
      <Stack gap={2}>
        <TypoB2Regular className="text-text-secondary">
          AI usage is prepaid: you load a credit pack, usage draws it down,
          and at zero the AI pauses until you reload — it can never run up a
          bill behind your back. Every movement on your balance is a visible,
          explainable entry.
        </TypoB2Regular>
        <TypoB2Regular className="text-text-secondary">
          Auto-reload is opt-in and requires a monthly ceiling you set. Past
          the ceiling, usage pauses like any prepaid account.
        </TypoB2Regular>
        <TypoB2Regular className="text-text-secondary">
          Credits fund the assistant on planton.ai organizations and in the
          Planton desktop app. Self-hosted deployments don&apos;t use
          credits — see the next question.
        </TypoB2Regular>
      </Stack>
    ),
  },
  {
    title: 'Does the AI Assistant work on my own infrastructure?',
    description: (
      <Stack gap={2}>
        <TypoB2Regular className="text-text-secondary">
          Not yet. Today the assistant runs on planton.ai organizations and
          in the Planton desktop app; self-hosted deployments don&apos;t
          include it yet.
        </TypoB2Regular>
        <TypoB2Regular className="text-text-secondary">
          It is coming: the assistant will run inside your own cluster using
          your own LLM provider key, so nothing leaves your infrastructure
          and no Planton credits are involved.
        </TypoB2Regular>
      </Stack>
    ),
  },
  {
    title: 'Can I cancel or get a refund?',
    description: (
      <Stack gap={2}>
        <TypoB2Regular className="text-text-secondary">
          Cancel anytime — no forms, no interviews. Subscriptions run out
          their paid period and stop. Licenses carry a 14-day money-back
          guarantee. Unused AI credit packs are refundable within 14 days of
          purchase.
        </TypoB2Regular>
        <TypoB2Regular className="text-text-secondary">
          To stop a license auto-renewal, email{' '}
          <Link className="text-white underline" href="mailto:support@planton.ai">
            support@planton.ai
          </Link>{' '}
          from your purchase email. The full details live in our{' '}
          <Link className="text-white underline" href="/refund-policy">
            refund policy
          </Link>
          .
        </TypoB2Regular>
      </Stack>
    ),
  },
];

export const Faqs: FC = () => {
  const [expanded, setExpanded] = useState<number | false>(false);

  const handleChange = (panel: number) => (event: React.SyntheticEvent, newExpanded: boolean) => {
    setExpanded(newExpanded ? panel : false);
  };

  return (
    <Box className="bg-[#0a0a0a] relative">
      <Stack className="w-full items-center py-12 z-50 px-4 md:px-8">
        <Box className="w-full max-w-7xl px-6 md:px-10 py-10 rounded-xl bg-[#151515] border border-[#2a2a2a]">
          <Stack className="gap-8">
            <TypoH2 className="text-center">Curious About Our Pricing?</TypoH2>
            <Stack className="gap-4">
              {faqs.map((faq, index) => (
                <Accordion
                  key={index}
                  expanded={expanded === index}
                  onChange={handleChange(index)}
                  variant="outlined"
                  className="bg-inherit before:content-none after:content-[''] after:block after:border-t after:border-[1px] after:border-solid after:border-[#373737]"
                >
                  <AccordionSummary className="px-0">
                    <Stack className="flex flex-row gap-2">
                      <Add />
                      <Typography>{faq.title}</Typography>
                    </Stack>
                  </AccordionSummary>
                  <AccordionDetails>{faq.description}</AccordionDetails>
                </Accordion>
              ))}
            </Stack>
          </Stack>
        </Box>
      </Stack>
    </Box>
  );
};
