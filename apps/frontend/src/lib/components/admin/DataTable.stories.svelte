<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DataTable from './DataTable.svelte';
  import Panel from './Panel.svelte';
  import CopyId from './CopyId.svelte';
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
  Admin table primitive with a standalone rounded scroll viewport, contrasting
  header and body, empty state row, optional row hover/click affordance, and
  automatic load-more support. Inside \`Panel noPadding\`, the panel owns the
  shared radius and the table meets adjacent content at square internal seams.
  `.trim();

  const { Story } = defineMeta({
    title: 'Admin/DataTable',
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
        story: 'Use `emptyMessage` to keep empty admin lists quiet and direct.'
      }
    }
  }}
>
  <div class="max-w-3xl">
    <Panel title="Spaces" noPadding>
      <DataTable
        items={[]}
        columns={4}
        emptyMessage="No spaces found"
        header={tableHeader}
        row={tableRow}
      />
    </Panel>
  </div>
</Story>

{#snippet tableHeader()}
  <th class="table-header-cell">Name</th>
  <th class="table-header-cell">ID</th>
  <th class="table-header-cell text-right">Members</th>
  <th class="table-header-cell">Visibility</th>
{/snippet}

{#snippet tableRow(row: SpaceRow)}
  <td class="px-4 py-3 font-medium">{row.name}</td>
  <td class="px-4 py-3"><CopyId value={row.id} /></td>
  <td class="px-4 py-3 text-right tabular-nums">{row.members}</td>
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
