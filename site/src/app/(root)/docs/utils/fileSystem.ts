import fs from 'fs';
import path from 'path';
import matter from 'gray-matter';
import { generateExcerptFromContent } from '@/lib/utils';

const DOCS_DIRECTORY = path.join(/* turbopackIgnore: true */ process.cwd(), 'public', 'docs');

export interface DocItem {
  name: string;
  type: 'file' | 'directory';
  path: string;
  children?: DocItem[];
  // Enhanced properties for dynamic sidebar
  title?: string;
  description?: string;
  icon?: string;
  category?: string;
  order?: number;
  badge?: string; // For "Popular", "Beta", etc.
  isExternal?: boolean;
  externalUrl?: string;
  hasIndex?: boolean; // For directories with index files
  sidebarTitle?: string; // Optional shorter label for sidebar navigation (from sidebar_title frontmatter)
  excerpt?: string;
}

export interface MarkdownContent {
  content: string;
  data: {
    title?: string;
    description?: string;
    icon?: string;
    category?: string;
    order?: number;
    badge?: string;
    isExternal?: boolean;
    externalUrl?: string;
    [key: string]: string | string[] | number | boolean | undefined;
  };
  isMdx?: boolean;
}

export async function getMarkdownContent(filePath: string): Promise<string> {
  const possiblePaths = [
    path.join(/* turbopackIgnore: true */ DOCS_DIRECTORY, `${filePath}.md`),
    path.join(/* turbopackIgnore: true */ DOCS_DIRECTORY, filePath, 'index.md'),
    path.join(/* turbopackIgnore: true */ DOCS_DIRECTORY, filePath, 'README.md'),
  ];

  for (const candidatePath of possiblePaths) {
    if (fs.existsSync(candidatePath)) {
      return fs.readFileSync(candidatePath, 'utf-8');
    }
  }

  const dirPath = path.join(/* turbopackIgnore: true */ DOCS_DIRECTORY, filePath);
  if (fs.existsSync(dirPath) && fs.statSync(dirPath).isDirectory()) {
    const files = fs.readdirSync(dirPath);
    const mdLikeFile = files.find((file) => file.endsWith('.md'));
    if (mdLikeFile) {
      return fs.readFileSync(path.join(/* turbopackIgnore: true */ dirPath, mdLikeFile), 'utf-8');
    }
  }

  throw new Error(`No markdown file found for path: ${filePath}`);
}

/**
 * Resolves a logical doc path to the actual file path relative to the docs directory.
 * Mirrors the resolution order of getMarkdownContent: {path}.md, {path}/index.md, {path}/README.md.
 * Returns the relative path including the .md extension (e.g. "platform/index.md", "cli.md").
 */
export function resolveDocFilePath(filePath: string): string {
  const candidates = [
    `${filePath}.md`,
    path.join(filePath, 'index.md'),
    path.join(filePath, 'README.md'),
  ];

  for (const candidate of candidates) {
    if (fs.existsSync(path.join(/* turbopackIgnore: true */ DOCS_DIRECTORY, candidate))) {
      return candidate;
    }
  }

  const dirPath = path.join(/* turbopackIgnore: true */ DOCS_DIRECTORY, filePath);
  if (fs.existsSync(dirPath) && fs.statSync(dirPath).isDirectory()) {
    const files = fs.readdirSync(dirPath);
    const mdFile = files.find((file) => file.endsWith('.md'));
    if (mdFile) {
      return path.join(filePath, mdFile);
    }
  }

  // Last resort: assume {path}.md (caller will get a 404 if it doesn't exist)
  return `${filePath}.md`;
}

export async function getDocumentationStructure(): Promise<DocItem[]> {
  return buildStructure(DOCS_DIRECTORY);
}

function buildStructure(dirPath: string, relativePath: string = ''): DocItem[] {
  if (!fs.existsSync(dirPath)) {
    return [];
  }

  const items = fs.readdirSync(dirPath);
  const structure: DocItem[] = [];

  for (const item of items) {
    // Skip hidden files/directories and underscore-prefixed directories (e.g., _rules)
    if (item.startsWith('.') || item.startsWith('_')) {
      continue;
    }
    const fullPath = path.join(/* turbopackIgnore: true */ dirPath, item);
    const stat = fs.statSync(fullPath);
    const itemRelativePath = path.join(relativePath, item);

    if (stat.isDirectory()) {
      const children = buildStructure(fullPath, itemRelativePath);
      const indexFiles = ['index.md', 'README.md'];
      // A section is a directory with pages under it OR a directory whose
      // only page is its index: a one-page section (a topic that has not
      // grown children yet) is still a page a reader can open and a route
      // the export must emit. Dropping it here made such a page unreachable.
      const hasIndex = indexFiles.some((indexFile) => fs.existsSync(path.join(/* turbopackIgnore: true */ fullPath, indexFile)));
      if (children.length > 0 || hasIndex) {
        // Try to get metadata (and content for excerpt) from index/README (.md only)
        let metadata: MarkdownContent['data'] = {};
        let indexContent = '';

        for (const indexFile of indexFiles) {
          const indexPath = path.join(/* turbopackIgnore: true */ fullPath, indexFile);
          if (fs.existsSync(indexPath)) {
            try {
              indexContent = fs.readFileSync(indexPath, 'utf-8');
              const { data } = matter(indexContent);
              metadata = data;
              break;
            } catch (error) {
              console.warn(`Failed to parse metadata from ${indexPath}:`, error);
            }
          }
        }

        const category = relativePath.split('/')[0] || item;

        structure.push({
          name: item,
          type: 'directory',
          path: itemRelativePath,
          children,
          title: metadata.title || formatName(item),
          description: metadata.description,
          category,
          order: metadata.order || 0,
          badge: metadata.badge,
          isExternal: (metadata.isExternal as boolean) || false,
          externalUrl: metadata.externalUrl as string | undefined,
          hasIndex,
          sidebarTitle: (metadata.sidebar_title as string) || undefined,
          excerpt: hasIndex && indexContent ? generateExcerptFromContent(indexContent) : '',
        });
      }
    } else if (item.endsWith('.md')) {
      // Skip certain files that are not meant for documentation
      // Also skip index.md and README.md as they represent directory content
      if (
        !item.startsWith('prompt.') &&
        !item.startsWith('response.') &&
        !item.includes('.not-good.') &&
        !['index.md', 'README.md'].includes(item)
      ) {
        try {
          const fileContent = fs.readFileSync(fullPath, 'utf-8');
          const { data } = matter(fileContent);
          const category = relativePath.split('/')[0] || 'general';

          structure.push({
            name: item.replace(/\.md$/i, ''),
            type: 'file',
            path: itemRelativePath.replace(/\.md$/i, ''),
            title: (data.title as string) || formatName(item.replace(/\.md$/i, '')),
            description: data.description as string | undefined,
            category,
            order: (data.order as number) || 0,
            badge: data.badge as string | undefined,
            isExternal: (data.isExternal as boolean) || false,
            externalUrl: data.externalUrl as string | undefined,
            sidebarTitle: (data.sidebar_title as string) || undefined,
            excerpt: generateExcerptFromContent(fs.readFileSync(fullPath, 'utf-8'))
          });
        } catch (error) {
          console.warn(`Failed to parse metadata from ${fullPath}:`, error);
          const category = relativePath.split('/')[0] || 'general';
          structure.push({
            name: item.replace(/\.md$/i, ''),
            type: 'file',
            path: itemRelativePath.replace(/\.md$/i, ''),
            title: formatName(item.replace(/\.md$/i, '')),
            category,
            order: 0
          });
        }
      }
    }
  }

  // Sort by order first, then by type, then by name
  return structure.sort((a, b) => {
    // First by order
    if (a.order !== b.order) {
      return (a.order || 0) - (b.order || 0);
    }
    // Then by type (directories first)
    if (a.type !== b.type) {
      return a.type === 'directory' ? -1 : 1;
    }
    // Finally by name
    return a.name.localeCompare(b.name);
  });
}

export function getDocPathFromSlug(slug: string[]): string {
  return slug.join('/');
}

export function getSlugFromPath(filePath: string): string[] {
  return filePath.split('/').filter(Boolean);
}

function formatName(name: string): string {
  // Convert kebab-case or snake_case to Title Case
  return name
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (l) => l.toUpperCase())
    .replace(/\s+/g, ' ')
    .trim();
}

// Function to get the next documentation item
export function getNextDocItem(
  currentPath: string,
  structure: DocItem[]
): { title: string; excerpt: string; slug: string } | null {
  // Flatten the structure into reading order.
  // Directories with an index page are included before their children
  // so that section overview pages participate in prev/next navigation.
  const flattenItems = (items: DocItem[]): DocItem[] => {
    const result: DocItem[] = [];
    for (const item of items) {
      if (item.type === 'file') {
        result.push(item);
      } else if (item.type === 'directory') {
        if (item.hasIndex) {
          result.push(item);
        }
        if (item.children) {
          result.push(...flattenItems(item.children));
        }
      }
    }
    return result;
  };

  const allItems = flattenItems(structure);
  const currentIndex = allItems.findIndex((item) => item.path === currentPath);

  if (currentIndex === -1 || currentIndex === allItems.length - 1) {
    return null;
  }

  const nextDocItem = allItems[currentIndex + 1];

  return {
    title: nextDocItem.title || '',
    excerpt: nextDocItem.excerpt || '',
    slug: nextDocItem.path,
  };
}

/**
 * Generates static params for documentation routes based on the documentation structure.
 * This function can be reused across different documentation pages that need to generate
 * static routes for markdown files and directories.
 *
 * @param structure - The documentation structure to generate params from
 * @returns Array of slug parameters for static generation
 */
export function generateStaticParamsFromStructure(structure: DocItem[]): { slug: string[] }[] {
  const params: { slug: string[] }[] = [];

  // Add the root docs path
  params.push({ slug: [] });

  const addPaths = (items: DocItem[], currentPath: string[] = []) => {
    items.forEach((item) => {
      if (item.type === 'file') {
        // Add the clean route (without .md extension)
        params.push({ slug: [...currentPath, item.name] });


      } else if (item.type === 'directory') {
        // If directory has an index file, add a path for the directory itself
        if (item.hasIndex) {
          params.push({ slug: [...currentPath, item.name] });
        }
        // Recursively add paths for children
        addPaths(item.children || [], [...currentPath, item.name]);
      }
    });
  };

  addPaths(structure);
  // Include root /docs path for static export with catch-all route
  params.push({ slug: [] });

  return params;
}

/**
 * Processes documentation slug parameters to handle markdown extensions and clean paths.
 * This function can be reused across different documentation pages that need to process
 * slug parameters and handle both clean routes and .md extension routes.
 *
 * @param slug - The slug array from the route parameters
 * @returns Object containing processed slug information
 */
export function processDocumentationSlug(slug: string[] = []) {
  // Strip .md extensions from slug parts to handle both clean routes and .md routes
  const cleanSlug = slug.map((part) => part.replace(/\.md$/i, ''));
  const path = cleanSlug.join('/');

  return {
    originalSlug: slug,
    cleanSlug,
    path: path || 'index',
  };
}
