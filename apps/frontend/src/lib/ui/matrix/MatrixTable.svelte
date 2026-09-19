<!--
@component

Domain-neutral matrix layout. The component owns axis geometry, optional row
grouping and compact spacing, horizontal overflow, sticky row headings, and
coordinated row/column highlighting. Below 640px of container width, row labels
wrap above their cells and remain visible while columns scroll. Consumers
provide all domain labels and cell content with snippets.
-->
<script lang="ts" generics="TRow, TColumn">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { m } from '$lib/i18n/messages';
  import DataTable from '$lib/ui/DataTable.svelte';
  import MatrixColumnHeading from './MatrixColumnHeading.svelte';

  const matrixId = $props.id();
  let availableWidth = $state(0);
  // Use the containing pane's width, including narrow desktop panes.
  const stacked = $derived(availableWidth > 0 && availableWidth < 640);
  const rowId = (row: TRow) => `${matrixId}-row-${encodeURIComponent(getRowKey(row))}`;
  const columnId = (column: TColumn) => `${matrixId}-column-${encodeURIComponent(getColumnKey(column))}`;

  // Defer resize-driven layout changes to avoid feeding a new row height back
  // into the same ResizeObserver delivery cycle.
  const measureWidth: Attachment<HTMLDivElement> = (element) => {
    let frame = 0;
    availableWidth = element.clientWidth;
    const observer = new ResizeObserver(() => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => { availableWidth = element.clientWidth; });
    });
    observer.observe(element);
    return () => { observer.disconnect(); cancelAnimationFrame(frame); };
  };

  let {
    rows,
    columns,
    getRowKey,
    getColumnKey,
    getGroupKey,
    group: groupContent,
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
    spacerTestId = 'matrix-spacer',
    compact = false,
    hasMore = false,
    loadingMore = false,
    onLoadMore
  }: {
    /** Incrementally load columns when their horizontal edge is visible. */
    hasMore?: boolean;
    loadingMore?: boolean;
    onLoadMore?: () => unknown;
    rows: TRow[];
    columns: TColumn[];
    getRowKey: (row: TRow) => string;
    getColumnKey: (column: TColumn) => string;
    getGroupKey?: (row: TRow) => string | null | undefined;
    group?: Snippet<[TRow]>;
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
    /** Reduce padding around the standard 40-pixel cell controls. */
    compact?: boolean;
  } = $props();

  const columnSentinel: Attachment<HTMLTableCellElement> = (element) => {
    if (!hasMore || loadingMore || !onLoadMore) return;
    const callback = onLoadMore;
    const root = element.closest('.data-table-viewport');
    if (!root) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          observer.disconnect();
          callback();
        }
      },
      { root, rootMargin: '0px 160px 0px 160px' }
    );
    observer.observe(element);
    return () => observer.disconnect();
  };

  type Coordinate = { row: string; column: string };
  let hoveredCell = $state<Coordinate | null>(null);
  let focusedCell = $state<Coordinate | null>(null);
  let hoveredElement: HTMLElement | null = null;
  let focusedElement: HTMLElement | null = null;
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

  function setHovered(row: TRow, column: TColumn, event: MouseEvent) {
    hoveredElement = event.currentTarget as HTMLElement;
    hoveredCell = { row: getRowKey(row), column: getColumnKey(column) };
  }

  function pointerInsideHover(event: MouseEvent): boolean {
    if (!hoveredElement?.isConnected) return false;
    const bounds = hoveredElement.getBoundingClientRect();
    return event.clientX >= bounds.left && event.clientX < bounds.right &&
      event.clientY >= bounds.top && event.clientY < bounds.bottom;
  }

  function clearHover() {
    hoveredCell = null;
    hoveredElement = null;
  }

  function leaveCell(event: MouseEvent) {
    // Inert temporarily retargets pointer events to an ancestor. The pointer
    // has not left the cell, so keep its row and column highlight stable.
    if (hoveredElement?.closest('[inert]') && pointerInsideHover(event)) return;
    clearHover();
  }

  function movePointer(event: PointerEvent) {
    // Pointer movement still reaches window while the matrix is inert.
    if (hoveredElement?.closest('[inert]') && !pointerInsideHover(event)) clearHover();
  }

  function clearFocus() {
    focusedCell = null;
    focusedElement = null;
  }

  function moveFocus(event: FocusEvent) {
    if (focusedElement && event.target instanceof Node && !focusedElement.contains(event.target)) {
      clearFocus();
    }
  }

  function setFocused(row: TRow, column: TColumn, event: FocusEvent) {
    const target = event.target;
    if (!(target instanceof HTMLElement) || !target.matches(':focus-visible')) {
      clearFocus();
      return;
    }
    focusedElement = event.currentTarget as HTMLElement;
    focusedCell = { row: getRowKey(row), column: getColumnKey(column) };
  }
</script>

<svelte:window onpointermove={movePointer} onfocusin={moveFocus} onblur={() => { clearHover(); clearFocus(); }} />

<div class={['flex w-full min-h-0 min-w-0 flex-col', fillHeight && 'flex-1']} {@attach measureWidth}>
{#if stacked}<span class="sr-only">{@render leadingHeader()}</span>{/if}
<DataTable
  items={rows}
  columns={columns.length + trailingColumns + (stacked ? 1 : 2)}
  getKey={(row) => getRowKey(row)}
  getGroupKey={groupContent ? getGroupKey : undefined}
  {emptyMessage}
  {stickyHeader}
  {fillHeight}
  {stickyHeaderFadeOffset}
  hoverable={false}
>
  {#snippet group(row)}
    <div class={stacked ? 'sticky start-4 whitespace-normal break-words' : ''}
      style:width={stacked ? `${Math.max(0, availableWidth - 32)}px` : undefined}>
      {@render groupContent?.(row)}
    </div>
  {/snippet}
  {#snippet beforeRow(row)}
    {#if stacked}
      <tr>
        <th id={rowId(row)} colspan={columns.length + trailingColumns + 1}
          class="p-0 text-start font-normal bg-background">
          <div class="sticky start-0 box-border px-3 pt-3 pb-1 whitespace-normal break-words [&_*]:whitespace-normal [&_*]:break-words"
            style:width={`${availableWidth}px`}>
            {@render rowHeader(row, rowHighlighted(row))}
          </div>
        </th>
      </tr>
    {/if}
  {/snippet}
  {#snippet header()}
    {#if !stacked}
    <th
      class={[
        'sticky start-0 z-30 bg-background text-start align-bottom font-medium',
        compact ? 'px-3 py-2' : 'px-4 py-3'
      ]}
      style:width={rowHeaderWidth}
    >
      {@render leadingHeader()}
    </th>
    {/if}
    {#each columns as column (getColumnKey(column))}
      <th
        id={columnId(column)}
        scope="col"
        class={[
          compact
            ? 'px-0 py-2 text-center align-bottom font-medium'
            : 'px-0 py-3 text-center align-bottom font-medium',
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
    <th class="w-full bg-background p-0" aria-hidden={!loadingMore} {@attach columnSentinel}>
      {#if loadingMore}
        <span
          role="status"
          aria-label={m('ui.data_table.loading_more')}
          class="iconify icon-[uil--spinner-alt] animate-spin text-muted"
        ></span>
      {/if}
    </th>
  {/snippet}
  {#snippet row(row)}
    {#if !stacked}
    <th
      id={rowId(row)}
      scope="row"
      class={[
        'sticky start-0 z-10 text-start font-normal whitespace-nowrap',
        compact ? 'px-3 py-0.5' : 'px-4 py-2',
        rowHighlighted(row) ? 'bg-action/8' : 'bg-background'
      ]}
    >
      {@render rowHeader(row, rowHighlighted(row))}
    </th>
    {/if}
    {#each columns as column (getColumnKey(column))}
      {@const interactive = isCellInteractive(row, column)}
      <td
        headers={`${rowId(row)} ${columnId(column)}`}
        class={[
          compact ? 'px-0 py-0.5 text-center' : 'px-0 py-2 text-center',
          cellClass(row, column)
        ]}
        style="width: 2.5rem; min-width: 2.5rem"
        data-matrix-column={getColumnKey(column)}
        data-matrix-row={getRowKey(row)}
        {...cellAttributes?.(row, column) ?? {}}
        onmouseenter={interactive ? (event) => setHovered(row, column, event) : undefined}
        onmouseleave={interactive ? leaveCell : undefined}
        onpointerdown={interactive ? clearFocus : undefined}
        onfocusin={interactive ? (event) => setFocused(row, column, event) : undefined}
        onfocusout={interactive ? () => {
          if (!focusedElement?.closest('[inert]')) clearFocus();
        } : undefined}
      >
        {@render cell(row, column)}
      </td>
    {/each}
    {@render trailingCell?.(row)}
    <td class="w-full p-0" aria-hidden="true" data-testid={spacerTestId}></td>
  {/snippet}
</DataTable>
</div>
