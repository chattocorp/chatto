<script lang="ts">
  import { pushState } from '$app/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

  const isOrigin = $derived(serverRegistry.isOriginServer(getActiveServer()));

  let {
    serverName,
    loading = false
  }: {
    serverName: string;
    loading?: boolean;
  } = $props();
</script>

<PaneHeader title={serverName} {loading} skeletonButtons={1}>
  {#snippet actions()}
    {#if !isOrigin}
      <button
        class="iconify cursor-pointer text-muted uil--sign-out-alt hover:text-text"
        onclick={() =>
          pushState('', {
            modal: { type: 'leaveServer', spaceName: serverName }
          })}
        title="Leave server"
      >
      </button>
    {/if}
  {/snippet}
</PaneHeader>
