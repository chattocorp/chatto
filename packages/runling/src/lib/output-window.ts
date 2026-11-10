/** Bound a continuous log's text cache while the journal retains complete scrollback.
 * Callers remove entries opposite the visible edge, and let Virtua anchor each change. */
import type { OutputEntry } from './log-page.ts';

export const OUTPUT_CACHE_ENTRIES = 1000;
export const OUTPUT_CACHE_BYTES = 1024 * 1024;

/** Evict the requested edge without changing entry identity or order. Journal entries are
 * individually smaller than the byte budget. The caller preserves the viewport's edge. */
export function trimOutputWindow(
  entries: readonly OutputEntry[],
  edge: 'start' | 'end',
  limits = { entries: OUTPUT_CACHE_ENTRIES, bytes: OUTPUT_CACHE_BYTES }
): { entries: OutputEntry[]; removed: number } {
  const encoder = new TextEncoder();
  const sizes = entries.map((entry) => encoder.encode(entry.text).byteLength);
  let bytes = sizes.reduce((total, size) => total + size, 0);
  let start = 0;
  let end = entries.length;
  while (end > start && (end - start > limits.entries || bytes > limits.bytes)) {
    bytes -= edge === 'start' ? sizes[start++]! : sizes[--end]!;
  }
  return { entries: entries.slice(start, end), removed: start + entries.length - end };
}
