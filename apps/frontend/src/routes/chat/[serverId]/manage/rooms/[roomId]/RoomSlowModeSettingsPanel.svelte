<!--
@component

Slow Mode of a room. The interval stays local until the form submits.
-->
<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { Hint, Panel } from '$lib/ui';
  import { Button, Select } from '$lib/ui/form';
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

  const interval = new DraftField(() => String(room.slowModeSeconds));
  const threadInterval = new DraftField(() => String(room.threadSlowModeSeconds ?? 0));
  let saving = $state(false);
  const changed = $derived(
    interval.value !== interval.original ||
      (room.threadSlowModeSeconds !== undefined && threadInterval.value !== threadInterval.original)
  );

  /** Lets the page guard navigation without copying the section's draft state. */
  export function hasChanges(): boolean {
    return changed || saving;
  }

  function discard(): void {
    interval.value = String(room.slowModeSeconds);
    threadInterval.value = String(room.threadSlowModeSeconds ?? 0);
  }

  function intervalOptions(saved: number, draft: string) {
    const locale = getLocale();
    const presets = SLOW_MODE_PRESETS.map((seconds) => ({
      value: String(seconds),
      label:
        seconds === 0
          ? m('admin.rooms_admin.slow_mode_off')
          : formatSlowModeInterval(seconds, locale)
    }));
    // Keep a custom draft selectable even when a remote update changes the saved interval.
    for (const current of new Set([saved, Number(draft)])) {
      if (presets.some((option) => option.value === String(current))) continue;
      presets.splice(1, 0, {
        value: String(current),
        label: m('admin.rooms_admin.slow_mode_custom', {
          interval: formatSlowModeInterval(current, locale)
        })
      });
    }
    return presets;
  }

  const options = $derived(intervalOptions(room.slowModeSeconds, interval.value));
  const threadOptions = $derived(
    intervalOptions(room.threadSlowModeSeconds ?? 0, threadInterval.value)
  );

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (saving || !changed) return;
    const patch: RoomSettingsPatch = {};
    if (interval.value !== interval.original) patch.slowModeSeconds = Number(interval.value);
    if (
      room.threadSlowModeSeconds !== undefined &&
      threadInterval.value !== threadInterval.original
    ) {
      patch.threadSlowModeSeconds = Number(threadInterval.value);
    }
    saving = true;
    try {
      if (await onSave(patch)) discard();
    } finally {
      saving = false;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.slow_mode')}
  subtitle={m('admin.rooms_admin.settings.slow_mode_subtitle')}
  icon="iconify icon-[uil--stopwatch]"
>
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <Select
      id="room-settings-slow-mode"
      bind:value={interval.value}
      disabled={saving}
      label={room.threadSlowModeSeconds === undefined
        ? m('admin.rooms_admin.settings.slow_mode_interval')
        : m('admin.rooms_admin.settings.channel_slow_mode_interval')}
      {options}
    />
    {#if room.threadSlowModeSeconds !== undefined}
      <Select
        id="room-settings-thread-slow-mode"
        bind:value={threadInterval.value}
        disabled={saving}
        label={m('admin.rooms_admin.settings.thread_slow_mode_interval')}
        options={threadOptions}
      />
      <Hint>{m('admin.rooms_admin.settings.independent_slow_mode_help')}</Hint>
    {:else}
      <Hint>{m('admin.rooms_admin.settings.slow_mode_help')}</Hint>
    {/if}
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
