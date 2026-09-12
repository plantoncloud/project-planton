'use client';

import { FC, useCallback, useState } from 'react';
import { Box, IconButton } from '@mui/material';
import { Check, ContentCopy } from '@mui/icons-material';

interface CommandBlockProps {
  /** One command per line; copied together, separated by newlines. */
  commands: readonly string[];
  /** Screen-reader name for the copy button. */
  label: string;
  className?: string;
}

// Several commands a person types in order, with one copy affordance for the
// lot, in a chrome bar above the text like CodeTabs so it never sits on a
// command. Long lines wrap between tokens first: each space-separated token is
// an inline block, so a path or a flag value moves to the next line whole
// rather than splitting at one of its own hyphens, and only a token wider than
// the block itself is allowed to break inside (overflow-wrap: anywhere) --
// nothing is ever clipped. What is copied is the plain text, not the markup.
// The single-line sibling is CopyCommand; the tabbed one is the shared
// CodeTabs.
export const CommandBlock: FC<CommandBlockProps> = ({ commands, label, className = '' }) => {
  const [copied, setCopied] = useState(false);
  const text = commands.join('\n');

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [text]);

  return (
    <Box className={`rounded-lg bg-[#0d0d0d] border border-[#2a2a2a] overflow-hidden ${className}`}>
      <Box className="flex items-center justify-between h-8 pl-3 pr-1 border-b border-[#2a2a2a] bg-[#1a1a1a]">
        <span className="text-[11px] text-[#666]">Terminal</span>
        <IconButton
          onClick={handleCopy}
          size="small"
          aria-label={copied ? 'Copied' : label}
          className="!text-[#666] hover:!text-[#ededed]"
        >
          {copied ? <Check sx={{ fontSize: 14 }} /> : <ContentCopy sx={{ fontSize: 14 }} />}
        </IconButton>
      </Box>
      <Box component="pre" className="p-4 font-mono text-xs leading-relaxed text-[#b0b0b0] whitespace-pre-wrap">
        {commands.map((command, i) => (
          <span key={command} className="block">
            {command.split(' ').map((token, j) => (
              <span key={`${i}-${j}`} className="inline-block [overflow-wrap:anywhere]">
                {j > 0 ? ' ' : ''}
                {token}
              </span>
            ))}
          </span>
        ))}
      </Box>
    </Box>
  );
};
