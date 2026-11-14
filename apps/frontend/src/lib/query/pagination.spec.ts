import { describe, expect, it } from 'vitest';
import { flattenUniquePages, nextOffset } from './pagination';

describe('nextOffset', () => {
  it('advances by the number of consumed rows', () => {
    expect(nextOffset(true, 20, 40)).toBe(60);
  });

  it('ends pagination when the server reports no more rows', () => {
    expect(nextOffset(false, 20, 40)).toBeUndefined();
  });

  it('ends pagination when a page consumed no rows', () => {
    expect(nextOffset(true, 0, 40)).toBeUndefined();
  });
});

describe('flattenUniquePages', () => {
  const key = (item: { id: string }) => item.id;

  it('returns an empty list before the first page loads', () => {
    expect(
      flattenUniquePages(undefined, (page: { rows: { id: string }[] }) => page.rows, key)
    ).toEqual([]);
  });

  it('keeps the first item when pages repeat a key', () => {
    const data = {
      pages: [
        {
          rows: [
            { id: 'a', page: 1 },
            { id: 'b', page: 1 }
          ]
        },
        {
          rows: [
            { id: 'b', page: 2 },
            { id: 'c', page: 2 }
          ]
        }
      ]
    };
    expect(flattenUniquePages(data, (page) => page.rows, key)).toEqual([
      { id: 'a', page: 1 },
      { id: 'b', page: 1 },
      { id: 'c', page: 2 }
    ]);
  });
});
