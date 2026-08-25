<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MatrixTable from './MatrixTable.svelte';

  type Row = { id: string; label: string };
  type Column = { id: string; label: string; kind: 'server' | 'group' | 'room' };

  const rows: Row[] = [
    { id: 'mentions', label: 'Mentions' },
    { id: 'replies', label: 'Replies' },
    { id: 'reactions', label: 'Reactions' }
  ];
  const columns: Column[] = [
    { id: 'server', label: 'Example server', kind: 'server' },
    { id: 'community', label: 'Community', kind: 'group' },
    { id: 'general', label: '#general', kind: 'room' },
    { id: 'support', label: '#support', kind: 'room' },
    { id: 'design', label: '#design', kind: 'room' }
  ];

  const componentDescription = `
  Domain-neutral matrix geometry for settings and administration. Consumers
  supply stable row and column keys, headings, cell snippets, scope shading,
  and interaction rules. The table supplies rotated column headings, sticky
  row headings, native horizontal scrolling, and coordinated hover and focus
  highlighting.
  `.trim();

  const { Story } = defineMeta({
    title: 'Admin/Matrix primitives',
    component: MatrixTable,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  import MatrixCellButton from './MatrixCellButton.svelte';

  function columnClass(column: Column): string {
    if (column.kind === 'server') return 'bg-surface-emphasized/40';
    if (column.kind === 'group') return 'bg-surface-emphasized/20';
    return '';
  }
</script>

<Story
  name="Light"
  asChild
  globals={{ theme: 'light' }}
  parameters={{
    docs: {
      description: {
        story:
          'Move the pointer or keyboard focus between cells to see the active row, column, and intersection.'
      }
    }
  }}
>
  <div class="max-w-3xl rounded-lg bg-background p-4">
    <MatrixTable
      {rows}
      {columns}
      getRowKey={(row) => row.id}
      getColumnKey={(column) => column.id}
      {columnClass}
      emptyMessage="No rows"
      leadingHeader={activityHeader}
      rowHeader={labelRow}
      columnHeader={labelColumn}
      cell={interactiveCell}
    />
  </div>
</Story>

<Story
  name="Dark"
  asChild
  globals={{ theme: 'dark' }}
  parameters={{
    docs: {
      description: {
        story: 'The same scope hierarchy and interaction language in the dark theme.'
      }
    }
  }}
>
  <div class="max-w-3xl rounded-lg bg-background p-4">
    <MatrixTable
      {rows}
      {columns}
      getRowKey={(row) => row.id}
      getColumnKey={(column) => column.id}
      {columnClass}
      emptyMessage="No rows"
      leadingHeader={activityHeader}
      rowHeader={labelRow}
      columnHeader={labelColumn}
      cell={interactiveCell}
    />
  </div>
</Story>

<Story
  name="Loading and disabled"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Cell adapters can keep one operation pending while other cells remain available, or disable the full surface when editing is not allowed.'
      }
    }
  }}
>
  <div class="max-w-3xl rounded-lg bg-background p-4">
    <MatrixTable
      {rows}
      {columns}
      getRowKey={(row) => row.id}
      getColumnKey={(column) => column.id}
      {columnClass}
      emptyMessage="No rows"
      leadingHeader={activityHeader}
      rowHeader={labelRow}
      columnHeader={labelColumn}
      cell={statusCell}
    />
  </div>
</Story>

<Story
  name="Narrow viewport"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The table keeps the row heading at the start edge and uses native horizontal scrolling for the scope columns.'
      }
    }
  }}
>
  <div class="w-72 rounded-lg bg-background p-2">
    <MatrixTable
      {rows}
      {columns}
      getRowKey={(row) => row.id}
      getColumnKey={(column) => column.id}
      {columnClass}
      rowHeaderWidth="9rem"
      emptyMessage="No rows"
      leadingHeader={activityHeader}
      rowHeader={labelRow}
      columnHeader={labelColumn}
      cell={interactiveCell}
    />
  </div>
</Story>

{#snippet activityHeader()}
  Activity
{/snippet}

{#snippet labelRow(row: Row, highlighted: boolean)}
  <span class={highlighted ? 'text-action' : ''}>{row.label}</span>
{/snippet}

{#snippet labelColumn(column: Column, highlighted: boolean)}
  <span class={highlighted ? 'text-action' : 'text-muted'}>{column.label}</span>
{/snippet}

{#snippet interactiveCell(row: Row, column: Column)}
  <MatrixCellButton
    tone={row.id === 'reactions' ? 'warning' : 'action'}
    explicit={column.kind !== 'room'}
    icon={row.id === 'reactions' ? 'icon-[uil--bell]' : 'icon-[uil--volume-up]'}
    ariaLabel={`${row.label} in ${column.label}`}
    onActivate={() => undefined}
  />
{/snippet}

{#snippet statusCell(row: Row, column: Column)}
  <MatrixCellButton
    tone="action"
    explicit
    icon="icon-[uil--volume-up]"
    loading={row.id === 'mentions'}
    disabled={row.id !== 'mentions'}
    ariaLabel={`${row.label} in ${column.label}`}
    onActivate={() => undefined}
  />
{/snippet}
