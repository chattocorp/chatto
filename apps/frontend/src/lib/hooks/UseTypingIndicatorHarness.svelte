<script lang="ts">
  import { createTypingIndicator } from './useTypingIndicator.svelte';

  let {
    getConfig
  }: {
    getConfig: Parameters<typeof createTypingIndicator>[0];
  } = $props();

  export const indicator = createTypingIndicator(() => getConfig());
</script>

<output>{indicator.userIds.join(',')}</output>
