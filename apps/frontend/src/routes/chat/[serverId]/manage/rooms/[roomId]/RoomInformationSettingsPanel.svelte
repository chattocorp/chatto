<!-- @component Edits channel information as Markdown source in its own form. -->
<script lang="ts">
  import { page } from '$app/state';
  import { MAX_ROOM_INFORMATION_LENGTH } from '@chatto/client/api/rooms';
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import { m } from '$lib/i18n/messages';
  import { Panel } from '$lib/ui';
  import { TextArea } from '$lib/ui/form';
  import RoomSettingsForm from './RoomSettingsForm.svelte';
  import type { RoomSettingsPatch } from './roomSettings';

  let {
    information,
    onSave
  }: {
    information: string;
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const draft = new DraftField(() => information);
  const changed = $derived(draft.value !== draft.original);
  const tooLong = $derived([...draft.value].length > MAX_ROOM_INFORMATION_LENGTH);
  let form = $state<RoomSettingsForm>();

  /** The route can finish scrolling before the managed room request supplies the editor. */
  function focusLinkedEditor(input: HTMLTextAreaElement): void {
    if (page.url.hash !== `#${input.id}`) return;
    input.focus({ preventScroll: true });
    input.scrollIntoView({ block: 'center' });
  }

  /** Protect both pending saves and unsaved Markdown during navigation. */
  export function hasChanges(): boolean {
    return form?.hasChanges() ?? changed;
  }
</script>

<Panel title={m('room.information.title')} icon="iconify icon-[uil--info-circle]">
  <RoomSettingsForm
    bind:this={form}
    {changed}
    disabled={tooLong}
    onSave={() => onSave({ information: draft.value })}
    onDiscard={() => (draft.value = information)}
  >
    {#snippet children(saving: boolean)}
      <TextArea
        id="room-information"
        attachment={focusLinkedEditor}
        label={m('settings.preferences.editor.markdown.label')}
        rows={12}
        bind:value={draft.value}
        disabled={saving}
        error={tooLong
          ? m('room.information.too_long', { max: MAX_ROOM_INFORMATION_LENGTH })
          : undefined}
      />
    {/snippet}
  </RoomSettingsForm>
</Panel>
