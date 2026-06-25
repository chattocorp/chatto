<script lang="ts">
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

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

{#if adminHref}
  <PaneHeader title={serverName} {loading} skeletonButtons={1}>
    {#snippet actions()}
      <HeaderIconButton icon="uil--setting" label="Server administration" href={adminHref} />
    {/snippet}
  </PaneHeader>
{:else}
  <PaneHeader title={serverName} {loading} skeletonButtons={0} />
{/if}
