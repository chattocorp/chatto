<!--
@component

Domain-neutral dense matrix layout. The component owns axis geometry,
horizontal overflow, sticky row headings, and coordinated row/column
highlighting. Consumers provide all domain labels and cell content with
snippets.
-->
<script lang="ts" generics="TRow, TColumn">
  import type { Snippet } from 'svelte';
  import { DataTable } from '$lib/components/admin';
  import MatrixColumnHeading from './MatrixColumnHeading.svelte';

  let {
    rows,
    columns,
    getRowKey,
    getColumnKey,
    leadingHeader,
    rowHeader,
    columnHeader,
    cell,
    columnClass,
    columnAttributes,
    cellAttributes,
    isCellInteractive = () => true,
    emptyMessage,
    stickyHeader = false,
    fillHeight = false,
    stickyHeaderFadeOffset = 'top-0',
    trailingHeader,
    trailingCell,
    trailingColumns = 0,
    rowHeaderWidth = '14rem',
    columnHeaderHeight = '12rem',
    spacerTestId = 'matrix-spacer'
  }: {
    rows: TRow[];
    columns: TColumn[];
    getRowKey: (row: TRow) => string;
    getColumnKey: (column: TColumn) => string;
    leadingHeader: Snippet;
    rowHeader: Snippet<[TRow, boolean]>;
    columnHeader: Snippet<[TColumn, boolean]>;
    cell: Snippet<[TRow, TColumn]>;
    columnClass?: (column: TColumn) => string;
    columnAttributes?: (column: TColumn) => Record<string, string>;
    cellAttributes?: (row: TRow, column: TColumn) => Record<string, string>;
    isCellInteractive?: (row: TRow, column: TColumn) => boolean;
    emptyMessage: string;
    stickyHeader?: boolean;
    fillHeight?: boolean;
    stickyHeaderFadeOffset?: string;
    trailingHeader?: Snippet;
    trailingCell?: Snippet<[TRow]>;
    trailingColumns?: number;
    rowHeaderWidth?: string;
    columnHeaderHeight?: string;
    spacerTestId?: string;
  } = $props();

  type Coordinate = { row: string; column: string };
  let hoveredCell = $state<Coordinate | null>(null);
  let focusedCell = $state<Coordinate | null>(null);
  const highlightedCell = $derived(hoveredCell ?? focusedCell);

  function rowHighlighted(row: TRow): boolean {
    return highlightedCell?.row === getRowKey(row);
  }

  function columnHighlighted(column: TColumn): boolean {
    return highlightedCell?.column === getColumnKey(column);
  }

  function cellClass(row: TRow, column: TColumn): string {
    const rowActive = rowHighlighted(row);
    const columnActive = columnHighlighted(column);
    if (rowActive && columnActive) return 'bg-action/15';
    if (rowActive || columnActive) return 'bg-action/8';
    return columnClass?.(column) ?? '';
  }

  function setHovered(row: TRow, column: TColumn) {
    hoveredCell = { row: getRowKey(row), column: getColumnKey(column) };
  }

  function setFocused(row: TRow, column: TColumn, event: FocusEvent) {
    const target = event.target;
    if (!(target instanceof HTMLElement) || !target.matches(':focus-visible')) {
      focusedCell = null;
      return;
    }
    focusedCell = { row: getRowKey(row), column: getColumnKey(column) };
  }
</script>

<DataTable
  items={rows}
  columns={columns.length + trailingColumns + 2}
  getKey={(row) => getRowKey(row)}
  {emptyMessage}
  {stickyHeader}
  {fillHeight}
  {stickyHeaderFadeOffset}
  hoverable={false}
>
  {#snippet header()}
    <th
      class="sticky start-0 z-30 bg-background px-4 py-3 text-start align-bottom font-medium"
      style:width={rowHeaderWidth}
    >
      {@render leadingHeader()}
    </th>
    {#each columns as column (getColumnKey(column))}
      <th
        class={[
          'px-0 py-3 text-center align-bottom font-medium',
          columnHighlighted(column)
            ? 'bg-action/10 text-action'
            : (columnClass?.(column) ?? 'bg-background')
        ]}
        style="width: 2rem; min-width: 2rem"
        style:height={columnHeaderHeight}
        data-matrix-column={getColumnKey(column)}
        {...columnAttributes?.(column) ?? {}}
      >
        <MatrixColumnHeading>
          {@render columnHeader(column, columnHighlighted(column))}
        </MatrixColumnHeading>
      </th>
    {/each}
    {@render trailingHeader?.()}
    <th class="w-full bg-background p-0" aria-hidden="true"></th>
  {/snippet}
  {#snippet row(row)}
    <th
      scope="row"
      class={[
        'sticky start-0 z-10 px-4 py-2 text-start font-normal whitespace-nowrap',
        rowHighlighted(row) ? 'bg-action/8' : 'bg-background'
      ]}
    >
      {@render rowHeader(row, rowHighlighted(row))}
    </th>
    {#each columns as column (getColumnKey(column))}
      {@const interactive = isCellInteractive(row, column)}
      <td
        class={['px-0 py-2 text-center', cellClass(row, column)]}
        style="width: 2.5rem; min-width: 2.5rem"
        data-matrix-column={getColumnKey(column)}
        data-matrix-row={getRowKey(row)}
        {...cellAttributes?.(row, column) ?? {}}
        onmouseenter={interactive ? () => setHovered(row, column) : undefined}
        onmouseleave={interactive ? () => (hoveredCell = null) : undefined}
        onpointerdown={interactive ? () => (focusedCell = null) : undefined}
        onfocusin={interactive ? (event) => setFocused(row, column, event) : undefined}
        onfocusout={interactive ? () => (focusedCell = null) : undefined}
      >
        {@render cell(row, column)}
      </td>
    {/each}
    {@render trailingCell?.(row)}
    <td class="w-full p-0" aria-hidden="true" data-testid={spacerTestId}></td>
  {/snippet}
</DataTable>
