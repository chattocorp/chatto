<!--
@component

Slow Mode of a room. The interval saves immediately; the select keeps the
saved interval visible until the room accepts the change.
-->
<script lang="ts">
  import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { Hint, Panel } from '$lib/ui';
  import { Select } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { formatSlowModeInterval, SLOW_MODE_PRESETS } from '$lib/slowMode';
  import type { RoomSettingsPatch } from './roomSettings';

  let {
    room,
    onSave
  }: {
    room: AdminManagedRoom;
    /** Saves a sparse patch. Resolves to true when the room accepted it. */
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const options = $derived.by(() => {
    const locale = getLocale();
    const presets = SLOW_MODE_PRESETS.map((seconds) => ({
      value: String(seconds),
      label:
        seconds === 0
          ? m('admin.rooms_admin.slow_mode_off')
          : formatSlowModeInterval(seconds, locale)
    }));
    const current = room.slowModeSeconds;
    if (!SLOW_MODE_PRESETS.some((seconds) => seconds === current)) {
      presets.splice(1, 0, {
        value: String(current),
        label: m('admin.rooms_admin.slow_mode_custom', {
          interval: formatSlowModeInterval(current, locale)
        })
      });
    }
    return presets;
  });

  async function change(value: string): Promise<void> {
    const slowModeSeconds = Number(value);
    if (slowModeSeconds === room.slowModeSeconds) return;
    await onSave({ slowModeSeconds });
  }
</script>

<Panel
  title={m('admin.rooms_admin.slow_mode')}
  subtitle={m('admin.rooms_admin.settings.slow_mode_subtitle')}
  icon="iconify icon-[uil--stopwatch]"
>
  <div class="flex max-w-2xl flex-col gap-4">
    <Select
      id="room-settings-slow-mode"
      value={String(room.slowModeSeconds)}
      label={m('admin.rooms_admin.settings.slow_mode_interval')}
      {options}
      onValueChange={change}
    />
    <Hint>{m('admin.rooms_admin.settings.slow_mode_help')}</Hint>
  </div>
</Panel>
