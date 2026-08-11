<script lang="ts">
  import { untrack } from 'svelte';
  import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { Panel } from '$lib/components/admin';
  import { Button, Checkbox, Select, TextArea, TextInput } from '$lib/ui/form';
  import {
    hasValidRoomNameCharacters,
    normalizeRoomName,
    ROOM_NAME_MAX_LENGTH,
    roomNameCharacterCount
  } from '$lib/utils/roomName';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import { buildRoomSettingsUpdate } from './roomSettings';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { formatSlowModeInterval, SLOW_MODE_PRESETS } from '$lib/slowMode';

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
  let name = $state(untrack(() => room.name));
  let description = $state(untrack(() => room.description ?? ''));
  let universal = $state(untrack(() => room.isUniversal));
  let slowModeSeconds = $state(untrack(() => String(room.slowModeSeconds)));

  // Query snapshots may refresh while this editor is mounted. Adopt each remote
  // field only when that field is pristine, preserving unrelated local edits.
  $effect(() => {
    const nextName = room.name;
    const nextDescription = room.description ?? '';
    const nextUniversal = room.isUniversal;
    const nextSlowModeSeconds = room.slowModeSeconds;
    untrack(() => {
      const nameWasPristine = name === originalName;
      const descriptionWasPristine = description === originalDescription;
      const universalWasPristine = universal === originalUniversal;
      const slowModeWasPristine = Number(slowModeSeconds) === originalSlowModeSeconds;
      originalName = nextName;
      originalDescription = nextDescription;
      originalUniversal = nextUniversal;
      originalSlowModeSeconds = nextSlowModeSeconds;
      if (nameWasPristine) name = nextName;
      if (descriptionWasPristine) description = nextDescription;
      if (universalWasPristine) universal = nextUniversal;
      if (slowModeWasPristine) slowModeSeconds = String(nextSlowModeSeconds);
    });
  });

  const normalizedName = $derived(normalizeRoomName(name));
  const nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return m('admin.rooms_admin.room_name_empty');
    if (name !== name.trim()) return m('admin.rooms_admin.room_name_trim');
    if (!hasValidRoomNameCharacters(normalizedName)) {
      return m('admin.rooms_admin.room_name_charset');
    }
    if (roomNameCharacterCount(normalizedName) > ROOM_NAME_MAX_LENGTH) {
      return m('admin.rooms_admin.room_name_too_long');
    }
    return undefined;
  });
  const changed = $derived(
    normalizedName !== originalName ||
      description.trim() !== originalDescription ||
      universal !== originalUniversal ||
      Number(slowModeSeconds) !== originalSlowModeSeconds
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

  function save(event: SubmitEvent): void {
    event.preventDefault();
    if (saving || nameError || !name.trim() || !changed) return;
    onSave(
      buildRoomSettingsUpdate(
        room.id,
        { name, description, universal, slowModeSeconds: Number(slowModeSeconds) },
        {
          name: originalName,
          description: originalDescription,
          universal: originalUniversal,
          slowModeSeconds: originalSlowModeSeconds
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
    <div class="flex justify-end">
      <Button type="submit" loading={saving} disabled={!name.trim() || !!nameError || !changed}>
        {m('admin.permissions.save_changes')}
      </Button>
    </div>
  </form>
</Panel>
