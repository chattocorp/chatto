<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Demos/Permission editor'
  });

  type State = 'allow' | 'deny' | 'neutral';

  type Permission = {
    id: string;
    label: string;
    help: string;
    inherited: State;
    state: State;
  };

  type PermissionColumn = {
    id: 'inherited' | 'override';
    label: string;
  };
</script>

<script lang="ts">
  import Pill from '$lib/ui/Pill.svelte';
  import HelpTooltip from '$lib/ui/HelpTooltip.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import MatrixCellButton from '$lib/ui/matrix/MatrixCellButton.svelte';
  import MatrixTable from '$lib/ui/matrix/MatrixTable.svelte';

  const columns: PermissionColumn[] = [
    { id: 'inherited', label: 'Inherited' },
    { id: 'override', label: 'Room override' }
  ];

  let permissions = $state<Permission[]>([
    {
      id: 'message.post',
      label: 'message.post',
      help: 'Send messages to the room.',
      inherited: 'allow',
      state: 'neutral'
    },
    {
      id: 'message.post-in-thread',
      label: 'message.post-in-thread',
      help: 'Reply inside an existing thread, even when the room blocks new top-level posts.',
      inherited: 'allow',
      state: 'allow'
    },
    {
      id: 'message.react',
      label: 'message.react',
      help: 'Add and remove reactions on messages.',
      inherited: 'allow',
      state: 'neutral'
    },
    {
      id: 'message.manage',
      label: 'message.manage',
      help: "Edit and delete other users' messages.",
      inherited: 'deny',
      state: 'neutral'
    },
    {
      id: 'room.create',
      label: 'room.create',
      help: 'Create new rooms.',
      inherited: 'allow',
      state: 'deny'
    }
  ]);

  function cycle(perm: Permission) {
    perm.state = perm.state === 'neutral' ? 'allow' : perm.state === 'allow' ? 'deny' : 'neutral';
  }

  function effective(perm: Permission): 'allow' | 'deny' {
    if (perm.state === 'allow') return 'allow';
    if (perm.state === 'deny') return 'deny';
    return perm.inherited === 'deny' ? 'deny' : 'allow';
  }
</script>

<Story name="Role permissions" asChild>
  <div class="mx-auto max-w-3xl p-6">
    <header class="mb-4 flex items-baseline justify-between">
      <div>
        <h1 class="text-2xl font-bold">Role permissions</h1>
        <p class="text-sm text-muted">Member role · #general</p>
      </div>
      <Pill tone="server">Room override</Pill>
    </header>

    <Hint tone="info">
      Activate an override cell to cycle between inherit, allow, and deny.
      The inherited value still shows the space-level role.
    </Hint>

    <div class="mt-6">
      <MatrixTable
        rows={permissions}
        {columns}
        getRowKey={(permission) => permission.id}
        getColumnKey={(column) => column.id}
        isCellInteractive={(_, column) => column.id === 'override'}
        emptyMessage="No permissions found"
        leadingHeader={permissionHeader}
        rowHeader={permissionRow}
        columnHeader={permissionColumn}
        cell={permissionCell}
        trailingHeader={effectiveHeader}
        trailingCell={effectiveCell}
        trailingColumns={1}
        rowHeaderWidth="18rem"
      />
    </div>
  </div>
</Story>

{#snippet permissionHeader()}
  Permission
{/snippet}

{#snippet permissionRow(permission: Permission, highlighted: boolean)}
  <div class="flex items-center gap-2">
    <code class={['font-mono text-sm', highlighted ? 'text-action' : '']}>{permission.label}</code>
    <HelpTooltip>{permission.help}</HelpTooltip>
  </div>
{/snippet}

{#snippet permissionColumn(column: PermissionColumn, highlighted: boolean)}
  <span class={highlighted ? 'text-action' : 'text-muted'}>{column.label}</span>
{/snippet}

{#snippet permissionCell(permission: Permission, column: PermissionColumn)}
  {#if column.id === 'inherited'}
    <Pill
      tone={permission.inherited === 'allow'
        ? 'success'
        : permission.inherited === 'deny'
          ? 'danger'
          : 'muted'}
      compact
    >
      {permission.inherited === 'neutral' ? 'inherit' : permission.inherited}
    </Pill>
  {:else}
    <MatrixCellButton
      tone={permission.state === 'deny' ? 'danger' : 'success'}
      explicit={permission.state !== 'neutral'}
      inheritedMarker={permission.state === 'neutral'}
      icon={permission.state === 'deny'
        ? 'icon-[uil--times]'
        : permission.state === 'allow'
          ? 'icon-[uil--check]'
          : 'icon-[uil--link]'}
      pressed={permission.state !== 'neutral'}
      ariaLabel={`Override ${permission.label}: ${permission.state}. Activate to cycle the value.`}
      title="Cycle inherit, allow, deny"
      onActivate={() => cycle(permission)}
    />
  {/if}
{/snippet}

{#snippet effectiveHeader()}
  <th class="bg-background px-4 py-3 text-start align-bottom font-medium">Effective</th>
{/snippet}

{#snippet effectiveCell(permission: Permission)}
  <td class="bg-background px-4 py-2">
    <Pill tone={effective(permission) === 'allow' ? 'success' : 'danger'}>
      {effective(permission)}
    </Pill>
  </td>
{/snippet}
