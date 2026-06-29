<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { createAdminRoomLayoutAPI, type AdminRoomGroup } from '@chatto/api-client/adminRoomLayout';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import * as m from '$lib/i18n/messages';

  const groupId = $derived(page.params.groupId!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const connection = useConnection();
  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })
  );

  let group = $state<AdminRoomGroup | null>(null);
  let loadId = 0;

  async function loadGroupName(targetGroupId: string) {
    const thisId = ++loadId;
    group = null;
    try {
      const conn = connection();
      const api = createAdminRoomLayoutAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const groups = await api.listAdminRoomLayout();
      if (thisId !== loadId) return;
      group = groups.find((candidate) => candidate.id === targetGroupId) ?? null;
    } catch {
      if (thisId === loadId) group = null;
    }
  }

  $effect(() => {
    loadGroupName(groupId);
  });

  const pageTitle = $derived(
    group
      ? m['admin.rooms_admin.permissions_page_title']({ name: group.name })
      : m['admin.rooms_admin.group_permissions_title_fallback']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={group?.name ?? ''}
    subtitle={m['admin.rooms_admin.group_permissions_subtitle']()}
    {backHref}
    backLabel={m['admin.rooms_admin.back_to_rooms']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Hint>{m['admin.rooms_admin.group_permissions_hint']()}</Hint>
    <PermissionMatrix {groupId} />
  </div>
</div>
