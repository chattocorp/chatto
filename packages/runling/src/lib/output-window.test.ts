import { expect, test } from 'vitest';
import { trimOutputWindow, OUTPUT_CACHE_BYTES, OUTPUT_CACHE_ENTRIES } from './output-window.ts';
import type { OutputEntry } from './log-page.ts';

const entry = (cursor: number, text = `line-${cursor}`): OutputEntry => ({
  cursor,
  timestamp: cursor,
  text
});

test('keeps the visible edge under count and byte limits without modifying source entries', () => {
  const rows = Array.from({ length: 1200 }, (_, index) => entry(index));
  const original = structuredClone(rows);
  const latest = trimOutputWindow(rows, 'start');
  const oldest = trimOutputWindow(rows, 'end');
  expect(latest.entries).toHaveLength(OUTPUT_CACHE_ENTRIES);
  expect(oldest.entries).toHaveLength(OUTPUT_CACHE_ENTRIES);
  expect(latest.entries[0]).toBe(rows[200]);
  expect(oldest.entries.at(-1)).toBe(rows[999]);
  expect(latest.removed).toBe(200);
  expect(rows).toEqual(original);
  for (const edge of ['start', 'end'] as const) {
    const large = rows.map((row) => ({ ...row, text: '🐈'.repeat(1000) }));
    const result = trimOutputWindow(large, edge);
    expect(result.entries.length).toBeLessThan(OUTPUT_CACHE_ENTRIES);
    expect(
      result.entries.reduce((size, row) => size + Buffer.byteLength(row.text), 0)
    ).toBeLessThanOrEqual(OUTPUT_CACHE_BYTES);
  }
});

test('scrolling both directions can reload evicted history without duplicates or gaps', () => {
  const journal = Array.from({ length: 5000 }, (_, index) => entry(index));
  let window = journal.slice(-200);
  const seen = new Set(window.map((row) => row.cursor));
  while (window[0]!.cursor > 0) {
    const first = window[0]!.cursor;
    const older = journal.slice(Math.max(0, first - 200), first);
    older.forEach((row) => seen.add(row.cursor));
    window = trimOutputWindow([...older, ...window], 'end').entries;
    expect(new Set(window.map((row) => row.cursor)).size).toBe(window.length);
  }
  expect(seen.size).toBe(journal.length);
  expect(window[0]!.cursor).toBe(0);
  while (window.at(-1)!.cursor < journal.length - 1) {
    const after = window.at(-1)!.cursor + 1;
    window = trimOutputWindow([...window, ...journal.slice(after, after + 200)], 'start').entries;
    expect(
      window.every((row, index) => !index || row.cursor === window[index - 1]!.cursor + 1)
    ).toBe(true);
  }
  expect(window.at(-1)!.cursor).toBe(journal.length - 1);
});

test('an empty or already bounded window retains every blank and repeated message', () => {
  expect(trimOutputWindow([], 'start')).toEqual({ entries: [], removed: 0 });
  const rows = [entry(1, ''), entry(2, 'repeat'), entry(3, 'repeat')];
  expect(trimOutputWindow(rows, 'end')).toEqual({ entries: rows, removed: 0 });
});
