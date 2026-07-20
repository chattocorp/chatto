<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getChromePermissions } from '$lib/state/server/chromePermissions.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';

  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import * as m from '$lib/i18n/messages';

  let { children } = $props();

  const chromePermissions = getChromePermissions();
  const serverPerms = getServerPermissions();

  // Server management routes are gated here. Resource-scoped room routes
  // perform their own checks after loading the target resource.
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const seg = serverIdToSegment(getActiveServer());
    const params = { serverId: seg };
    const serverBase = resolve('/chat/[serverId]/manage/server', params);
    const manageBase = serverBase.slice(0, -'/server'.length);
    const generalBase = serverBase + '/general';
    const membersBase = serverBase + '/members';
    const roomsBase = resolve('/chat/[serverId]/manage/rooms', params);
    const roomGroupsBase = manageBase + '/room-groups';
    const moderationBase = serverBase + '/moderation';
    const permissionsBase = serverBase + '/permissions';
    const securityBase = serverBase + '/security';
    const systemBase = serverBase + '/system';
    const eventLogBase = serverBase + '/event-log';

    // General settings page requires server manage permission
    if (pathname.startsWith(generalBase)) {
      return () => chromePermissions.current.canManage;
    }

    // Members pages call AdminUserService.ListMembers/GetMember, which
    // require admin.view-users.
    if (pathname.startsWith(membersBase)) {
      return () => serverPerms.current.canAdminViewUsers;
    }

    // The room collection is a server-wide layout editor. Individual room
    // pages allow delegated managers and enforce access after loading the room.
    if (pathname === roomsBase || pathname === `${roomsBase}/`) {
      return () => chromePermissions.current.canManageRooms;
    }
    if (pathname.startsWith(`${roomsBase}/`)) return () => true;

    // Resource-scoped room-group pages enforce access after loading the group.
    if (pathname.startsWith(`${roomGroupsBase}/`)) {
      return () => true;
    }

    // Moderation pages: the resolver enforces server-scope room.ban-member.
    if (pathname.startsWith(moderationBase)) {
      return () => chromePermissions.current.canViewAdmin;
    }

    // Permissions pages call the server/group role permission matrix APIs,
    // which require role.manage.
    if (pathname.startsWith(permissionsBase)) {
      return () => chromePermissions.current.canManageRoles;
    }

    // Security (blocked usernames) — server.manage
    if (pathname.startsWith(securityBase)) {
      return () => chromePermissions.current.canManage;
    }

    // System info (NATS/JetStream stats) — owner-only for now.
    if (pathname.startsWith(systemBase)) {
      return () => serverPerms.current.canAdminViewSystem;
    }

    // Event log inspection — admin.view-audit
    if (pathname.startsWith(eventLogBase)) {
      return () => serverPerms.current.canAdminViewAudit;
    }

    // Default: require server manage for unknown management routes.
    return () => chromePermissions.current.canManage;
  }

  const hasPermission = $derived(getRoutePermissionCheck(page.url.pathname)());

  const permissionsLoaded = $derived(
    chromePermissions.current.loaded && serverPerms.current.loaded
  );
</script>

{#if !permissionsLoaded}
  <!-- blank shell while permissions load; avoids an Access Denied flash -->
{:else if hasPermission}
  {@render children?.()}
{:else}
  <AccessDenied
    message={m['ui.access_denied.message']()}
    backHref={resolve('/chat/[serverId]', {
      serverId: serverIdToSegment(getActiveServer())
    })}
    backLabel={m['admin.nav.back_to_server']()}
  />
{/if}
