/** File references in investigation answers: `path:line` or `path:start-end`. The host checks
 * them against the investigated directory and can turn the valid ones into links. */
import { readFile, realpath, stat } from 'node:fs/promises';
import { isAbsolute, relative, resolve } from 'node:path';

export interface Citation {
  path: string;
  start: number;
  end: number;
}

export interface CheckedCitation extends Citation {
  /** Why the reference is invalid. Absent for a valid reference. */
  problem?: string;
}

/** A relative file path with an extension, then `:line` or `:start-end`. A path inside a URL
 * does not match, because a `/` or `:` before it is not allowed. */
const REFERENCE = /(?<![\w/.:@-])((?:[\w@.-]+\/)*[\w@.-]*\.[A-Za-z0-9]+):(\d+)(?:-(\d+))?(?!\d)/g;

/** The file references in a text, each one once, in order of first use. */
export function findCitations(text: string): Citation[] {
  const seen = new Map<string, Citation>();
  for (const [, path, start, end] of text.matchAll(REFERENCE)) {
    const citation = { path: path!, start: Number(start), end: Number(end ?? start) };
    seen.set(`${citation.path}:${citation.start}-${citation.end}`, citation);
  }
  return [...seen.values()];
}

/** Check that each reference names lines of a file inside `directory`. */
export async function checkCitations(
  directory: string,
  citations: readonly Citation[]
): Promise<CheckedCitation[]> {
  const root = await realpath(directory);
  const lineCounts = new Map<string, number | string>();
  const lineCount = async (path: string): Promise<number | string> => {
    if (isAbsolute(path) || path.split('/').includes('..')) return 'is not a relative path';
    try {
      const file = await realpath(resolve(root, path));
      const local = relative(root, file);
      if (!local || local.startsWith('..') || isAbsolute(local)) return 'is outside the directory';
      const info = await stat(file);
      if (!info.isFile()) return 'is not a file';
      if (info.size > 5_000_000) return 'is too large to check';
      const text = await readFile(file, 'utf8');
      return text.endsWith('\n') ? text.split('\n').length - 1 : text.split('\n').length;
    } catch {
      return 'does not exist';
    }
  };
  const checked: CheckedCitation[] = [];
  for (const citation of citations) {
    if (!lineCounts.has(citation.path))
      lineCounts.set(citation.path, await lineCount(citation.path));
    const count = lineCounts.get(citation.path)!;
    const problem =
      typeof count === 'string'
        ? `${citation.path} ${count}`
        : citation.start < 1 || citation.end < citation.start || citation.end > count
          ? `${citation.path} has ${count} lines; lines ${citation.start}-${citation.end} do not exist`
          : undefined;
    checked.push({ ...citation, ...(problem ? { problem } : {}) });
  }
  return checked;
}

/** Fenced code blocks and inline code spans. */
const CODE = /(```[\s\S]*?```|`[^`\n]+`)/;

/**
 * Replace valid references with links to the lines on GitHub at a fixed commit. A code span
 * that holds only a reference becomes a link; references in other code stay as they are.
 */
export function linkCitations(
  text: string,
  valid: readonly Citation[],
  { repository, commit }: { repository: string; commit: string }
): string {
  const known = new Set(valid.map(({ path, start, end }) => `${path}:${start}-${end}`));
  const link = (reference: string, path: string, start: string, end: string | undefined) => {
    const [first, last] = [Number(start), Number(end ?? start)];
    if (!known.has(`${path}:${first}-${last}`)) return undefined;
    const lines = first === last ? `L${first}` : `L${first}-L${last}`;
    return `[\`${reference}\`](https://github.com/${repository}/blob/${commit}/${path}#${lines})`;
  };
  const whole = new RegExp(`^\`${REFERENCE.source}\`$`);
  return text
    .split(CODE)
    .map((part, index) => {
      // Odd parts are code.
      if (index % 2 === 1) {
        const match = whole.exec(part);
        return (match && link(part.slice(1, -1), match[1]!, match[2]!, match[3])) ?? part;
      }
      return part.replace(
        REFERENCE,
        (reference, path: string, start: string, end?: string) =>
          link(reference, path, start, end) ?? reference
      );
    })
    .join('');
}
