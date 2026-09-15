<script lang="ts">
  import CallDeviceSettings from './CallDeviceSettings.svelte';
  import { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  function createPreferences(unavailable: boolean, threshold: number, effects: boolean, noiseSuppression: boolean) {
    const preferences = new CallPreferencesState('storybook-call-devices');
    preferences.setDevice('audioinput', unavailable ? 'unavailable-microphone' : '');
    preferences.setDevice('audiooutput', '');
    preferences.setDevice('videoinput', '');
    preferences.setJoinMuted(unavailable);
    preferences.setVoiceAmount(0);
    preferences.setNoiseSuppression(noiseSuppression);
    preferences.setMicrophoneThreshold(threshold);
    if (effects) preferences.setVoiceAmount(100);
    return preferences;
  }
  let {
    unavailable = false,
    inCall = false,
    threshold = -60,
    effects = false,
    noiseSuppression = false,
    gateUnavailable = false
  }: {
    effects?: boolean;
    noiseSuppression?: boolean;
    unavailable?: boolean;
    inCall?: boolean;
    threshold?: number;
    gateUnavailable?: boolean;
  } = $props();
</script>

{#key `${unavailable}:${threshold}:${effects}:${noiseSuppression}`}
  {@const preferences = createPreferences(unavailable, threshold, effects, noiseSuppression)}
  <CallDeviceSettings {preferences} {inCall} {gateUnavailable} callLevel={inCall ? 0.6 : 0} />
{/key}
