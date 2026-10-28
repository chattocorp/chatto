<!-- @component Room-owned join/leave display setting with an explicit Save action. -->
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
    /** Saves only this panel's setting and reports whether it was accepted. */
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const showEvents = new DraftField(() => room.showMembershipEvents ?? true);
  const supported = $derived(room.showMembershipEvents !== undefined);
  const changed = $derived(showEvents.value !== showEvents.original);
  let saving = $state(false);

  /** Includes pending saves in the page's navigation guard. */
  export function hasChanges(): boolean {
    return changed || saving;
  }

  function discard(): void {
    showEvents.value = room.showMembershipEvents ?? true;
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!supported || saving || !changed) return;
    saving = true;
    try {
      if (await onSave({ showMembershipEvents: showEvents.value })) discard();
    } finally {
      saving = false;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.membership_events_title')}
  icon="iconify icon-[uil--users-alt]"
>
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
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
    <div class="flex justify-end gap-2">
      <Button variant="secondary" disabled={saving || !changed} onclick={discard}>
        {m('admin.rooms_admin.settings.discard_changes')}
      </Button>
      <Button type="submit" loading={saving} disabled={!supported || !changed}>
        {m('admin.permissions.save_changes')}
      </Button>
    </div>
  </form>
</Panel>
