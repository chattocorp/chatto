<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getSpacePermissions } from '$lib/state/space';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';

  const getInstanceId = getActiveServer();
  import AccessDenied from '$lib/ui/AccessDenied.svelte';

  let { children } = $props();

  const spacePermissions = getSpacePermissions();
  const instancePerms = getServerPermissions();

  // Check if user can access ANY admin section — space-side (server roles,
  // rooms, members) OR instance-side (runtime config, system info).
  const canAccessAnyAdmin = $derived(
    spacePermissions.current.hasAnyAdminPermission || instancePerms.current.canViewAdmin
  );

  // Map routes to required permissions
  // Returns the permission check function for each route prefix
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const seg = serverIdToSegment(getInstanceId());
    const params = { serverId: seg };
    const adminBase = resolve('/chat/[serverId]/(chrome)/server-admin', params);
    const generalBase = resolve('/chat/[serverId]/(chrome)/server-admin/general', params);
    const membersBase = resolve('/chat/[serverId]/(chrome)/server-admin/members', params);
    const roomsBase = adminBase + '/rooms';
    const rolesBase = adminBase + '/roles';
    const inspectorBase = adminBase + '/inspector';
    const securityBase = adminBase + '/security';
    const systemBase = adminBase + '/system';

    // General settings page requires space.manage permission
    if (pathname.startsWith(generalBase)) {
      return () => spacePermissions.current.canManage;
    }

    // Members pages: viewable by anyone with the space-side roles.assign or
    // the instance-side admin.view-users — covers both "server moderator
    // managing members" and "instance admin browsing the user directory."
    if (pathname.startsWith(membersBase)) {
      return () =>
        spacePermissions.current.canAssignRoles ||
        instancePerms.current.canAdminViewUsers;
    }

    // Rooms pages require room.manage permission
    if (pathname.startsWith(roomsBase)) {
      return () => spacePermissions.current.canManageRooms;
    }

    // Roles pages: space.roles.manage OR instance.admin.view-roles
    if (pathname.startsWith(rolesBase)) {
      return () =>
        spacePermissions.current.canManageRoles ||
        instancePerms.current.canAdminViewRoles;
    }

    // Permission inspector — same audience as the roles list
    if (pathname.startsWith(inspectorBase)) {
      return () =>
        spacePermissions.current.canManageRoles ||
        instancePerms.current.canAdminViewRoles;
    }

    // Security (blocked usernames) — instance-admin scope
    if (pathname.startsWith(securityBase)) {
      return () => instancePerms.current.canViewAdmin;
    }

    // System info (NATS/JetStream stats) — admin.view-system
    if (pathname.startsWith(systemBase)) {
      return () => instancePerms.current.canAdminViewSystem;
    }

    // Admin home page is accessible to anyone with ANY admin permission
    if (pathname === adminBase) {
      return () => canAccessAnyAdmin;
    }

    // Default: require space.manage for any other admin route
    return () => spacePermissions.current.canManage;
  }

  const hasPermission = $derived(getRoutePermissionCheck(page.url.pathname)());

  const permissionsLoaded = $derived(
    spacePermissions.current.loaded && instancePerms.current.loaded
  );
</script>

{#if !permissionsLoaded}
  <!-- blank shell while permissions load; avoids an Access Denied flash -->
{:else if hasPermission}
  {@render children?.()}
{:else}
  <AccessDenied
    message="You do not have permission to access this page."
    backHref={resolve('/chat/[serverId]', {
      serverId: serverIdToSegment(getInstanceId())
    })}
    backLabel="Return to Server"
  />
{/if}
