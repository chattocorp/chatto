<!--
@component

LiveKit section of the System page: the voice and video call setup. The tab
is hidden when the server does not report LiveKit status; a direct visit
then shows a hint.
-->
<script lang="ts">
  import { Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import LiveKitPanel from '../LiveKitPanel.svelte';
  import { useSystem } from '../systemContext';

  const system = useSystem();
</script>

{#if system.info.livekit.connectionState === 'unavailable'}
  <Hint>{m('admin.system.livekit_unavailable')}</Hint>
{:else}
  <LiveKitPanel status={system.info.livekit} />
{/if}
