<!-- @component Local microphone effects; changes apply to the active call or voice test. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { Checkbox } from '$lib/ui/form';
  import type { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  import MicrophoneSensitivity from './MicrophoneSensitivity.svelte';

  let {
    preferences,
    level = 0,
    unavailable = false,
    onBoostChange,
    onThresholdChange
  }: {
    preferences: CallPreferencesState;
    level?: number;
    unavailable?: boolean;
    onBoostChange?: (enabled: boolean) => Promise<void>;
    onThresholdChange?: (value: number) => Promise<void>;
  } = $props();
  let changing = $state(false);

  async function changeBoost(enabled: boolean): Promise<void> {
    if (changing) return;
    changing = true;
    try {
      if (onBoostChange) await onBoostChange(enabled);
      else preferences.setVoiceBoosting(enabled);
    } finally {
      changing = false;
    }
  }
</script>

<div class="flex flex-col gap-5">
  <Checkbox
    id="microphone-voice-boosting"
    label={m('voice.preferences.voice_boosting')}
    description={m('voice.preferences.voice_boosting_description')}
    disabled={changing || (unavailable && !preferences.voiceBoosting)}
    bind:checked={() => preferences.voiceBoosting, changeBoost}
  />
  <MicrophoneSensitivity {preferences} {level} {unavailable} {onThresholdChange} />
</div>
