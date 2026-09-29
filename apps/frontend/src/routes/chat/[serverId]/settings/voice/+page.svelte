<script lang="ts">
  import CallDeviceSettings from '$lib/components/settings/CallDeviceSettings.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { PageTitle } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  const scope = useServerScope();
  // The scope, and so its store and call, stays the same while this page is mounted.
  const voiceCall = serverUi(scope.store).voiceCall;
</script>

<PageTitle title={m('voice.preferences.title')} />

{#key `${scope.serverId}:${voiceCall.isInAnyCall}`}
  {#if voiceCall.preferences}
    <CallDeviceSettings
      preferences={voiceCall.preferences}
      inCall={voiceCall.isInAnyCall}
      callLevel={voiceCall.microphoneLevel}
      gateUnavailable={voiceCall.microphoneGateUnavailable}
      onDeviceChange={(kind, id) =>
        kind === 'audioinput'
          ? voiceCall.setAudioDevice(id)
          : kind === 'audiooutput'
            ? voiceCall.setAudioOutputDevice(id)
            : voiceCall.setVideoDevice(id)}
    />
  {/if}
{/key}
