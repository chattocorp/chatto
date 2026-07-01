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

  // Map routes to required permissions
  // Returns the permission check function for each route prefix
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const seg = serverIdToSegment(getActiveServer());
    const params = { serverId: seg };
    const adminBase = resolve('/chat/[serverId]/server-admin', params);
    const generalBase = resolve('/chat/[serverId]/server-admin/general', params);
    const membersBase = resolve('/chat/[serverId]/server-admin/members', params);
    const roomsBase = adminBase + '/rooms';
    const moderationBase = adminBase + '/moderation';
    const permissionsBase = adminBase + '/permissions';
    const securityBase = adminBase + '/security';
    const systemBase = adminBase + '/system';
    const eventLogBase = adminBase + '/event-log';

    // General settings page requires server manage permission
    if (pathname.startsWith(generalBase)) {
      return () => chromePermissions.current.canManage;
    }

    // Members pages call AdminMemberService.ListMembers/GetMember, which
    // require admin.view-users.
    if (pathname.startsWith(membersBase)) {
      return () => serverPerms.current.canAdminViewUsers;
    }

    // Rooms pages require room.manage permission
    if (pathname.startsWith(roomsBase)) {
      return () => chromePermissions.current.canManageRooms;
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

    // Default: require server manage for any other admin route
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
