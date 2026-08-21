<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getChromePermissions } from '$lib/state/server/chromePermissions.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { m } from '$lib/i18n/messages';

  let { children } = $props();

  const getChromePermissionsState = getChromePermissions();
  const chromePermissions = $derived(getChromePermissionsState());
  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  const serverPermissions = $derived(serverScope.store.permissions);

  // Server management routes are gated here. Resource-scoped room routes
  // perform their own checks after loading the target resource.
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const params = { serverId: serverSegment };
    const serverBase = resolve('/chat/[serverId]/manage/server', params);
    const manageBase = serverBase.slice(0, -'/server'.length);
    const generalBase = serverBase + '/general';
    const botsBase = serverBase + '/bots';
    const membersBase = serverBase + '/members';
    const invitationsBase = serverBase + '/invite-links';
    const roomsBase = resolve('/chat/[serverId]/manage/rooms', params);
    const roomGroupsBase = manageBase + '/room-groups';
    const moderationBase = serverBase + '/moderation';
    const permissionsBase = serverBase + '/permissions';
    const securityBase = serverBase + '/security';
    const systemBase = serverBase + '/system';
    const eventLogBase = serverBase + '/event-log';

    // General settings page requires server manage permission
    if (pathname.startsWith(generalBase)) {
      return () => chromePermissions?.canManage ?? false;
    }

    // Bot owners retain management of existing bots after losing bot.create.
    // The bot APIs enforce ownership and bot.manage for each returned resource
    // and mutation, so reaching this page does not grant additional authority.
    if (pathname.startsWith(botsBase)) {
      return () => true;
    }

    // Members pages call AdminUserService.ListMembers/GetMember, which
    // require admin.view-users.
    if (pathname.startsWith(membersBase)) {
      return () => serverPermissions.canAdminViewUsers;
    }

    if (pathname.startsWith(invitationsBase)) {
      return () => serverPermissions.canManageInvites;
    }

    // The room collection is a server-wide layout editor. Individual room
    // pages allow delegated managers and enforce access after loading the room.
    if (pathname === roomsBase || pathname === `${roomsBase}/`) {
      return () => chromePermissions?.canManageRooms ?? false;
    }
    if (pathname.startsWith(`${roomsBase}/`)) return () => true;

    // Resource-scoped room-group pages enforce access after loading the group.
    if (pathname.startsWith(`${roomGroupsBase}/`)) {
      return () => true;
    }

    // Moderation pages: the resolver enforces server-scope room.ban-member.
    if (pathname.startsWith(moderationBase)) {
      return () => chromePermissions?.canViewAdmin ?? false;
    }

    // Permissions pages call the server/group role permission matrix APIs,
    // which require role.manage.
    if (pathname.startsWith(permissionsBase)) {
      return () => chromePermissions?.canManageRoles ?? false;
    }

    // Security (blocked usernames) — server.manage
    if (pathname.startsWith(securityBase)) {
      return () => chromePermissions?.canManage ?? false;
    }

    // System info (NATS/JetStream stats) — owner-only for now.
    if (pathname.startsWith(systemBase)) {
      return () => serverPermissions.canAdminViewSystem;
    }

    // Event log inspection — admin.view-audit
    if (pathname.startsWith(eventLogBase)) {
      return () => serverPermissions.canAdminViewAudit;
    }

    // Default: require server manage for unknown management routes.
    return () => chromePermissions?.canManage ?? false;
  }

  const hasPermission = $derived(getRoutePermissionCheck(page.url.pathname)());

  const permissionsLoaded = $derived(chromePermissions !== null && serverPermissions.loaded);
</script>

{#if !permissionsLoaded}
  <!-- blank shell while permissions load; avoids an Access Denied flash -->
{:else if hasPermission}
  {@render children?.()}
{:else}
  <AccessDenied
    message={m('ui.access_denied.message')}
    backHref={resolve('/chat/[serverId]', {
      serverId: serverSegment
    })}
    backLabel={m('admin.nav.back_to_server')}
  />
{/if}
