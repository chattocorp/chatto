<script lang="ts">
  import type { ClientSyncState } from './clientSync.svelte';

  let { state }: { state: ClientSyncState } = $props();
</script>

{#if state.hasPendingRemoteHome}
  <span data-testid="pending-home">Pending home</span>
{/if}
