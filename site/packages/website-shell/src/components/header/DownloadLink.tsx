'use client';

import type { FC } from 'react';
import Link from 'next/link';
import { Button } from '@mui/material';
import { Download as DownloadIcon } from '@mui/icons-material';

// The persistent way to the desktop app from any page. A quiet text link, not
// a second primary: the header's one white button stays "Sign up", and the
// download page itself carries the platform-named primary. Sits beside the
// auth cluster rather than inside it so AuthButtons keeps its single job
// (who you are), and this keeps its own (how you get the app).
const linkSx = {
  fontSize: '0.875rem',
  fontWeight: 500,
  textTransform: 'none',
  borderRadius: '10px',
  gap: 0.75,
} as const;

export const DesktopDownloadLink: FC = () => (
  <Button
    LinkComponent={Link}
    href="/download"
    startIcon={<DownloadIcon sx={{ fontSize: 16 }} />}
    style={{ color: '#a0a0a0' }}
    sx={{ ...linkSx, display: { xs: 'none', md: 'inline-flex' }, '&:hover': { color: '#ededed' } }}
  >
    Download
  </Button>
);

export const MobileDownloadLink: FC = () => (
  <Button
    LinkComponent={Link}
    href="/download"
    startIcon={<DownloadIcon sx={{ fontSize: 16 }} />}
    style={{ color: '#a0a0a0' }}
    sx={{ ...linkSx, width: '100%', justifyContent: 'center' }}
  >
    Download Desktop
  </Button>
);
