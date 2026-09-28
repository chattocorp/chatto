<script lang="ts">
  import { PageTitle, AccessDenied } from '$lib/ui';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  import { m } from '$lib/i18n/messages';

  let { children } = $props();

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  const permissions = $derived(serverScope.store.permissions);

  // Server management routes are gated here. Resource-scoped room routes
  // perform their own checks after loading the target resource.
  function getRoutePermissionCheck(pathname: string): () => boolean {
    const params = { serverId: serverSegment };
    const serverBase = resolve('/chat/[serverId]/manage/server', params);
    const manageBase = serverBase.slice(0, -'/server'.length);
    const generalBase = serverBase + '/general';
    const neighborsBase = serverBase + '/neighbors';
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
      return () => permissions.canManageServer;
    }

    if (pathname.startsWith(neighborsBase)) {
      return () => permissions.canManageNeighbors;
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
      return () => permissions.canAdminViewUsers;
    }

    if (pathname.startsWith(invitationsBase)) {
      return () => permissions.canManageInvites;
    }

    // The room collection is a server-wide layout editor. Individual room
    // pages allow delegated managers and enforce access after loading the room.
    if (pathname === roomsBase || pathname === `${roomsBase}/`) {
      return () => permissions.canManageRooms;
    }
    if (pathname.startsWith(`${roomsBase}/`)) return () => true;

    // Resource-scoped room-group pages enforce access after loading the group.
    if (pathname.startsWith(`${roomGroupsBase}/`)) {
      return () => true;
    }

    // The suspension list requires effective server-scope room.remove-member.
    if (pathname.startsWith(moderationBase)) {
      return () => permissions.canModerateRooms;
    }

    // Permissions pages call the server/group role permission matrix APIs,
    // which require role.manage.
    if (pathname.startsWith(permissionsBase)) {
      return () => permissions.canAdminManageRoles;
    }

    // Security (blocked usernames) — server.manage
    if (pathname.startsWith(securityBase)) {
      return () => permissions.canManageServer;
    }

    // System info (NATS/JetStream stats) — owner-only for now.
    if (pathname.startsWith(systemBase)) {
      return () => permissions.canAdminViewSystem;
    }

    // Event log inspection — admin.view-audit
    if (pathname.startsWith(eventLogBase)) {
      return () => permissions.canAdminViewAudit;
    }

    // Default: require server manage for unknown management routes.
    return () => permissions.canManageServer;
  }

  const hasPermission = $derived(getRoutePermissionCheck(page.url.pathname)());
</script>

{#if !permissions.loaded}
  <!-- blank shell while permissions load; avoids an Access Denied flash -->
  <PageTitle />
{:else if hasPermission}
  {@render children?.()}
{:else}
  <PageTitle title={m('ui.access_denied.title')} />
  <AccessDenied
    message={m('ui.access_denied.message')}
    backHref={resolve('/chat/[serverId]', {
      serverId: serverSegment
    })}
    backLabel={m('admin.nav.back_to_server')}
  />
{/if}
