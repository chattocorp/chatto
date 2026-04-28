<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  type RoleOverview = {
    roleName: string;
    displayName: string;
    isInstanceRole: boolean;
    isSystem: boolean;
    position: number;
    overrideCount: number;
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
          overrideCount: r.permissions.length + r.permissionDenials.length
        })
      )
      .sort((a, b) => a.position - b.position);
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
      <div class="rounded-lg border border-danger/20 bg-danger/10 p-4 text-danger">{error}</div>
    {/if}

    {#if loading}
      <div class="text-muted">Loading...</div>
    {:else}
      <div class="bg-surface-2 rounded-lg border border-border p-4 text-sm text-muted">
        Room overrides take precedence over space-level role configuration. Roles with no overrides
        inherit their space settings. Use the inspector to see effective permissions for any user.
      </div>

      <Panel title="Roles applicable in this room" icon="iconify uil--shield-check">
        <table class="w-full border-collapse">
          <thead>
            <tr class="border-b border-border bg-surface-200/50">
              <th class="px-4 py-3 text-left text-sm font-medium">Role</th>
              <th class="px-4 py-3 text-center text-sm font-medium">Scope</th>
              <th class="px-4 py-3 text-center text-sm font-medium">Type</th>
              <th class="px-4 py-3 text-center text-sm font-medium">Overrides in this room</th>
              <th class="px-4 py-3 text-center text-sm font-medium"></th>
            </tr>
          </thead>
          <tbody>
            {#each roles as role (role.roleName)}
              <tr
                class="cursor-pointer border-b border-border bg-surface last:border-b-0 hover:bg-surface-200"
                onclick={() => editRole(role)}
              >
                <td class="px-4 py-3">
                  <div class="font-medium">{role.displayName}</div>
                  <code class="text-xs text-muted">{role.roleName}</code>
                </td>
                <td class="px-4 py-3 text-center">
                  {#if role.isInstanceRole}
                    <span class="rounded bg-accent/10 px-2 py-0.5 text-xs font-medium text-accent">
                      Instance
                    </span>
                  {:else}
                    <span class="rounded bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                      Space
                    </span>
                  {/if}
                </td>
                <td class="px-4 py-3 text-center">
                  {#if role.isSystem}
                    <span class="rounded bg-surface-200 px-2 py-0.5 text-xs text-muted">System</span>
                  {:else}
                    <span class="rounded bg-primary/10 px-2 py-0.5 text-xs text-primary">Custom</span>
                  {/if}
                </td>
                <td class="px-4 py-3 text-center">
                  {#if role.overrideCount > 0}
                    <span class="rounded bg-primary/10 px-1.5 py-0.5 text-xs text-primary">
                      {role.overrideCount}
                    </span>
                  {:else}
                    <span class="text-xs text-muted/60">none</span>
                  {/if}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="iconify text-muted uil--angle-right"></span>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="5" class="px-4 py-8 text-center text-muted">No roles found</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </Panel>
    {/if}
  </div>
</div>
