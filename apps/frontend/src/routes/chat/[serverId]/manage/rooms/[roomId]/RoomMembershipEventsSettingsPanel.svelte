<!-- @component Room-owned join/leave display setting that saves immediately. -->
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
    /** Saves only this panel's setting and reports whether it was accepted. */
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const supported = $derived(room.showMembershipEvents !== undefined);
  let checked = $derived(room.showMembershipEvents ?? true);
  let saving = $state(false);

  async function save(showMembershipEvents: boolean): Promise<void> {
    if (saving || !supported) return;
    saving = true;
    try {
      await onSave({ showMembershipEvents });
    } finally {
      // The room can be cleared while this request finishes after navigation.
      checked = room?.showMembershipEvents ?? true;
      saving = false;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.membership_events_title')}
  icon="iconify icon-[uil--users-alt]"
>
  <div class="flex max-w-2xl flex-col gap-4">
    <Checkbox
      id="room-settings-membership-events"
      bind:checked
      onchange={() => save(checked)}
      disabled={!supported}
      loading={saving}
      label={m('admin.rooms_admin.settings.show_membership_events')}
      description={m('admin.rooms_admin.settings.membership_events_description')}
    />
    {#if !supported}
      <Hint>{m('admin.rooms_admin.settings.membership_events_unavailable')}</Hint>
    {/if}
  </div>
</Panel>
