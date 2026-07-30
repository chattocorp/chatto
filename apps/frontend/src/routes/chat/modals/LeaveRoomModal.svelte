<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { LeaveRoomModalState } from '$lib/modal';
  import { serverIdToSegment } from '$lib/navigation';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';

  let {
    modal,
    onclose
  }: {
    modal: LeaveRoomModalState;
    onclose: () => void;
  } = $props();

  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  let leaving = $state(false);

  async function leaveRoom() {
    leaving = true;
    try {
      const api = serverConnectionManager.getClient(activeServerId).getAPI(createRoomCommandAPI);
      await api.leaveRoom(modal.roomId);
    } catch (error) {
      toast.error(m['room.leave.failed']());
      console.error('Error leaving room:', error);
      onclose();
      return;
    } finally {
      leaving = false;
    }

    clearLastRoom(activeServerId);
    goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
  }
</script>

<ConfirmDialog
  title={m['room.leave.title']()}
  actionLabel={m['room.leave.action']()}
  actionIcon="iconify uil--sign-out-alt"
  loading={leaving}
  onconfirm={leaveRoom}
  {onclose}
>
  {m['room.leave.prompt']({ room: modal.roomName })}
</ConfirmDialog>
