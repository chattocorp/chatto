<script lang="ts">
  import type { DeleteAttachmentModalState } from '$lib/modal';
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
    modal: DeleteAttachmentModalState;
    onclose: () => void;
  } = $props();

  let deleting = $state(false);

  async function deleteAttachment() {
    deleting = true;
    try {
      const api = serverConnectionManager.getClient(getActiveServer()).getAPI(createMessageAPI);
      await api.deleteAttachment(modal.roomId, modal.eventId, modal.attachmentId);
    } catch (error) {
      toast.error(m['room.attachment.delete_failed']());
      console.error('Error deleting attachment:', error);
      onclose();
      return;
    } finally {
      deleting = false;
    }

    notifyRoomMessageMutated({
      roomId: modal.roomId,
      eventId: modal.eventId,
      reason: 'attachment-deleted'
    });
    onclose();
  }
</script>

<ConfirmDialog
  title={m['room.attachment.delete_title']()}
  actionLabel={m['common.delete']()}
  actionIcon="iconify uil--trash-alt"
  loading={deleting}
  onconfirm={deleteAttachment}
  {onclose}
>
  {m['room.attachment.delete_prompt']()}
</ConfirmDialog>
