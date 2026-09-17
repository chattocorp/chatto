<!-- @component Shared microphone-health action for the current-user and local participant cards. -->
<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import CompactActionButton from '$lib/ui/CompactActionButton.svelte';
  import { pushState } from '$app/navigation';

  const scope = useServerScope();
  const call = $derived(scope.store.voiceCall);
  function checkMicrophone() {
    pushState('', { modal: { type: 'microphoneSilence', serverId: scope.serverId } });
  }
</script>

{#if call.microphoneSilent}
  <CompactActionButton
    label={m('voice.microphone_silent_hint')}
    class="text-warning"
    onclick={checkMicrophone}
    data-testid="microphone-silence-hint"
  >
    <span class="iconify icon-[uil--exclamation-triangle]" aria-hidden="true"></span>
  </CompactActionButton>
{/if}
