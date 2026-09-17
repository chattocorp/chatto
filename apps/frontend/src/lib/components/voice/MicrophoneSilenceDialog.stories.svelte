<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MicrophoneSilenceDialog from './MicrophoneSilenceDialog.svelte';
  const { Story } = defineMeta({
    title: 'Voice/Microphone silence explanation',
    component: MicrophoneSilenceDialog,
    args: { serverId: 'origin', onclose: () => {} }
  });
</script>

<Story name="Quiet input" />
