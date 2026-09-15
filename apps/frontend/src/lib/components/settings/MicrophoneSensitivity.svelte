<!-- @component Shared pre-gate meter and browser-local transmission threshold. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import RangeField from '$lib/ui/form/RangeField.svelte';
  import { Hint } from '$lib/ui';
  import type { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  let {
    preferences,
    level = 0,
    unavailable = false,
    id = 'microphone-sensitivity'
  }: {
    preferences: CallPreferencesState;
    level?: number;
    unavailable?: boolean;
    id?: string;
  } = $props();
</script>

<div class="flex flex-col gap-2">
  <RangeField
    {id}
    label={m('voice.preferences.sensitivity')}
    min={-60}
    max={0}
    bind:value={
      () => preferences.microphoneThreshold,
      (value) => preferences.setMicrophoneThreshold(value ?? -60)
    }
    displayValue={preferences.microphoneThreshold === -60
      ? m('voice.preferences.gate_off')
      : `${preferences.microphoneThreshold} dB`}
    disabled={unavailable}
  />
  <div
    role="meter"
    aria-label={m('voice.preferences.input_level')}
    aria-valuemin="0"
    aria-valuemax="1"
    aria-valuenow={level}
    class="relative h-3 overflow-hidden rounded-full bg-surface"
  >
    <div class="h-full rounded-full bg-action" style:width={`${level * 100}%`}></div>
    {#if preferences.microphoneThreshold > -60}
      <div
        aria-hidden="true"
        class="absolute inset-y-0 w-0.5 bg-text"
        style:left={`${((preferences.microphoneThreshold + 60) / 60) * 100}%`}
      ></div>
    {/if}
  </div>
  {#if unavailable}<Hint>{m('voice.preferences.gate_unavailable')}</Hint>{/if}
</div>
