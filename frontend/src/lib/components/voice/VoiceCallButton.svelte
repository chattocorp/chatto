<!--
@component

Phone icon button for joining voice calls. Shown in the room header.
Hidden when already in this call (the VoiceCallPanel provides leave controls).

States:
- Idle: phone icon (click to join)
- Connecting: spinner
- In call: hidden (panel handles it)
- Disabled: already in another call

**Props:**
- `roomId` - The room ID
- `livekitUrl` - The LiveKit server WebSocket URL
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const stores = serverRegistry.getStore(getActiveServer());
  const voiceCallState = stores.voiceCall;
  import { toast } from '$lib/ui/toast';

  let {
    roomId,
    livekitUrl
  }: {
    roomId: string;
    livekitUrl: string;
  } = $props();

  let isInThisCall = $derived(voiceCallState.isInCall(roomId));
  let isInAnotherCall = $derived(voiceCallState.isInAnyCall && !isInThisCall);
  let isConnecting = $derived(voiceCallState.connecting && voiceCallState.roomId === roomId);

  async function handleJoin() {
    try {
      await voiceCallState.join(livekitUrl, roomId);
    } catch {
      stores.handleVoiceCallJoinFailed(roomId);
      toast.error('Failed to join voice call');
    }
  }
</script>

{#if isConnecting}
  <span class="group/pane-header-icon-button pane-header-icon-button" title="Connecting...">
    <span class="pane-header-icon-glyph animate-spin uil--spinner" aria-hidden="true"></span>
  </span>
{:else if !isInThisCall}
  <button
    type="button"
    class="group/pane-header-icon-button pane-header-icon-button"
    onclick={handleJoin}
    disabled={isInAnotherCall}
    title={isInAnotherCall ? 'Already in another call' : 'Join voice call'}
  >
    <span class="pane-header-icon-glyph uil--phone" aria-hidden="true"></span>
  </button>
{/if}
