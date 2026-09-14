<!--
@component

Shows the description of one message attachment without edit controls.
-->
<script lang="ts">
  import type { AttachmentDescriptionModalState } from '$lib/modal';
  import { m } from '$lib/i18n/messages';
  import { Dialog } from '$lib/ui';

  let {
    modal,
    onclose
  }: {
    modal: AttachmentDescriptionModalState;
    onclose: () => void;
  } = $props();
</script>

<Dialog visible size="sm" title={m('room.attachment.description_title')} {onclose}>
  <p class="max-w-prose break-words whitespace-pre-wrap text-text" dir="auto">
    {modal.description}
  </p>
</Dialog>
