<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getSpacePermissions } from '$lib/state/space';

  const getInstanceId = getActiveInstance();
  import AccessDenied from '$lib/ui/AccessDenied.svelte';

  let { children } = $props();

  const spacePermissions = getSpacePermissions();

  // Check if user can access ANY settings section (single source of truth from space layout)
  const canAccessAnySettings = $derived(spacePermissions.current.hasAnyAdminPermission);

  // Map routes to required permissions
  // Returns the permission check function for each route prefix
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const seg = instanceIdToSegment(getInstanceId());
    const params = { instanceId: seg };
    const adminBase = resolve('/chat/[instanceId]/(chrome)/server-admin', params);
    const generalBase = resolve('/chat/[instanceId]/(chrome)/server-admin/general', params);
    const membersBase = resolve('/chat/[instanceId]/(chrome)/server-admin/members', params);
    const roomsBase = resolve('/chat/[instanceId]/(chrome)/server-admin', params) + '/rooms';
    const rolesBase = resolve('/chat/[instanceId]/(chrome)/server-admin', params) + '/roles';
    const inspectorBase = resolve('/chat/[instanceId]/(chrome)/server-admin', params) + '/inspector';

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
    backHref={resolve('/chat/[instanceId]', {
      instanceId: instanceIdToSegment(getInstanceId())
    })}
    backLabel="Return to Space"
  />
{/if}
