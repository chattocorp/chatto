<!-- @component
Direct attachment management icons. The owner supplies permitted callbacks and
opens confirmation or edit dialogs. Place these actions inside PreviewToolbar.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import PreviewAction from './PreviewAction.svelte';

  let {
    onEdit,
    onDelete,
    editLabel
  }: {
    onEdit?: () => void;
    onDelete?: () => void;
    editLabel: string;
  } = $props();
</script>

{#if onEdit}
  <PreviewAction label={editLabel} icon="icon-[uil--file-edit-alt]" onclick={onEdit} />
{/if}
{#if onDelete}
  <PreviewAction
    label={m('room.attachment.delete_label')}
    icon="icon-[uil--trash-alt]"
    onclick={onDelete}
  />
{/if}
