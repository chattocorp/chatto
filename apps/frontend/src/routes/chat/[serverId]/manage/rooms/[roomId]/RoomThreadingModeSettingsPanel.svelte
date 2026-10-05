<!--
@component

Threading Mode of a room. Selecting a mode saves it immediately and shows the
requested mode while the save is in progress.
-->
<script lang="ts">
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { ChoiceRow, Hint, Panel } from '$lib/ui';
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

  let requested = $state<RoomThreadingMode | null>(null);
  const selected = $derived(requested ?? room.threadingMode);

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

  async function choose(threadingMode: RoomThreadingMode): Promise<void> {
    if (requested !== null || threadingMode === room.threadingMode) return;
    requested = threadingMode;
    try {
      await onSave({ threadingMode });
    } finally {
      requested = null;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.threading_title')}
  subtitle={m('admin.rooms_admin.settings.threading_subtitle')}
  icon="iconify icon-[uil--comments-alt]"
>
  <div class="flex max-w-2xl flex-col gap-4">
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m('admin.rooms_admin.settings.threading_title')}
      aria-busy={requested !== null || undefined}
    >
      {#each options as option (option.value)}
        <ChoiceRow
          label={option.label}
          description={option.description}
          selected={selected === option.value}
          disabled={requested !== null}
          onclick={() => void choose(option.value)}
        />
      {/each}
    </div>
    <Hint>{m('admin.rooms_admin.settings.threading_help')}</Hint>
  </div>
</Panel>
