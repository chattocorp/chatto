/** Helpers for offset-paginated infinite queries. */

/**
 * Return the offset of the next page, or `undefined` when there is none.
 *
 * A page that consumed no rows also ends pagination, so a server that reports
 * more rows without returning any cannot cause a request loop.
 */
export function nextOffset(hasMore: boolean, consumed: number, offset: number): number | undefined {
  return hasMore && consumed > 0 ? offset + consumed : undefined;
}

/**
 * Flatten the loaded pages into one list and keep the first item for each key.
 *
 * Offset pages can repeat a row when the server list changes between two page
 * reads. A repeated row must not render twice.
 */
export function flattenUniquePages<Page, Item>(
  data: { readonly pages: readonly Page[] } | undefined,
  items: (page: Page) => readonly Item[],
  key: (item: Item) => string
): Item[] {
  const seen = new Set<string>();
  return (data?.pages ?? []).flatMap((page) =>
    items(page).filter((item) => {
      const itemKey = key(item);
      if (seen.has(itemKey)) return false;
      seen.add(itemKey);
      return true;
    })
  );
}
