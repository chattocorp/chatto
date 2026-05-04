<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getSpacePermissions } from '$lib/state/space';

  const getInstanceId = getActiveInstance();
  import AccessDenied from '$lib/ui/AccessDenied.svelte';

  let { children, data } = $props();

  const spacePermissions = getSpacePermissions();

  // Check if user can access ANY settings section (single source of truth from space layout)
  const canAccessAnySettings = $derived(spacePermissions.current.hasAnyAdminPermission);

  // Map routes to required permissions
  // Returns the permission check function for each route prefix
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const spaceId = data.spaceId!;
    const seg = instanceIdToSegment(getInstanceId());
    const params = { instanceId: seg, spaceId };
    const adminBase = resolve('/chat/[instanceId]/[spaceId]/admin', params);
    const generalBase = resolve('/chat/[instanceId]/[spaceId]/admin/general', params);
    const membersBase = resolve('/chat/[instanceId]/[spaceId]/admin/members', params);
    const roomsBase = resolve('/chat/[instanceId]/[spaceId]/admin', params) + '/rooms';
    const rolesBase = resolve('/chat/[instanceId]/[spaceId]/admin', params) + '/roles';
    const inspectorBase = resolve('/chat/[instanceId]/[spaceId]/admin', params) + '/inspector';
    const invitesBase = resolve('/chat/[instanceId]/[spaceId]/admin', params) + '/invites';

    // General settings page requires space.manage permission
    if (pathname.startsWith(generalBase)) {
      return () => spacePermissions.current.canManage;
    }

    // Members pages require roles.assign permission
    if (pathname.startsWith(membersBase)) {
      return () => spacePermissions.current.canAssignRoles;
    }

    // Rooms pages require room.manage permission
    if (pathname.startsWith(roomsBase)) {
      return () => spacePermissions.current.canManageRooms;
    }

    // Roles pages require roles.manage permission
    if (pathname.startsWith(rolesBase)) {
      return () => spacePermissions.current.canManageRoles;
    }

    // Permission inspector also gated on roles.manage — same audience as roles
    if (pathname.startsWith(inspectorBase)) {
      return () => spacePermissions.current.canManageRoles;
    }

    // Invites page requires members.invite permission
    if (pathname.startsWith(invitesBase)) {
      return () => spacePermissions.current.canInviteMembers;
    }

    // Admin home page is accessible to anyone with ANY admin permission
    if (pathname === adminBase) {
      return () => canAccessAnySettings;
    }

    // Default: require space.manage for any other admin route
    return () => spacePermissions.current.canManage;
  }

  const hasPermission = $derived(getRoutePermissionCheck(page.url.pathname)());
</script>

{#if hasPermission}
  {@render children?.()}
{:else}
  <AccessDenied
    message="You do not have permission to access this page."
    backHref={resolve('/chat/[instanceId]/[spaceId]', {
      instanceId: instanceIdToSegment(getInstanceId()),
      spaceId: data.spaceId!
    })}
    backLabel="Return to Space"
  />
{/if}
