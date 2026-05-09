<script lang="ts">
  import { goto } from '$app/navigation';
  import { getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const spaceId = $derived(getActiveInstanceSpaceId()());
  const roomId = $derived(page.params.roomId!);

  // Instance role detail pages require instance admin (admin.manage-roles).
  // A space admin without that permission would land on a permission-denied
  // shell — gate the column-header click for instance roles. Space role
  // detail just needs role.manage on this space, which the viewer must
  // already have to be looking at this room matrix at all.
  const instancePerms = getInstancePermissions();
  const canManageInstanceRoles = $derived(instancePerms.current.canAdminManageRoles);

  // Roles don't live at the room tier — clicking a column header navigates
  // to the role's home (instance role → instance admin; space role → space admin)
  // so the user can edit metadata, see assigned users, and so on.
  function openRoleDetail(role: { roleName: string; isInstanceRole: boolean }) {
    if (role.isInstanceRole) {
      goto(
        resolve('/chat/[instanceId]/admin/roles/[name]', {
          instanceId: instanceSegment,
          name: role.roleName
        })
      );
    } else {
      goto(
        resolve('/chat/[instanceId]/(chrome)/server-admin/roles/[name]', {
          instanceId: instanceSegment,
          name: role.roleName
        })
      );
    }
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
      isRoleClickable={(role) => (role.isInstanceRole ? canManageInstanceRoles : true)}
    />
  </div>
</div>
