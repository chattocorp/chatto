<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  type RoleOverview = {
    roleName: string;
    displayName: string;
    isInstanceRole: boolean;
    isSystem: boolean;
    position: number;
    permissions: string[];
    permissionDenials: string[];
  };

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const connection = useConnection();
  const spaceId = $derived(page.params.spaceId!);
  const roomId = $derived(page.params.roomId!);

  let roles = $state<RoleOverview[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  $effect(() => {
    if (spaceId && roomId) {
      loadData();
    }
  });

  async function loadData() {
    const currentSpace = spaceId;
    const currentRoom = roomId;

    loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query RoomPermissionRoles($spaceId: ID!, $roomId: ID!) {
          room(spaceId: $spaceId, roomId: $roomId) {
            id
            roomPermissionOverrides {
              roleName
              displayName
              isInstanceRole
              isSystem
              position
              permissions
              permissionDenials
            }
          }
        }
      `),
      { spaceId: currentSpace, roomId: currentRoom }
    );

    if (currentSpace !== spaceId || currentRoom !== roomId) return;

    loading = false;
    if (resp.error) {
      error = resp.error.message;
      return;
    }
    if (!resp.data?.room) {
      error = 'Room not found';
      return;
    }

    roles = resp.data.room.roomPermissionOverrides
      .map(
        (r): RoleOverview => ({
          roleName: r.roleName,
          displayName: r.displayName,
          isInstanceRole: r.isInstanceRole,
          isSystem: r.isSystem,
          position: r.position,
          permissions: r.permissions,
          permissionDenials: r.permissionDenials
        })
      )
      // Group by scope (Space first), then by position within each group.
      // Space and instance roles use independent position numbering, so a flat
      // sort by position alone interleaves them confusingly.
      .sort((a, b) => {
        if (a.isInstanceRole !== b.isInstanceRole) return a.isInstanceRole ? 1 : -1;
        return a.position - b.position;
      });
  }

  function editRole(role: RoleOverview) {
    goto(
      resolve('/chat/[instanceId]/[spaceId]/[roomId]/settings/permissions/[roleName]', {
        instanceId: instanceSegment,
        spaceId,
        roomId,
        roleName: role.roleName
      })
    );
  }
</script>

<PageTitle title="Room Permissions" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Room Permissions"
    subtitle="Pick a role to view or change its room-level overrides"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if error}
      <Hint variant="danger">{error}</Hint>
    {/if}

    {#if loading}
      <div class="text-muted">Loading...</div>
    {:else}
      <Hint>
        Room overrides take precedence over space-level role configuration. Roles with no overrides
        inherit their space settings. Use the inspector to see effective permissions for any user.
      </Hint>

      <Panel title="Roles applicable in this room" icon="iconify uil--shield-check" noPadding>
        <DataTable
          items={roles}
          columns={5}
          getKey={(r) => r.roleName}
          onRowClick={editRole}
          emptyMessage="No roles found"
        >
          {#snippet header()}
            <th class="px-4 py-3 font-medium">Role</th>
            <th class="px-4 py-3 text-center font-medium">Scope</th>
            <th class="px-4 py-3 text-center font-medium">Type</th>
            <th class="px-4 py-3 text-center font-medium">Overrides in this room</th>
            <th class="px-4 py-3"></th>
          {/snippet}
          {#snippet row(role)}
            <td class="px-4 py-3">
              <div class="font-medium">{role.displayName}</div>
              <code class="text-xs text-muted">{role.roleName}</code>
            </td>
            <td class="px-4 py-3 text-center">
              <Pill tone={role.isInstanceRole ? 'accent' : 'primary'}>
                {role.isInstanceRole ? 'Instance' : 'Space'}
              </Pill>
            </td>
            <td class="px-4 py-3 text-center">
              <Pill tone={role.isSystem ? 'muted' : 'primary'}>
                {role.isSystem ? 'System' : 'Custom'}
              </Pill>
            </td>
            <td class="px-4 py-3">
              {#if role.permissions.length === 0 && role.permissionDenials.length === 0}
                <span class="text-xs text-muted/60">none</span>
              {:else}
                <div class="flex flex-wrap gap-1">
                  {#each role.permissions as perm (perm)}
                    <Pill tone="success" title="Allow {perm}">{perm}</Pill>
                  {/each}
                  {#each role.permissionDenials as perm (perm)}
                    <Pill tone="danger" title="Deny {perm}">{perm}</Pill>
                  {/each}
                </div>
              {/if}
            </td>
            <td class="px-4 py-3 text-right">
              <span class="iconify text-muted uil--angle-right"></span>
            </td>
          {/snippet}
        </DataTable>
      </Panel>
    {/if}
  </div>
</div>
