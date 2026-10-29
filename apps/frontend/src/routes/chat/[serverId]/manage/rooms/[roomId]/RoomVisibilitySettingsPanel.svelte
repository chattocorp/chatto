<!--
@component

Visibility of a room. The checkbox edits a local draft until the form submits.
-->
<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { Hint, Panel } from '$lib/ui';
  import { Checkbox } from '$lib/ui/form';
  import RoomSettingsForm from './RoomSettingsForm.svelte';
  import { m } from '$lib/i18n/messages';
  import type { RoomSettingsPatch } from './roomSettings';

  let {
    room,
    onSave
  }: {
    room: AdminManagedRoom;
    /** Saves a sparse patch. Resolves to true when the room accepted it. */
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const universal = new DraftField(() => room.isUniversal);
  let form = $state<RoomSettingsForm>();
  const changed = $derived(universal.value !== universal.original);

  /** Lets the page guard navigation without copying the section's draft state. */
  export function hasChanges(): boolean {
    return form?.hasChanges() ?? changed;
  }

  function discard(): void {
    universal.value = room.isUniversal;
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.visibility_title')}
  subtitle={m('admin.rooms_admin.settings.visibility_subtitle')}
  icon="iconify icon-[uil--eye]"
>
  <RoomSettingsForm
    bind:this={form}
    {changed}
    onSave={() => onSave({ universal: universal.value })}
    onDiscard={discard}
  >
    {#snippet children(saving: boolean)}
      <Checkbox
        id="room-settings-universal"
        bind:checked={universal.value}
        disabled={saving}
        label={m('admin.rooms_admin.universal_room')}
        description={m('admin.rooms_admin.settings.universal_description')}
      />
      <Hint>{m('admin.rooms_admin.settings.visibility_help')}</Hint>
    {/snippet}
  </RoomSettingsForm>
</Panel>
