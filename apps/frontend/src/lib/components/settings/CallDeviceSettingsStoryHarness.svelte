<script lang="ts">
  import CallDeviceSettings from './CallDeviceSettings.svelte';
  import { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  function createPreferences(unavailable: boolean, threshold: number) {
    const preferences = new CallPreferencesState('storybook-call-devices');
    preferences.setDevice('audioinput', unavailable ? 'unavailable-microphone' : '');
    preferences.setDevice('audiooutput', '');
    preferences.setDevice('videoinput', '');
    preferences.setJoinMuted(unavailable);
    preferences.setMicrophoneThreshold(threshold);
    return preferences;
  }
  let {
    unavailable = false,
    inCall = false,
    threshold = -60,
    gateUnavailable = false
  }: {
    unavailable?: boolean;
    inCall?: boolean;
    threshold?: number;
    gateUnavailable?: boolean;
  } = $props();
</script>

{#key `${unavailable}:${threshold}`}
  {@const preferences = createPreferences(unavailable, threshold)}
  <CallDeviceSettings {preferences} {inCall} {gateUnavailable} callLevel={inCall ? 0.6 : 0} />
{/key}
