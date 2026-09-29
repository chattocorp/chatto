<!--
@component

Visibility of a room: whether it is a Universal room. The checkbox saves
immediately and shows the requested value while the save is in progress.
-->
<script lang="ts">
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { Hint, Panel } from '$lib/ui';
  import { Checkbox } from '$lib/ui/form';
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

  let requested = $state<boolean | null>(null);

  async function change(event: Event): Promise<void> {
    const universal = (event.currentTarget as HTMLInputElement).checked;
    requested = universal;
    try {
      await onSave({ universal });
    } finally {
      requested = null;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.visibility_title')}
  subtitle={m('admin.rooms_admin.settings.visibility_subtitle')}
  icon="iconify icon-[uil--eye]"
>
  <div class="flex max-w-2xl flex-col gap-4">
    <Checkbox
      id="room-settings-universal"
      checked={requested ?? room.isUniversal}
      disabled={requested !== null}
      loading={requested !== null}
      label={m('admin.rooms_admin.universal_room')}
      description={m('admin.rooms_admin.settings.universal_description')}
      onchange={change}
    />
    <Hint>{m('admin.rooms_admin.settings.visibility_help')}</Hint>
  </div>
</Panel>
