/**
 * One collapsible group of a {@link VirtualGroupedList}, such as a member
 * presence group or a file date group.
 */
export interface VirtualListGroup<T extends { id: string }> {
  /** Stable group identifier, unique within the list. */
  id: string;
  label: string;
  /** Group rows in display order. Row IDs must be unique across all groups. */
  items: T[];
  /** Unique storage key for the persisted collapsed state. */
  persistKey: string;
  /** Collapsed state when no preference is stored. */
  defaultCollapsed?: boolean;
  /** Optional stable selector for the disclosure button. */
  testid?: string;
}

/** A group whose collapsed state has been read from `roomGroupCollapse`. */
export type ResolvedListGroup<T extends { id: string }> = VirtualListGroup<T> & {
  collapsed: boolean;
};

/**
 * One entry of a flattened grouped list. Headings and rows share one flat list
 * so a single virtualizer covers every group.
 */
export type GroupedListItem<T extends { id: string }> =
  | {
      type: 'header';
      key: string;
      group: ResolvedListGroup<T>;
      /** True when the heading draws the divider above its group: every heading after the first, or all headings with `separateFirst`. */
      separated: boolean;
    }
  | {
      type: 'row';
      key: string;
      groupId: string;
      row: T;
      /** True for the final row of its group, which receives the group's bottom padding. */
      last: boolean;
    };

/**
 * Flattens groups into virtualizer items in display order. A collapsed group
 * contributes only its heading. Row keys derive from row IDs, so a row that
 * moves between groups keeps its identity.
 */
export function buildGroupedListItems<T extends { id: string }>(
  groups: readonly ResolvedListGroup<T>[],
  { separateFirst = false }: { separateFirst?: boolean } = {}
): GroupedListItem<T>[] {
  const items: GroupedListItem<T>[] = [];
  groups.forEach((group, groupIndex) => {
    items.push({
      type: 'header',
      key: `group:${group.id}`,
      group,
      separated: separateFirst || groupIndex > 0
    });
    if (group.collapsed) return;
    group.items.forEach((row, index) => {
      items.push({
        type: 'row',
        key: `row:${row.id}`,
        groupId: group.id,
        row,
        last: index === group.items.length - 1
      });
    });
  });
  return items;
}
