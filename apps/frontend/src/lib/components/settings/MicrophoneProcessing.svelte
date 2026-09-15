<!-- @component Local microphone effects; changes apply to the active call or voice test. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import RangeField from '$lib/ui/form/RangeField.svelte';
  import type { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  import MicrophoneSensitivity from './MicrophoneSensitivity.svelte';

  let {
    preferences,
    level = 0,
    unavailable = false
  }: {
    preferences: CallPreferencesState;
    level?: number;
    unavailable?: boolean;
  } = $props();
</script>

<div class="flex flex-col gap-5">
  <MicrophoneSensitivity {preferences} {level} {unavailable} />
  <div class="flex flex-col gap-2">
    <RangeField
      id="microphone-voice"
      label={m('voice.preferences.your_voice')}
      min={0}
      max={100}
      step={0.1}
      rainbow
      disabled={unavailable}
      bind:value={() => preferences.voiceAmount, (value) => preferences.setVoiceAmount(value ?? 0)}
      displayValue={preferences.voiceAmount === 0
        ? m('voice.preferences.voice_normal')
        : preferences.voiceAmount === 50
          ? m('voice.preferences.voice_cool')
          : preferences.voiceAmount === 100
            ? m('voice.preferences.voice_awesome')
            : `${Math.round(preferences.voiceAmount)}%`}
    />
    <div class="grid grid-cols-3 gap-2 px-3 text-xs text-muted" aria-hidden="true">
      <span>{m('voice.preferences.voice_normal')}</span>
      <span class="text-center">{m('voice.preferences.voice_cool')}</span>
      <span class="text-end">{m('voice.preferences.voice_awesome')}</span>
    </div>
  </div>
</div>
