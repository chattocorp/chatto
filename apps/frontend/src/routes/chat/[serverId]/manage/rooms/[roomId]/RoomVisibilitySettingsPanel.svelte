<!--
@component

Visibility of a room. The checkbox edits a local draft until the form submits.
-->
<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { Hint, Panel } from '$lib/ui';
  import { Button, Checkbox } from '$lib/ui/form';
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
  let saving = $state(false);
  const changed = $derived(universal.value !== universal.original);

  /** Lets the page guard navigation without copying the section's draft state. */
  export function hasChanges(): boolean {
    return changed || saving;
  }

  function discard(): void {
    universal.value = room.isUniversal;
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (saving || !changed) return;
    saving = true;
    try {
      if (await onSave({ universal: universal.value })) discard();
    } finally {
      saving = false;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.visibility_title')}
  subtitle={m('admin.rooms_admin.settings.visibility_subtitle')}
  icon="iconify icon-[uil--eye]"
>
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <Checkbox
      id="room-settings-universal"
      bind:checked={universal.value}
      disabled={saving}
      label={m('admin.rooms_admin.universal_room')}
      description={m('admin.rooms_admin.settings.universal_description')}
    />
    <Hint>{m('admin.rooms_admin.settings.visibility_help')}</Hint>
    <div class="flex justify-end gap-2">
      <Button variant="secondary" disabled={saving || !changed} onclick={discard}>
        {m('admin.rooms_admin.settings.discard_changes')}
      </Button>
      <Button type="submit" loading={saving} disabled={!changed}>
        {m('admin.permissions.save_changes')}
      </Button>
    </div>
  </form>
</Panel>
