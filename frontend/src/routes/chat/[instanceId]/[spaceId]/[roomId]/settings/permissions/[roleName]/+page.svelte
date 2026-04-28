<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { PermissionGrid, type PermissionState } from '$lib/components/rbac';

  type RoleOverride = {
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
  const roleName = $derived(page.params.roleName!);

  let role = $state<RoleOverride | null>(null);
  let availablePermissions = $state<string[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updating = $state<string | null>(null);

  const backHref = $derived(
    resolve('/chat/[instanceId]/[spaceId]/[roomId]/settings/permissions', {
      instanceId: instanceSegment,
      spaceId,
      roomId
    })
  );

  $effect(() => {
    if (spaceId && roomId && roleName) {
      loadData();
    }
  });

  async function loadData() {
    const currentSpace = spaceId;
    const currentRoom = roomId;
    const currentRole = roleName;

    loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query RoomRoleOverride($spaceId: ID!, $roomId: ID!) {
          room(spaceId: $spaceId, roomId: $roomId) {
            id
            name
            availableRoomPermissions
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

    // Stale response guard
    if (
      currentSpace !== spaceId ||
      currentRoom !== roomId ||
      currentRole !== roleName
    ) {
      return;
    }

    loading = false;
    if (resp.error) {
      error = resp.error.message;
      return;
    }
    if (!resp.data?.room) {
      error = 'Room not found';
      return;
    }

    availablePermissions = resp.data.room.availableRoomPermissions;
    role = resp.data.room.roomPermissionOverrides.find((r) => r.roleName === currentRole) ?? null;
    if (!role) {
      error = `Role "${currentRole}" is not available in this room`;
    }
  }

  async function setPermissionState(permission: string, newState: PermissionState) {
    if (!role) return;
    updating = permission;
    error = null;

    let mutation;
    switch (newState) {
      case 'allow':
        mutation = graphql(`
          mutation GrantRoomPermissionForRole($input: GrantRoomPermissionInput!) {
            grantRoomPermission(input: $input)
          }
        `);
        break;
      case 'deny':
        mutation = graphql(`
          mutation DenyRoomPermissionForRole($input: DenyRoomPermissionInput!) {
            denyRoomPermission(input: $input)
          }
        `);
        break;
      case 'neutral':
        mutation = graphql(`
          mutation ClearRoomPermissionForRole($input: ClearRoomPermissionInput!) {
            clearRoomPermission(input: $input)
          }
        `);
        break;
    }

    const resp = await connection().client.mutation(mutation, {
      input: { spaceId, roomId, role: roleName, permission }
    });

    if (resp.error) {
      error = resp.error.message;
    } else if (role) {
      // Optimistic update
      role.permissions = role.permissions.filter((p) => p !== permission);
      role.permissionDenials = role.permissionDenials.filter((p) => p !== permission);
      if (newState === 'allow') {
        role.permissions = [...role.permissions, permission];
        toast.success(`Granted ${permission} for ${role.displayName}`);
      } else if (newState === 'deny') {
        role.permissionDenials = [...role.permissionDenials, permission];
        toast.success(`Denied ${permission} for ${role.displayName}`);
      } else {
        toast.success(`Cleared ${permission} for ${role.displayName}`);
      }
    }

    updating = null;
  }
</script>

<PageTitle title={`${roleName} | Room Permissions`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={role?.displayName ?? roleName}
    subtitle="Room-level overrides for this role"
    showMobileNav
  >
    {#snippet actions()}
      <Button variant="secondary" href={backHref}>Back to roles</Button>
    {/snippet}
  </PaneHeader>

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if error}
      <Hint variant="danger">{error}</Hint>
    {/if}

    {#if loading || !role}
      <div class="text-muted">Loading...</div>
    {:else}
      <Hint>
        Set <strong>Allow</strong> or <strong>Deny</strong> to override this role's space-level
        configuration in this room. <strong>Neutral</strong> means the permission inherits from the
        role's space-level setting.
      </Hint>

      <Panel title="Permission Overrides" icon="iconify uil--shield-check">
        <PermissionGrid
          permissions={availablePermissions}
          grantedPermissions={role.permissions}
          deniedPermissions={role.permissionDenials}
          updatingPermission={updating}
          onSetState={setPermissionState}
        />
      </Panel>
    {/if}
  </div>
</div>
