<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  const getInstanceId = getActiveServer();
  const instanceSegment = $derived(serverIdToSegment(getInstanceId()));
  const roomId = $derived(page.params.roomId!);

  // Role detail pages require admin.manage-roles; gate the column-header
  // click for non-admins so they don't land on a permission-denied shell.
  const instancePerms = getServerPermissions();
  const canManageRolesFull = $derived(instancePerms.current.canAdminManageRoles);

  // Roles don't live at the room tier — clicking a column header navigates
  // to the role's home in the unified server-admin so the user can edit
  // metadata, see assigned users, and so on.
  function openRoleDetail(role: { roleName: string }) {
    goto(
      resolve('/chat/[serverId]/(chrome)/server-admin/roles/[name]', {
        serverId: instanceSegment,
        name: role.roleName
      })
    );
  }
</script>

<PageTitle title="Room Permissions" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Room Permissions"
    subtitle="Override permissions per role for this room"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <PermissionMatrix
      {roomId}
      onRoleClick={openRoleDetail}
      isRoleClickable={() => canManageRolesFull}
    />
  </div>
</div>
