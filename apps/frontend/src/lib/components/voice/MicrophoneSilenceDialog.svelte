<!-- @component Explains the quiet-input warning without starting another microphone capture. -->
<script lang="ts">
  import { Dialog } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { Button } from '$lib/ui/form';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';

  let { serverId, onclose }: { serverId: string; onclose: () => void } = $props();

  function openPreferences() {
    // Replace the modal history entry so Back does not reopen the warning.
    void goto(
      resolve('/chat/[serverId]/settings/voice', { serverId: serverIdToSegment(serverId) }),
      { replaceState: true }
    );
  }
</script>

<Dialog visible title={m('voice.microphone_silent_hint')} size="sm" {onclose}>
  <p>{m('voice.microphone_silence_explanation')}</p>
  {#snippet footer()}
    <Button variant="secondary" onclick={onclose}>{m('ui.close')}</Button>
    <Button onclick={openPreferences}>{m('voice.preferences.title')}</Button>
  {/snippet}
</Dialog>
