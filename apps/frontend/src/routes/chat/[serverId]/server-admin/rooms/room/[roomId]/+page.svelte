<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { createRoomDirectoryAPI, type DirectoryRoomDetails } from '@chatto/api-client/roomDirectory';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import * as m from '$lib/i18n/messages';

  const roomId = $derived(page.params.roomId!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const connection = useConnection();
  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })
  );

  let room = $state<DirectoryRoomDetails | null>(null);
  let loadId = 0;

  async function loadRoomName(targetRoomId: string) {
    const thisId = ++loadId;
    room = null;
    try {
      const conn = connection();
      const api = createRoomDirectoryAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const nextRoom = await api.getRoom(targetRoomId);
      if (thisId !== loadId) return;
      room = nextRoom;
    } catch {
      if (thisId === loadId) room = null;
    }
  }

  $effect(() => {
    loadRoomName(roomId);
  });

  const pageTitle = $derived(
    room
      ? m['admin.rooms_admin.permissions_page_title']({ name: `#${room.name}` })
      : m['admin.rooms_admin.room_permissions_title_fallback']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={room ? `#${room.name}` : ''}
    subtitle={m['admin.rooms_admin.room_permissions_subtitle']()}
    {backHref}
    backLabel={m['admin.rooms_admin.back_to_rooms']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Hint>{m['admin.rooms_admin.room_permissions_hint']()}</Hint>
    <PermissionMatrix {roomId} />
  </div>
</div>
