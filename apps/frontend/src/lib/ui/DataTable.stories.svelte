<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DataTable from './DataTable.svelte';
  import Panel from './Panel.svelte';
  import CopyId from './CopyId.svelte';
  import EmptyState from './EmptyState.svelte';
  import Pill from '$lib/ui/Pill.svelte';
  import { Button } from '$lib/ui/form';

  type SpaceRow = {
    id: string;
    name: string;
    members: number;
    visibility: 'Public' | 'Invite-only' | 'Private';
  };

  const rows: SpaceRow[] = [
    { id: 'SPC-8DM4Q', name: 'Product', members: 142, visibility: 'Public' },
    { id: 'SPC-2JLA9', name: 'Moderation', members: 12, visibility: 'Private' },
    { id: 'SPC-4MN0X', name: 'Community', members: 87, visibility: 'Invite-only' }
  ];
  const scrollingRows = Array.from({ length: 24 }, (_, index) => ({
    id: `SPC-${String(index + 1).padStart(4, '0')}`,
    name: `Space ${index + 1}`,
    members: (index + 1) * 12,
    visibility: index % 3 === 0 ? 'Public' : index % 3 === 1 ? 'Invite-only' : 'Private'
  })) satisfies SpaceRow[];

  const componentDescription = `
  Record-table primitive with a standalone rounded scroll viewport, contrasting
  header and body, empty state row, optional row hover/click affordance, and
  automatic load-more support. Inside \`Panel noPadding\`, the panel owns the
  shared radius and the table uses flush ruled rows that meet adjacent content
  at square internal seams.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/DataTable',
    component: DataTable,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: componentDescription
        }
      }
    }
  });
</script>

<Story
  name="Records"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The default record table: a rounded inset viewport, strong header/body boundary, hoverable ruled rows, and caller-owned cell layout.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Spaces" noPadding>
      <DataTable
        items={rows}
        columns={4}
        getKey={(row) => row.id}
        header={tableHeader}
        row={tableRow}
      />
    </Panel>
  </div>
</Story>

<Story
  name="Linked rows"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Put a `data-table-row-link` on a real link in the primary cell. The whole row opens the link, the link is one keyboard stop, and other controls in the row stay operable.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Spaces" noPadding>
      <DataTable items={rows} columns={4} getKey={(row) => row.id} header={tableHeader}>
        {#snippet row(row: SpaceRow)}
          <td class="px-4 py-3 font-medium">
            <a class="data-table-row-link" href={`#${row.id}`}>{row.name}</a>
          </td>
          <td class="px-4 py-3"><CopyId value={row.id} /></td>
          <td class="px-4 py-3 text-end tabular-nums">{row.members}</td>
          <td class="px-4 py-3"><Pill>{row.visibility}</Pill></td>
        {/snippet}
      </DataTable>
    </Panel>
  </div>
</Story>

<Story
  name="Following controls"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'When controls or notices precede an edge-to-edge table, the panel keeps one outer radius and the internal boundary remains square.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Members" noPadding>
      <div class="flex items-center justify-between gap-3 border-b border-border p-5">
        <span class="text-sm text-muted">Add people who should have access to this space.</span>
        <Button size="sm" variant="secondary">Add member</Button>
      </div>
      <DataTable
        items={rows}
        columns={4}
        getKey={(row) => row.id}
        header={tableHeader}
        row={tableRow}
      />
    </Panel>
  </div>
</Story>

<Story
  name="Sticky header"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Dense matrices and long administrative tables can retain their column labels in a bounded scrolling viewport.'
      }
    }
  }}
>
  <div class="flex h-96 max-w-3xl flex-col">
    <Panel title="Space permissions" noPadding fillHeight>
      <DataTable
        items={scrollingRows}
        columns={4}
        getKey={(row) => row.id}
        stickyHeader
        fillHeight
        header={tableHeader}
        row={tableRow}
      />
    </Panel>
  </div>
</Story>

<Story
  name="Empty"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use `emptyMessage` for a quiet empty list, or the `empty` snippet when the next useful action belongs in the table.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Spaces" noPadding>
      <DataTable items={[]} columns={4} header={tableHeader} row={tableRow}>
        {#snippet empty()}
          <div class="flex min-h-52 flex-col">
            <EmptyState icon="icon-[uil--building]" title="No spaces yet">
              Create a space to organise conversations for a group of members.
              <div class="mt-4">
                <Button size="sm">Create space</Button>
              </div>
            </EmptyState>
          </div>
        {/snippet}
      </DataTable>
    </Panel>
  </div>
</Story>

<Story
  name="Loading"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'While the first page is pending, `loading` replaces the empty row with one sized pending block inside the table.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Spaces" noPadding>
      <DataTable items={[]} columns={4} loading header={tableHeader} row={tableRow} />
    </Panel>
  </div>
</Story>

<Story
  name="Loading more"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Set `hasMore` and `onLoadMore` for automatic pagination. `loadingMore` shows a pending block after the last row while the next page loads.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Spaces" noPadding>
      <DataTable
        items={rows}
        columns={4}
        getKey={(row) => row.id}
        hasMore
        loadingMore
        onLoadMore={() => {}}
        header={tableHeader}
        row={tableRow}
      />
    </Panel>
  </div>
</Story>

<Story
  name="Grouped rows"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Pass `getGroupKey` and a `group` snippet to insert a full-width heading row before each run of rows that share a key.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Spaces" noPadding>
      <DataTable
        items={[...rows].sort((a, b) => a.visibility.localeCompare(b.visibility))}
        columns={4}
        getKey={(row) => row.id}
        getGroupKey={(row) => row.visibility}
        header={tableHeader}
        row={tableRow}
      >
        {#snippet group(row: SpaceRow)}
          <h3 class="text-sm font-medium text-muted">{row.visibility}</h3>
        {/snippet}
      </DataTable>
    </Panel>
  </div>
</Story>

{#snippet tableHeader()}
  <th class="table-header-cell">Name</th>
  <th class="table-header-cell">ID</th>
  <th class="table-header-cell text-end">Members</th>
  <th class="table-header-cell">Visibility</th>
{/snippet}

{#snippet tableRow(row: SpaceRow)}
  <td class="px-4 py-3 font-medium">{row.name}</td>
  <td class="px-4 py-3"><CopyId value={row.id} /></td>
  <td class="px-4 py-3 text-end tabular-nums">{row.members}</td>
  <td class="px-4 py-3">
    <Pill
      tone={row.visibility === 'Public'
        ? 'success'
        : row.visibility === 'Private'
          ? 'muted'
          : 'neutral'}
    >
      {row.visibility}
    </Pill>
  </td>
{/snippet}
