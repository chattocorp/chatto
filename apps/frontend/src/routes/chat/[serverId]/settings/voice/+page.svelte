<script lang="ts">
  import CallDeviceSettings from '$lib/components/settings/CallDeviceSettings.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  const scope = useServerScope();
</script>

{#key `${scope.serverId}:${scope.store.voiceCall.isInAnyCall}`}
  {#if scope.store.voiceCall.preferences}
    <CallDeviceSettings
      preferences={scope.store.voiceCall.preferences}
      inCall={scope.store.voiceCall.isInAnyCall}
      callLevel={scope.store.voiceCall.microphoneLevel}
      gateUnavailable={scope.store.voiceCall.microphoneGateUnavailable}
    />
  {/if}
{/key}
