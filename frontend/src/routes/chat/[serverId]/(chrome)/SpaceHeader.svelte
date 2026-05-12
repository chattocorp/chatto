<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

  const isOrigin = $derived(serverRegistry.isOriginInstance(getActiveServer()));

  let {
    spaceName,
    canAccessSettings = false,
    loading = false
  }: {
    spaceName: string;
    canAccessSettings?: boolean;
    loading?: boolean;
  } = $props();
</script>

<PaneHeader title={spaceName} {loading} skeletonButtons={2}>
  {#snippet actions()}
    {#if canAccessSettings}
      <a
        href={resolve('/chat/[serverId]/(chrome)/server-admin', {
          serverId: serverIdToSegment(getActiveServer()),
        })}
        class="iconify cursor-pointer text-muted uil--setting hover:text-text"
        title="Space settings"
      >
      </a>
    {/if}
    {#if !isOrigin}
      <button
        class="iconify cursor-pointer text-muted uil--sign-out-alt hover:text-text"
        onclick={() =>
          pushState('', {
            modal: { type: 'leaveServer', spaceName }
          })}
        title="Leave server"
      >
      </button>
    {/if}
  {/snippet}
</PaneHeader>
