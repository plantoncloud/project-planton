'use client';

import { FC, useState, useCallback } from 'react';
import { Box, Typography, IconButton } from '@mui/material';
import { ContentCopy, Check } from '@mui/icons-material';
import { motion, AnimatePresence } from 'framer-motion';

export interface CodeTab {
  label: string;
  code: string;
  /**
   * One or two sentences about what the code does, rendered under the block
   * in prose. Keeps teaching out of the code itself, so what a person copies
   * is only what they type.
   */
  description?: string;
}

interface CodeTabsProps {
  tabs: CodeTab[];
  title?: string;
  className?: string;
  /** For tabs that hold one or two lines; the block sizes to its content instead of reserving a screen of code. */
  compact?: boolean;
}

export const CodeTabs: FC<CodeTabsProps> = ({ tabs, title, className = '', compact = false }) => {
  const [active, setActive] = useState(0);
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(tabs[active].code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [tabs, active]);

  return (
    <Box className={`rounded-xl bg-[#0d0d0d] border border-[#2a2a2a] overflow-hidden ${className}`}>
      <Box className="flex items-center justify-between border-b border-[#2a2a2a] bg-[#1a1a1a]">
        <Box className="flex items-center">
          {title && (
            <Typography className="text-xs text-[#555] px-4 py-2.5 border-r border-[#2a2a2a]">
              {title}
            </Typography>
          )}
          <Box className="flex overflow-x-auto">
            {tabs.map((tab, i) => (
              <button
                key={tab.label}
                onClick={() => setActive(i)}
                className={`
                  px-4 py-2.5 text-xs font-medium transition-colors duration-200
                  border-b-2 -mb-px whitespace-nowrap
                  ${
                    active === i
                      ? 'text-white border-white bg-white/5'
                      : 'text-[#666] border-transparent hover:text-[#999] hover:bg-white/[0.02]'
                  }
                `}
              >
                {tab.label}
              </button>
            ))}
          </Box>
        </Box>
        <IconButton
          onClick={handleCopy}
          size="small"
          className="!text-[#555] hover:!text-[#999] !mr-2"
        >
          {copied ? <Check sx={{ fontSize: 14 }} /> : <ContentCopy sx={{ fontSize: 14 }} />}
        </IconButton>
      </Box>

      <Box className={`p-4 font-mono text-[13px] leading-relaxed relative ${compact ? 'min-h-[72px]' : 'min-h-[200px]'}`}>
        <AnimatePresence mode="wait">
          <motion.pre
            key={active}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={{ duration: 0.2 }}
            className="text-[#b0b0b0] whitespace-pre-wrap overflow-x-auto"
          >
            {tabs[active].code}
          </motion.pre>
        </AnimatePresence>
      </Box>
      {tabs[active].description && (
        <Typography className="px-4 py-3 border-t border-[#2a2a2a] text-sm text-[#a0a0a0] leading-relaxed">
          {tabs[active].description}
        </Typography>
      )}
    </Box>
  );
};
