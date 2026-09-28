import { describe, expect, it } from 'vitest';
import { buildGroupedListItems, type ResolvedListGroup } from './groupedListItems';

function group(id: string, rowIds: string[], collapsed = false): ResolvedListGroup<{ id: string }> {
  return {
    id,
    label: id,
    items: rowIds.map((rowId) => ({ id: rowId })),
    persistKey: `test:${id}`,
    collapsed
  };
}

function describeItems(items: ReturnType<typeof buildGroupedListItems>): string[] {
  return items.map((item) =>
    item.type === 'header'
      ? `header:${item.group.id}${item.separated ? ':separated' : ''}`
      : `row:${item.row.id}${item.last ? ':last' : ''}`
  );
}

describe('buildGroupedListItems', () => {
  it('flattens headings and rows in display order', () => {
    const items = buildGroupedListItems([group('online', ['a', 'b']), group('offline', ['c'])]);

    expect(describeItems(items)).toEqual([
      'header:online',
      'row:a',
      'row:b:last',
      'header:offline:separated',
      'row:c:last'
    ]);
  });

  it('omits the rows of collapsed groups', () => {
    const items = buildGroupedListItems([group('online', ['a']), group('offline', ['b'], true)]);

    expect(describeItems(items)).toEqual([
      'header:online',
      'row:a:last',
      'header:offline:separated'
    ]);
  });

  it('separates the first group on request', () => {
    const items = buildGroupedListItems([group('online', ['a'])], { separateFirst: true });

    expect(describeItems(items)).toEqual(['header:online:separated', 'row:a:last']);
  });

  it('keys rows by row ID so a moved row keeps its identity', () => {
    const before = buildGroupedListItems([group('online', ['a']), group('offline', ['b'])]);
    const after = buildGroupedListItems([group('online', []), group('offline', ['a', 'b'])]);
    const key = (items: typeof before, id: string) =>
      items.find((item) => item.type === 'row' && item.row.id === id)?.key;

    expect(key(after, 'a')).toBe(key(before, 'a'));
    expect(new Set(after.map((item) => item.key)).size).toBe(after.length);
  });
});
