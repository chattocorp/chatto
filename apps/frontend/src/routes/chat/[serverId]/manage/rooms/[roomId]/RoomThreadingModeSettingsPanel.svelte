<!--
@component

Threading Mode of a room. Selecting a mode edits a draft until the form submits.
-->
<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { ChoiceRow, Hint, Panel } from '$lib/ui';
  import RoomSettingsForm from './RoomSettingsForm.svelte';
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
  let form = $state<RoomSettingsForm>();
  const changed = $derived(mode.value !== mode.original);

  /** Lets the page guard navigation without copying the section's draft state. */
  export function hasChanges(): boolean {
    return form?.hasChanges() ?? changed;
  }

  function discard(): void {
    mode.value = room.threadingMode;
  }

  const options = $derived([
    {
      value: RoomThreadingMode.REQUIRED,
      label: m('room.management.threading_mode_required'),
      description: m('admin.rooms_admin.threading_mode_required_description')
    },
    {
      value: RoomThreadingMode.ENCOURAGED,
      label: m('room.management.threading_mode_encouraged'),
      description: m('admin.rooms_admin.threading_mode_encouraged_description')
    },
    {
      value: RoomThreadingMode.ENABLED,
      label: m('room.management.threading_mode_enabled'),
      description: m('admin.rooms_admin.threading_mode_enabled_description')
    },
    {
      value: RoomThreadingMode.DISABLED,
      label: m('room.management.threading_mode_disabled'),
      description: m('admin.rooms_admin.threading_mode_disabled_description')
    }
  ]);
</script>

<Panel
  title={m('admin.rooms_admin.settings.threading_title')}
  subtitle={m('admin.rooms_admin.settings.threading_subtitle')}
  icon="iconify icon-[uil--comments-alt]"
>
  <RoomSettingsForm
    bind:this={form}
    {changed}
    onSave={() => onSave({ threadingMode: mode.value })}
    onDiscard={discard}
  >
    {#snippet children(saving: boolean)}
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
    {/snippet}
  </RoomSettingsForm>
</Panel>
