<!-- @component Local microphone effects; changes apply to the active call or voice test. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import Checkbox from '$lib/ui/form/Checkbox.svelte';
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
  <Checkbox
    id="microphone-voice-boosting"
    label={m('voice.preferences.voice_boosting')}
    description={m('voice.preferences.voice_boosting_description')}
    disabled={unavailable}
    bind:checked={() => preferences.voiceBoosting, (value) => preferences.setVoiceBoosting(value)}
  />
</div>
