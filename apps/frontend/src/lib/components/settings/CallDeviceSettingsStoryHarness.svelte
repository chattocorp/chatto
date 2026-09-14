<script lang="ts">
  import CallDeviceSettings from './CallDeviceSettings.svelte';
  import { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  function createPreferences(unavailable: boolean) {
    const preferences = new CallPreferencesState('storybook-call-devices');
    preferences.setDevice('audioinput', unavailable ? 'unavailable-microphone' : '');
    preferences.setDevice('audiooutput', '');
    preferences.setDevice('videoinput', '');
    preferences.setJoinMuted(unavailable);
    return preferences;
  }
  let { unavailable = false, inCall = false }: { unavailable?: boolean; inCall?: boolean } =
    $props();
</script>

{#key unavailable}
  {@const preferences = createPreferences(unavailable)}
  <CallDeviceSettings {preferences} {inCall} />
{/key}
