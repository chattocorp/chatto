<!-- @component Room-owned join/leave display setting with an explicit Save action. -->
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
    /** Saves only this panel's setting and reports whether it was accepted. */
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const showEvents = new DraftField(() => room.showMembershipEvents ?? true);
  const supported = $derived(room.showMembershipEvents !== undefined);
  const changed = $derived(showEvents.value !== showEvents.original);
  let form = $state<RoomSettingsForm>();

  /** Includes pending saves in the page's navigation guard. */
  export function hasChanges(): boolean {
    return form?.hasChanges() ?? changed;
  }

  function discard(): void {
    showEvents.value = room.showMembershipEvents ?? true;
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.membership_events_title')}
  icon="iconify icon-[uil--users-alt]"
>
  <RoomSettingsForm
    bind:this={form}
    {changed}
    disabled={!supported}
    onSave={() => onSave({ showMembershipEvents: showEvents.value })}
    onDiscard={discard}
  >
    {#snippet children(saving: boolean)}
      <Checkbox
        id="room-settings-membership-events"
        bind:checked={showEvents.value}
        disabled={saving || !supported}
        label={m('admin.rooms_admin.settings.show_membership_events')}
        description={m('admin.rooms_admin.settings.membership_events_description')}
      />
      {#if !supported}
        <Hint>{m('admin.rooms_admin.settings.membership_events_unavailable')}</Hint>
      {/if}
    {/snippet}
  </RoomSettingsForm>
</Panel>
