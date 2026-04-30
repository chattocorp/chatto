<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Demos/Admin dashboard',
    parameters: {
      layout: 'fullscreen'
    }
  });
</script>

<script lang="ts">
  import StatCard from '$lib/components/admin/StatCard.svelte';
  import Panel from '$lib/components/admin/Panel.svelte';
  import DataTable from '$lib/components/admin/DataTable.svelte';
  import CopyId from '$lib/components/admin/CopyId.svelte';
  import { Button } from '$lib/ui/form';
  import Pill from '$lib/ui/Pill.svelte';

  type SpaceRow = { id: string; name: string; members: number; rooms: number; status: 'public' | 'invite' | 'private' };

  const spaces: SpaceRow[] = [
    { id: 'SAbcDef1', name: 'Open Source Hangout', members: 142, rooms: 12, status: 'public' },
    { id: 'SAbcDef2', name: 'Plant Parents', members: 23, rooms: 4, status: 'public' },
    { id: 'SAbcDef3', name: 'Brutalist Architecture', members: 89, rooms: 7, status: 'invite' },
    { id: 'SAbcDef4', name: 'Cycling Touring', members: 451, rooms: 18, status: 'public' },
    { id: 'SAbcDef5', name: 'Internal Operations', members: 8, rooms: 3, status: 'private' }
  ];
</script>

<Story name="Instance overview" asChild>
  <div class="mx-auto max-w-6xl p-6">
    <header class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">Instance overview</h1>
        <p class="text-sm text-muted">chat.example.com — running v0.0.145</p>
      </div>
      <Button variant="secondary">
        <span class="iconify uil--refresh"></span>
        Refresh
      </Button>
    </header>

    <section class="mb-6 grid grid-cols-2 gap-4 md:grid-cols-4">
      <StatCard color="primary" icon="iconify uil--users-alt" value={713} label="Users" subtitle="+12 this week" />
      <StatCard color="success" icon="iconify uil--building" value={5} label="Spaces" />
      <StatCard color="warning" icon="iconify uil--message" value="48,221" label="Messages" subtitle="all-time" />
      <StatCard color="danger" icon="iconify uil--exclamation-triangle" value={3} label="Failed jobs" subtitle="last 24h" />
    </section>

    <Panel title="Spaces" subtitle="All spaces hosted on this instance" icon="iconify uil--building" count={spaces.length}>
      {#snippet actions()}
        <Button size="sm" variant="secondary">
          <span class="iconify uil--filter"></span>
          Filter
        </Button>
      {/snippet}

      <DataTable
        items={spaces}
        columns={5}
        getKey={(row) => row.id}
        header={tableHeader}
        row={tableRow}
      />
    </Panel>
  </div>
</Story>

{#snippet tableHeader()}
  <th class="px-4 py-2 text-left text-xs font-semibold uppercase text-muted">Name</th>
  <th class="px-4 py-2 text-left text-xs font-semibold uppercase text-muted">ID</th>
  <th class="px-4 py-2 text-right text-xs font-semibold uppercase text-muted">Members</th>
  <th class="px-4 py-2 text-right text-xs font-semibold uppercase text-muted">Rooms</th>
  <th class="px-4 py-2 text-left text-xs font-semibold uppercase text-muted">Visibility</th>
{/snippet}

{#snippet tableRow(row: { id: string; name: string; members: number; rooms: number; status: 'public' | 'invite' | 'private' })}
  <td class="px-4 py-2">{row.name}</td>
  <td class="px-4 py-2"><CopyId value={row.id} /></td>
  <td class="px-4 py-2 text-right tabular-nums">{row.members}</td>
  <td class="px-4 py-2 text-right tabular-nums">{row.rooms}</td>
  <td class="px-4 py-2">
    <Pill tone={row.status === 'public' ? 'success' : row.status === 'invite' ? 'primary' : 'muted'} dimmed>
      {row.status}
    </Pill>
  </td>
{/snippet}
