<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DataTable from './DataTable.svelte';
  import Panel from './Panel.svelte';
  import CopyId from './CopyId.svelte';
  import Pill from '$lib/ui/Pill.svelte';

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

  const componentDescription = `
  Admin table primitive with the shared panel-header treatment, empty state row,
  optional row hover/click affordance, and automatic load-more support. Usually
  placed inside \`Panel noPadding\` so the table owns the panel edges.
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
          'The default record table: sticky visual header treatment, hoverable rows, and caller-owned cell layout.'
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
  <th class="px-4 py-3 font-medium">Name</th>
  <th class="px-4 py-3 font-medium">ID</th>
  <th class="px-4 py-3 text-right font-medium">Members</th>
  <th class="px-4 py-3 font-medium">Visibility</th>
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
          : 'primary'}
    >
      {row.visibility}
    </Pill>
  </td>
{/snippet}
