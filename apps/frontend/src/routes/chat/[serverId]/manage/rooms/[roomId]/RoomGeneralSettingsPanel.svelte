<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import Panel from '$lib/ui/Panel.svelte';
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
  const name = new DraftField(() => room.name);
  const description = new DraftField(() => room.description ?? '');
  const universal = new DraftField(() => room.isUniversal);
  const slowModeSeconds = new DraftField(() => room.slowModeSeconds);
  const threadingMode = new DraftField(() => room.threadingMode);

  const normalizedName = $derived(normalizeRoomName(name.value));
  const nameError = $derived.by(() => {
    if (!name.value) return undefined;
    if (name.value.trim() === '') return m('admin.rooms_admin.room_name_empty');
    if (name.value !== name.value.trim()) return m('admin.rooms_admin.room_name_trim');
    const validationError = roomNameValidationError(normalizedName);
    if (validationError === 'empty') return m('admin.rooms_admin.room_name_empty');
    if (validationError === 'too_long') {
      return m('admin.rooms_admin.room_name_too_long');
    }
    if (validationError === 'invalid') return m('admin.rooms_admin.room_name_invalid');
    return undefined;
  });
  const changed = $derived(
    normalizedName !== name.original ||
      description.value.trim() !== description.original ||
      universal.value !== universal.original ||
      slowModeSeconds.value !== slowModeSeconds.original ||
      threadingMode.value !== threadingMode.original
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
    const current = slowModeSeconds.value;
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
    if (saving || nameError || !name.value.trim() || !changed) return;
    onSave(
      buildRoomSettingsUpdate(
        room.id,
        {
          name: name.value,
          description: description.value,
          universal: universal.value,
          slowModeSeconds: slowModeSeconds.value,
          threadingMode: threadingMode.value
        },
        {
          name: name.original,
          description: description.original,
          universal: universal.original,
          slowModeSeconds: slowModeSeconds.original,
          threadingMode: threadingMode.original
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
      bind:value={name.value}
      required
      disabled={saving}
      error={nameError}
    />
    <TextArea
      id="room-settings-description"
      label={m('rbac.role_form.description')}
      bind:value={description.value}
      rows={3}
      disabled={saving}
      placeholder={m('admin.rooms_admin.room_description_placeholder')}
    />
    <Checkbox
      id="room-settings-universal"
      bind:checked={universal.value}
      disabled={saving}
      label={m('admin.rooms_admin.universal_room')}
      description={UNIVERSAL_ROOM_HELP_TEXT}
    />
    <Select
      id="room-settings-slow-mode"
      bind:value={
        () => String(slowModeSeconds.value), (value) => (slowModeSeconds.value = Number(value))
      }
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
            selected={String(threadingMode.value) === option.value}
            disabled={saving}
            onclick={() => (threadingMode.value = Number(option.value))}
          />
        {/each}
      </div>
    </div>
    <div class="flex justify-end">
      <Button
        type="submit"
        loading={saving}
        disabled={!name.value.trim() || !!nameError || !changed}
      >
        {m('admin.permissions.save_changes')}
      </Button>
    </div>
  </form>
</Panel>
