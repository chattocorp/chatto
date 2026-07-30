<script lang="ts">
  import type { DeleteLinkPreviewModalState } from '$lib/modal';
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
    modal: DeleteLinkPreviewModalState;
    onclose: () => void;
  } = $props();

  let deleting = $state(false);

  async function deleteLinkPreview() {
    deleting = true;
    try {
      const api = serverConnectionManager.getClient(getActiveServer()).getAPI(createMessageAPI);
      await api.deleteLinkPreview(modal.roomId, modal.eventId, modal.previewUrl);
    } catch (error) {
      toast.error(m['room.link_preview.delete_failed']());
      console.error('Error deleting link preview:', error);
      onclose();
      return;
    } finally {
      deleting = false;
    }

    notifyRoomMessageMutated({
      roomId: modal.roomId,
      eventId: modal.eventId,
      reason: 'link-preview-deleted'
    });
    onclose();
  }
</script>

<ConfirmDialog
  title={m['room.link_preview.delete_title']()}
  actionLabel={m['common.delete']()}
  actionIcon="iconify uil--trash-alt"
  loading={deleting}
  onconfirm={deleteLinkPreview}
  {onclose}
>
  {m['room.link_preview.delete_prompt']()}
</ConfirmDialog>
