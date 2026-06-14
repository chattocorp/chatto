<script lang="ts">
  import { pushState } from '$app/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

  const isOrigin = $derived(serverRegistry.isOriginServer(getActiveServer()));

  let {
    serverName,
    adminHref,
    loading = false
  }: {
    serverName: string;
    adminHref?: string;
    loading?: boolean;
  } = $props();
</script>

<PaneHeader title={serverName} {loading} skeletonButtons={adminHref ? 2 : 1}>
  {#snippet actions()}
    {#if adminHref}
      <HeaderIconButton icon="uil--setting" label="Server administration" href={adminHref} />
    {/if}
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
