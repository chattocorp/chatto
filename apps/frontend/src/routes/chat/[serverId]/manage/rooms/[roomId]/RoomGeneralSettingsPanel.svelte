<script lang="ts">
  import { untrack } from 'svelte';
  import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { Panel } from '$lib/components/admin';
  import { Button, Checkbox, Select, TextArea, TextInput } from '$lib/ui/form';
  import { ChoiceRow } from '$lib/ui';
  import { normalizeRoomName, roomNameValidationError } from '$lib/utils/roomName';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import { buildRoomSettingsUpdate } from './roomSettings';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { formatSlowModeInterval, SLOW_MODE_PRESETS } from '$lib/slowMode';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    room,
    saving,
    onSave
  }: {
    room: AdminManagedRoom;
    saving: boolean;
    onSave: (update: ReturnType<typeof buildRoomSettingsUpdate>) => void;
  } = $props();

  // The parent keys this editor by room identity and successful save revision.
  let originalName = $state(untrack(() => room.name));
  let originalDescription = $state(untrack(() => room.description ?? ''));
  let originalUniversal = $state(untrack(() => room.isUniversal));
  let originalSlowModeSeconds = $state(untrack(() => room.slowModeSeconds));
  let originalThreadingMode = $state(untrack(() => room.threadingMode));
  let name = $state(untrack(() => room.name));
  let description = $state(untrack(() => room.description ?? ''));
  let universal = $state(untrack(() => room.isUniversal));
  let slowModeSeconds = $state(untrack(() => String(room.slowModeSeconds)));
  let threadingMode = $state(untrack(() => String(room.threadingMode)));

  // Query snapshots may refresh while this editor is mounted. Adopt each remote
  // field only when that field is pristine, preserving unrelated local edits.
  $effect(() => {
    const nextName = room.name;
    const nextDescription = room.description ?? '';
    const nextUniversal = room.isUniversal;
    const nextSlowModeSeconds = room.slowModeSeconds;
    const nextThreadingMode = room.threadingMode;
    untrack(() => {
      const nameWasPristine = name === originalName;
      const descriptionWasPristine = description === originalDescription;
      const universalWasPristine = universal === originalUniversal;
      const slowModeWasPristine = Number(slowModeSeconds) === originalSlowModeSeconds;
      const threadingModeWasPristine = Number(threadingMode) === originalThreadingMode;
      originalName = nextName;
      originalDescription = nextDescription;
      originalUniversal = nextUniversal;
      originalSlowModeSeconds = nextSlowModeSeconds;
      originalThreadingMode = nextThreadingMode;
      if (nameWasPristine) name = nextName;
      if (descriptionWasPristine) description = nextDescription;
      if (universalWasPristine) universal = nextUniversal;
      if (slowModeWasPristine) slowModeSeconds = String(nextSlowModeSeconds);
      if (threadingModeWasPristine) threadingMode = String(nextThreadingMode);
    });
  });

  const normalizedName = $derived(normalizeRoomName(name));
  const nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return m('admin.rooms_admin.room_name_empty');
    if (name !== name.trim()) return m('admin.rooms_admin.room_name_trim');
    const validationError = roomNameValidationError(normalizedName);
    if (validationError === 'empty') return m('admin.rooms_admin.room_name_empty');
    if (validationError === 'too_long') {
      return m('admin.rooms_admin.room_name_too_long');
    }
    if (validationError === 'invalid') return m('admin.rooms_admin.room_name_invalid');
    return undefined;
  });
  const changed = $derived(
    normalizedName !== originalName ||
      description.trim() !== originalDescription ||
      universal !== originalUniversal ||
      Number(slowModeSeconds) !== originalSlowModeSeconds ||
      Number(threadingMode) !== originalThreadingMode
  );
  const slowModeOptions = $derived.by(() => {
    const locale = getLocale();
    const options = SLOW_MODE_PRESETS.map((seconds) => ({
      value: String(seconds),
      label:
        seconds === 0
          ? m('admin.rooms_admin.slow_mode_off')
          : formatSlowModeInterval(seconds, locale)
    }));
    const current = Number(slowModeSeconds);
    if (!SLOW_MODE_PRESETS.some((seconds) => seconds === current)) {
      options.splice(1, 0, {
        value: String(current),
        label: m('admin.rooms_admin.slow_mode_custom', {
          interval: formatSlowModeInterval(current, locale)
        })
      });
    }
    return options;
  });
  const threadingModeOptions = $derived([
    {
      value: String(RoomThreadingMode.REQUIRED),
      label: m('admin.rooms_admin.threading_mode_required'),
      description: m('admin.rooms_admin.threading_mode_required_description')
    },
    {
      value: String(RoomThreadingMode.ENCOURAGED),
      label: m('admin.rooms_admin.threading_mode_encouraged'),
      description: m('admin.rooms_admin.threading_mode_encouraged_description')
    },
    {
      value: String(RoomThreadingMode.ENABLED),
      label: m('admin.rooms_admin.threading_mode_enabled'),
      description: m('admin.rooms_admin.threading_mode_enabled_description')
    },
    {
      value: String(RoomThreadingMode.DISABLED),
      label: m('admin.rooms_admin.threading_mode_disabled'),
      description: m('admin.rooms_admin.threading_mode_disabled_description')
    }
  ]);

  function save(event: SubmitEvent): void {
    event.preventDefault();
    if (saving || nameError || !name.trim() || !changed) return;
    onSave(
      buildRoomSettingsUpdate(
        room.id,
        {
          name,
          description,
          universal,
          slowModeSeconds: Number(slowModeSeconds),
          threadingMode: Number(threadingMode) as RoomThreadingMode
        },
        {
          name: originalName,
          description: originalDescription,
          universal: originalUniversal,
          slowModeSeconds: originalSlowModeSeconds,
          threadingMode: originalThreadingMode
        }
      )
    );
  }
</script>

<Panel title={m('admin.nav.general')} icon="iconify icon-[uil--setting]">
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <TextInput
      id="room-settings-name"
      label={m('rbac.role_form.name')}
      bind:value={name}
      required
      disabled={saving}
      error={nameError}
    />
    <TextArea
      id="room-settings-description"
      label={m('rbac.role_form.description')}
      bind:value={description}
      rows={3}
      disabled={saving}
      placeholder={m('admin.rooms_admin.room_description_placeholder')}
    />
    <Checkbox
      id="room-settings-universal"
      bind:checked={universal}
      disabled={saving}
      label={m('admin.rooms_admin.universal_room')}
      description={UNIVERSAL_ROOM_HELP_TEXT}
    />
    <Select
      id="room-settings-slow-mode"
      bind:value={slowModeSeconds}
      disabled={saving}
      label={m('admin.rooms_admin.slow_mode')}
      description={m('admin.rooms_admin.slow_mode_description')}
      options={slowModeOptions}
    />
    <div class="flex flex-col gap-2">
      <div>
        <p id="room-settings-threading-mode-label" class="font-medium">
          {m('admin.rooms_admin.threading_mode')}
        </p>
      </div>
      <div
        class="flex flex-col gap-2"
        role="radiogroup"
        aria-labelledby="room-settings-threading-mode-label"
      >
        {#each threadingModeOptions as option (option.value)}
          <ChoiceRow
            label={option.label}
            description={option.description}
            selected={threadingMode === option.value}
            disabled={saving}
            onclick={() => (threadingMode = option.value)}
          />
        {/each}
      </div>
    </div>
    <div class="flex justify-end">
      <Button type="submit" loading={saving} disabled={!name.trim() || !!nameError || !changed}>
        {m('admin.permissions.save_changes')}
      </Button>
    </div>
  </form>
</Panel>
