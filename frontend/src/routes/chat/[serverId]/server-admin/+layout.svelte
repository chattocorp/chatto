<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getChromePermissions } from '$lib/state/space';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';

  import AccessDenied from '$lib/ui/AccessDenied.svelte';

  let { children } = $props();

  const spacePermissions = getChromePermissions();
  const serverPerms = getServerPermissions();

  // Check if user can access ANY admin section — space-side (server roles,
  // rooms, members) OR instance-side (runtime config, system info).
  const canAccessAnyAdmin = $derived(
    spacePermissions.current.hasAnyAdminPermission || serverPerms.current.canViewAdmin
  );

  // Map routes to required permissions
  // Returns the permission check function for each route prefix
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const seg = serverIdToSegment(getActiveServer());
    const params = { serverId: seg };
    const adminBase = resolve('/chat/[serverId]/server-admin', params);
    const generalBase = resolve('/chat/[serverId]/server-admin/general', params);
    const membersBase = resolve('/chat/[serverId]/server-admin/members', params);
    const roomsBase = adminBase + '/rooms';
    const rolesBase = adminBase + '/roles';
    const securityBase = adminBase + '/security';
    const systemBase = adminBase + '/system';
    const eventLogBase = adminBase + '/event-log';

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
        serverPerms.current.canAdminViewUsers;
    }

    // Rooms pages require room.manage permission
    if (pathname.startsWith(roomsBase)) {
      return () => spacePermissions.current.canManageRooms;
    }

    // Roles pages: space.roles.manage OR instance.admin.view-roles
    if (pathname.startsWith(rolesBase)) {
      return () =>
        spacePermissions.current.canManageRoles ||
        serverPerms.current.canAdminViewRoles;
    }

    // Security (blocked usernames) — instance-admin scope
    if (pathname.startsWith(securityBase)) {
      return () => serverPerms.current.canViewAdmin;
    }

    // System info (NATS/JetStream stats) — admin.view-system
    if (pathname.startsWith(systemBase)) {
      return () => serverPerms.current.canAdminViewSystem;
    }

    // Event log inspection — admin.view-audit
    if (pathname.startsWith(eventLogBase)) {
      return () => serverPerms.current.canAdminViewAudit;
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
    spacePermissions.current.loaded && serverPerms.current.loaded
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
      serverId: serverIdToSegment(getActiveServer())
    })}
    backLabel="Return to Server"
  />
{/if}
