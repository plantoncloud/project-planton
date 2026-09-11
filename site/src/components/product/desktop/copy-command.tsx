'use client';

import { FC, useCallback, useState } from 'react';
import { Box, IconButton, Typography } from '@mui/material';
import { Check, ContentCopy } from '@mui/icons-material';

interface CopyCommandProps {
  command: string;
  /** Screen-reader name for the copy button, e.g. "Copy the Homebrew command". */
  label: string;
  className?: string;
}

// One shell command with a copy button: the affordance a person expects next
// to `brew install ...`. Mirrors CodeTabs' copy behavior (two-second
// acknowledgement) for a single line, so the two never feel like different
// products. Rendered as a real <button>, keyboard-reachable, with the theme's
// focus ring.
export const CopyCommand: FC<CopyCommandProps> = ({ command, label, className = '' }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(command).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [command]);

  return (
    <Box
      className={`inline-flex items-center gap-2 pl-4 pr-1.5 py-1.5 rounded-lg border border-[#2a2a2a] bg-[#111] max-w-full ${className}`}
    >
      <Typography component="code" className="text-xs md:text-sm text-[#ededed] font-mono truncate">
        {command}
      </Typography>
      <IconButton
        onClick={handleCopy}
        size="small"
        aria-label={copied ? 'Copied' : label}
        className="!text-[#666] hover:!text-[#ededed]"
      >
        {copied ? <Check sx={{ fontSize: 14 }} /> : <ContentCopy sx={{ fontSize: 14 }} />}
      </IconButton>
    </Box>
  );
};
