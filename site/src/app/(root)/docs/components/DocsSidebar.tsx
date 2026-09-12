'use client';

import { FC, useState, useCallback, useEffect, useRef, useMemo } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { Box, Typography, Chip, IconButton } from '@mui/material';
import {
  OpenInNew as ExternalLinkIcon
} from '@mui/icons-material';
import {
  KeyboardArrowRight as CollapseIcon,
  KeyboardArrowDown as ExpandIcon
} from '@mui/icons-material';
import { DocItem } from '@/app/(root)/docs/utils/fileSystem';
import {
  SIDEBAR_BADGE_COLORS,
  SIDEBAR_BADGE_DEFAULT,
  SIDEBAR_ACTIVE_CLASSES,
  SIDEBAR_ITEM_CLASSES,
} from '@/theme/docs';

interface DocsSidebarProps {
  structure: DocItem[];
  onNavigate?: () => void;
}

interface SidebarItemProps {
  item: DocItem;
  level?: number;
  onNavigate?: () => void;
  expandedPaths: Set<string>;
  onToggle: (path: string) => void;
}

const SidebarItem: FC<SidebarItemProps> = ({
  item,
  level = 0,
  onNavigate,
  expandedPaths,
  onToggle
}) => {
  const pathname = usePathname();
  const isActive = pathname === `/docs/${item.path}`;

  const handleNavigate = () => {
    if (onNavigate) {
      onNavigate();
    }
  };

  const renderBadge = () => {
    if (!item.badge) return null;

    const colorClass = SIDEBAR_BADGE_COLORS[item.badge] || SIDEBAR_BADGE_DEFAULT;

    return (
      <Chip
        label={item.badge}
        size="small"
        className={`ml-2 text-xs ${colorClass}`}
      />
    );
  };

  if (item.type === 'directory') {
    const isExpanded = expandedPaths.has(item.path);
    return (
      <Box>
        <Box
          className="flex items-center justify-between px-4 py-2 hover:bg-white/5"
          {...(isActive ? { 'data-active': 'true' } : {})}
        >
          <Box className="flex items-center flex-1">
            {item.hasIndex ? (
              <Link
                href={`/docs/${item.path}`}
                onClick={handleNavigate}
                className="flex-1"
              >
                <Typography className={`text-sm font-medium ${isActive ? 'text-white' : `${SIDEBAR_ITEM_CLASSES} hover:text-white`}`}>
                  {item.sidebarTitle || item.title || formatName(item.name)}
                </Typography>
              </Link>
            ) : (
              <Typography className={`${SIDEBAR_ITEM_CLASSES} text-sm font-medium`}>
                {item.sidebarTitle || item.title || formatName(item.name)}
              </Typography>
            )}
            {renderBadge()}
          </Box>
          {(item.children?.length ?? 0) > 0 && (
            <IconButton
              size="small"
              aria-label={isExpanded ? 'Collapse section' : 'Expand section'}
              aria-expanded={isExpanded}
              onClick={() => onToggle(item.path)}
              className={SIDEBAR_ITEM_CLASSES}
            >
              {isExpanded ? <ExpandIcon fontSize="small" /> : <CollapseIcon fontSize="small" />}
            </IconButton>
          )}
        </Box>
        {isExpanded && (
          <Box className="ml-4">
            {item.children?.map((child, index) => (
              <SidebarItem
                key={index}
                item={child}
                level={level + 1}
                onNavigate={onNavigate}
                expandedPaths={expandedPaths}
                onToggle={onToggle}
              />
            ))}
          </Box>
        )}
      </Box>
    );
  }

  if (item.isExternal && item.externalUrl) {
    return (
      <a
        href={item.externalUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="block"
      >
        <Box className={`flex items-center px-4 py-2 hover:bg-white/5 cursor-pointer ${SIDEBAR_ITEM_CLASSES}`}>
          <Typography className="text-sm flex-1">
            {item.sidebarTitle || item.title || formatName(item.name)}
          </Typography>
          <ExternalLinkIcon className="text-gray-500" sx={{ fontSize: 14 }} />
          {renderBadge()}
        </Box>
      </a>
    );
  }

  return (
    <Link href={`/docs/${item.path}`} onClick={handleNavigate}>
      <Box
        className={`flex items-center px-4 py-2 hover:bg-white/5 cursor-pointer ${isActive ? SIDEBAR_ACTIVE_CLASSES : SIDEBAR_ITEM_CLASSES}`}
        {...(isActive ? { 'data-active': 'true' } : {})}
      >
        <Typography className="text-sm flex-1">
          {item.sidebarTitle || item.title || formatName(item.name)}
        </Typography>
        {renderBadge()}
      </Box>
    </Link>
  );
};

function formatName(name: string): string {
  return name
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, l => l.toUpperCase())
    .replace(/\s+/g, ' ')
    .trim();
}

function getAncestorPaths(docPath: string): Set<string> {
  const result = new Set<string>();
  const segments = docPath.split('/').filter(Boolean);
  let acc = '';
  for (const segment of segments) {
    acc = acc ? `${acc}/${segment}` : segment;
    result.add(acc);
  }
  return result;
}

export const DocsSidebar: FC<DocsSidebarProps> = ({ structure, onNavigate }) => {
  const pathname = usePathname();
  const sidebarRef = useRef<HTMLDivElement>(null);

  const currentDocPath = useMemo(() => {
    const prefix = '/docs/';
    return pathname.startsWith(prefix) ? pathname.slice(prefix.length) : '';
  }, [pathname]);

  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(
    () => getAncestorPaths(currentDocPath)
  );

  useEffect(() => {
    if (!currentDocPath) return;
    const raf = requestAnimationFrame(() => {
      setExpandedPaths((prev) => {
        const ancestors = getAncestorPaths(currentDocPath);
        let allPresent = true;
        for (const a of ancestors) {
          if (!prev.has(a)) { allPresent = false; break; }
        }
        if (allPresent) return prev;
        const next = new Set(prev);
        for (const a of ancestors) next.add(a);
        return next;
      });
    });
    return () => cancelAnimationFrame(raf);
  }, [currentDocPath]);

  useEffect(() => {
    const container = sidebarRef.current;
    if (!container) return;
    const raf = requestAnimationFrame(() => {
      const activeEl = container.querySelector('[data-active="true"]');
      if (activeEl) {
        activeEl.scrollIntoView({ block: 'nearest' });
      }
    });
    return () => cancelAnimationFrame(raf);
  }, [currentDocPath]);

  const handleToggle = useCallback((path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  return (
    <Box ref={sidebarRef} className="h-full overflow-y-auto">
      <Box className="py-2">
        {structure.map((item, index) => (
          <SidebarItem
            key={index}
            item={item}
            onNavigate={onNavigate}
            expandedPaths={expandedPaths}
            onToggle={handleToggle}
          />
        ))}
      </Box>
    </Box>
  );
};
