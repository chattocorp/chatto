<!--
@component

Edits the description of one message attachment.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import type { EditAttachmentDescriptionModalState } from '$lib/modal';
  import { createMessageAPI } from '$lib/api-client/messages';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { notifyRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
  import { m } from '$lib/i18n/messages';
  import { FormDialog } from '$lib/ui';
  import { TextArea } from '$lib/ui/form';

  let {
    modal,
    onclose
  }: {
    modal: EditAttachmentDescriptionModalState;
    onclose: () => void;
  } = $props();

  let description = $state(untrack(() => modal.description));
  let loading = $state(false);
  let error = $state<string | null>(null);
  const descriptionLength = $derived(Array.from(description.trim()).length);
  const tooLong = $derived(descriptionLength > 1000);

  async function save() {
    if (loading || tooLong) return;
    loading = true;
    error = null;
    try {
      const api = serverConnectionManager.getClient(modal.serverId).getAPI(createMessageAPI);
      await api.setAttachmentDescription(
        modal.roomId,
        modal.eventId,
        modal.attachmentId,
        description
      );
      notifyRoomMessageMutated({
        serverId: modal.serverId,
        roomId: modal.roomId,
        eventId: modal.eventId,
        reason: 'attachment-description-updated'
      });
      onclose();
    } catch (cause) {
      error =
        cause instanceof Error ? cause.message : m('room.attachment.description_update_failed');
    } finally {
      loading = false;
    }
  }
</script>

<FormDialog
  visible
  title={m('room.attachment.description_title')}
  {loading}
  disabled={tooLong}
  {error}
  onsubmit={save}
  {onclose}
>
  <TextArea
    id="attachment-description"
    label={m('room.attachment.description_label')}
    description={m('room.attachment.description_help')}
    rows={6}
    bind:value={description}
    error={tooLong ? m('room.attachment.description_too_long', { max: 1000 }) : undefined}
  />
</FormDialog>
