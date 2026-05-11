<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

  const getInstanceId = getActiveInstance();
  const isOrigin = $derived(instanceRegistry.isOriginInstance(getInstanceId()));

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
        href={resolve('/chat/[instanceId]/(chrome)/server-admin', {
          instanceId: instanceIdToSegment(getInstanceId()),
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
