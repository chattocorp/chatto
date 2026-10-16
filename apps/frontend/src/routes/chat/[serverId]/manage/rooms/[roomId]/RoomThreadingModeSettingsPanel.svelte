<!--
@component

Threading Mode of a room. Selecting a mode edits a draft until the form submits.
-->
<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { ChoiceRow, Hint, Panel } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
  import type { RoomSettingsPatch } from './roomSettings';

  let {
    room,
    onSave
  }: {
    room: AdminManagedRoom;
    /** Saves a sparse patch. Resolves to true when the room accepted it. */
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const mode = new DraftField(() => room.threadingMode);
  let saving = $state(false);
  const changed = $derived(mode.value !== mode.original);

  /** Lets the page guard navigation without copying the section's draft state. */
  export function hasChanges(): boolean {
    return changed || saving;
  }

  function discard(): void {
    mode.value = room.threadingMode;
  }

  const options = $derived([
    {
      value: RoomThreadingMode.REQUIRED,
      label: m('admin.rooms_admin.threading_mode_required'),
      description: m('admin.rooms_admin.threading_mode_required_description')
    },
    {
      value: RoomThreadingMode.ENCOURAGED,
      label: m('admin.rooms_admin.threading_mode_encouraged'),
      description: m('admin.rooms_admin.threading_mode_encouraged_description')
    },
    {
      value: RoomThreadingMode.ENABLED,
      label: m('admin.rooms_admin.threading_mode_enabled'),
      description: m('admin.rooms_admin.threading_mode_enabled_description')
    },
    {
      value: RoomThreadingMode.DISABLED,
      label: m('admin.rooms_admin.threading_mode_disabled'),
      description: m('admin.rooms_admin.threading_mode_disabled_description')
    }
  ]);

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (saving || !changed) return;
    saving = true;
    try {
      if (await onSave({ threadingMode: mode.value })) discard();
    } finally {
      saving = false;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.threading_title')}
  subtitle={m('admin.rooms_admin.settings.threading_subtitle')}
  icon="iconify icon-[uil--comments-alt]"
>
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m('admin.rooms_admin.settings.threading_title')}
      aria-busy={saving || undefined}
    >
      {#each options as option (option.value)}
        <ChoiceRow
          label={option.label}
          description={option.description}
          selected={mode.value === option.value}
          disabled={saving}
          onclick={() => {
            mode.value = option.value;
          }}
        />
      {/each}
    </div>
    <Hint>{m('admin.rooms_admin.settings.threading_help')}</Hint>
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
