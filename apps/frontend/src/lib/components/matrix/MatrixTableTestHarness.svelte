<script lang="ts">
  import MatrixTable from './MatrixTable.svelte';

  type Row = { id: string; label: string };
  type Column = { id: string; label: string; kind: 'server' | 'group' | 'room' };

  let {
    rows = [
      { id: 'mentions', label: 'Mentions' },
      { id: 'replies', label: 'Replies' }
    ],
    columns = [
      { id: 'server', label: 'Example server', kind: 'server' },
      { id: 'general', label: 'General', kind: 'group' }
    ],
    interactive = true
  }: {
    rows?: Row[];
    columns?: Column[];
    interactive?: boolean;
  } = $props();

  function columnClass(column: Column): string {
    if (column.kind === 'server') return 'bg-surface-emphasized/40';
    if (column.kind === 'group') return 'bg-surface-emphasized/20';
    return '';
  }
</script>

<MatrixTable
  {rows}
  {columns}
  getRowKey={(row) => row.id}
  getColumnKey={(column) => column.id}
  {columnClass}
  columnAttributes={(column) => ({ 'data-test-column': column.id })}
  cellAttributes={(row, column) => ({
    'data-test-cell': `${row.id}:${column.id}`
  })}
  isCellInteractive={() => interactive}
  emptyMessage="No matrix rows"
>
  {#snippet leadingHeader()}
    Activity
  {/snippet}
  {#snippet rowHeader(row, highlighted)}
    <span data-test-row-heading={row.id} data-highlighted={highlighted}>{row.label}</span>
  {/snippet}
  {#snippet columnHeader(column, highlighted)}
    <span
      class={highlighted ? 'text-action' : 'text-muted'}
      data-test-column-heading={column.id}
      data-highlighted={highlighted}
    >
      {column.label}
    </span>
  {/snippet}
  {#snippet cell(row, column)}
    <button type="button" aria-label={`${row.label} in ${column.label}`}>Set</button>
  {/snippet}
</MatrixTable>
