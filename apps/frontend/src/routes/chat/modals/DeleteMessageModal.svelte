<script lang="ts">
  import type { DeleteMessageModalState } from '$lib/modal';
  import { createMessageAPI } from '$lib/api-client/messages';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { notifyRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';

  let {
    modal,
    onclose
  }: {
    modal: DeleteMessageModalState;
    onclose: () => void;
  } = $props();

  let deleting = $state(false);

  async function deleteMessage() {
    deleting = true;
    try {
      const api = serverConnectionManager.getClient(getActiveServer()).getAPI(createMessageAPI);
      await api.deleteMessage(modal.roomId, modal.eventId);
    } catch (error) {
      toast.error(m['room.message.delete_failed']());
      console.error('Error deleting message:', error);
      onclose();
      return;
    } finally {
      deleting = false;
    }

    notifyRoomMessageMutated({
      roomId: modal.roomId,
      eventId: modal.eventId,
      reason: 'message-deleted'
    });
    toast.success(m['room.message.deleted']());
    onclose();
  }
</script>

<ConfirmDialog
  title={m['room.message.delete_title']()}
  actionLabel={m['common.delete']()}
  actionIcon="iconify uil--trash-alt"
  loading={deleting}
  onconfirm={deleteMessage}
  {onclose}
>
  {m['room.message.delete_prompt']()}
</ConfirmDialog>
