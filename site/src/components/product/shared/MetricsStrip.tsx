'use client';

import { FC, useRef, useEffect, useState } from 'react';
import { Box, Typography } from '@mui/material';
import { motion, useInView, useReducedMotion } from 'framer-motion';

export interface MetricItem {
  value: string;
  label: string;
}

interface MetricsStripProps {
  metrics: MetricItem[];
  className?: string;
}

const AnimatedValue: FC<{ value: string; animate: boolean }> = ({ value, animate }) => {
  const [displayed, setDisplayed] = useState(value);

  useEffect(() => {
    const match = value.match(/^([<>~]?\s*)(\d+)(\+?%?\s*.*)$/);
    if (!animate || !match) return;

    const prefix = match[1];
    const target = parseInt(match[2], 10);
    const suffix = match[3];
    const duration = 1200;
    const startTime = performance.now();
    let raf: number;

    const tick = (now: number) => {
      const elapsed = now - startTime;
      const progress = Math.min(elapsed / duration, 1);
      const eased = 1 - Math.pow(1 - progress, 3);
      const current = Math.round(target * eased);
      setDisplayed(`${prefix}${current}${suffix}`);
      if (progress < 1) raf = requestAnimationFrame(tick);
    };

    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [animate, value]);

  return <>{displayed}</>;
};

export const MetricsStrip: FC<MetricsStripProps> = ({ metrics, className = '' }) => {
  const ref = useRef<HTMLDivElement>(null);
  const isInView = useInView(ref, { once: true, margin: '-40px' });
  // A reader who asked for less motion sees the figures at once, at their
  // final value: the count-up and the rise are decoration, and a strip whose
  // numbers only appear on scroll is an empty band to anyone who never
  // triggers it.
  const reduceMotion = useReducedMotion();
  const shown = isInView || Boolean(reduceMotion);

  return (
    <Box
      component="section"
      ref={ref}
      className={`w-full py-10 md:py-14 px-4 md:px-8 bg-[#111] ${className}`}
    >
      <Box className="max-w-5xl mx-auto">
        <Box className="grid grid-cols-2 md:grid-cols-4 gap-8 md:gap-4">
          {metrics.map((metric, i) => (
            <motion.div
              key={metric.label}
              initial={reduceMotion ? false : { opacity: 0, y: 16 }}
              animate={shown ? { opacity: 1, y: 0 } : { opacity: 0, y: 16 }}
              transition={{ duration: 0.5, delay: i * 0.12, ease: 'easeOut' }}
              className="text-center"
            >
              <Typography className="text-2xl md:text-3xl lg:text-4xl font-bold text-white tracking-tight">
                <AnimatedValue value={metric.value} animate={shown && !reduceMotion} />
              </Typography>
              <Typography className="text-xs md:text-sm text-[#666] mt-1.5 font-medium uppercase tracking-wider">
                {metric.label}
              </Typography>
            </motion.div>
          ))}
        </Box>
      </Box>
    </Box>
  );
};
