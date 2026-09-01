<script lang="ts">
  import type { DeleteMessageContentModalState } from '$lib/modal';
  import { createMessageAPI } from '$lib/api-client/messages';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { notifyRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import { ConfirmDialog } from '$lib/ui';

  let {
    modal,
    onclose
  }: {
    modal: DeleteMessageContentModalState;
    onclose: () => void;
  } = $props();

  let deleting = $state(false);

  function title(): string {
    switch (modal.type) {
      case 'deleteMessage':
        return m('room.message.delete_title');
      case 'deleteAttachment':
        return m('room.attachment.delete_title');
      case 'deleteLinkPreview':
        return m('room.link_preview.delete_title');
    }
  }

  function prompt(): string {
    switch (modal.type) {
      case 'deleteMessage':
        return m('room.message.delete_prompt');
      case 'deleteAttachment':
        return m('room.attachment.delete_prompt');
      case 'deleteLinkPreview':
        return m('room.link_preview.delete_prompt');
    }
  }

  function showDeleteError(error: unknown): void {
    switch (modal.type) {
      case 'deleteMessage':
        toast.error(m('room.message.delete_failed'));
        console.error('Error deleting message:', error);
        return;
      case 'deleteAttachment':
        toast.error(m('room.attachment.delete_failed'));
        console.error('Error deleting attachment:', error);
        return;
      case 'deleteLinkPreview':
        toast.error(m('room.link_preview.delete_failed'));
        console.error('Error deleting link preview:', error);
    }
  }

  async function deleteContent() {
    deleting = true;
    try {
      const api = serverConnectionManager.getClient(modal.serverId).getAPI(createMessageAPI);
      switch (modal.type) {
        case 'deleteMessage':
          await api.deleteMessage(modal.roomId, modal.eventId);
          break;
        case 'deleteAttachment':
          await api.deleteAttachment(modal.roomId, modal.eventId, modal.attachmentId);
          break;
        case 'deleteLinkPreview':
          await api.deleteLinkPreview(modal.roomId, modal.eventId, modal.previewUrl);
          break;
      }
    } catch (error) {
      showDeleteError(error);
      onclose();
      return;
    } finally {
      deleting = false;
    }

    switch (modal.type) {
      case 'deleteMessage':
        notifyRoomMessageMutated({
          serverId: modal.serverId,
          roomId: modal.roomId,
          eventId: modal.eventId,
          reason: 'message-deleted'
        });
        toast.success(m('room.message.deleted'));
        break;
      case 'deleteAttachment':
        notifyRoomMessageMutated({
          serverId: modal.serverId,
          roomId: modal.roomId,
          eventId: modal.eventId,
          reason: 'attachment-deleted'
        });
        break;
      case 'deleteLinkPreview':
        notifyRoomMessageMutated({
          serverId: modal.serverId,
          roomId: modal.roomId,
          eventId: modal.eventId,
          reason: 'link-preview-deleted'
        });
        break;
    }
    onclose();
  }
</script>

<ConfirmDialog
  title={title()}
  actionLabel={m('common.delete')}
  actionIcon="iconify icon-[uil--trash-alt]"
  loading={deleting}
  onconfirm={deleteContent}
  {onclose}
>
  {prompt()}
</ConfirmDialog>
