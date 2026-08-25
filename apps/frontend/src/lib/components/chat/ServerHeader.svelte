<script lang="ts">
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

  let {
    serverName,
    loading = false
  }: {
    serverName: string;
    loading?: boolean;
  } = $props();
</script>

<PaneHeader title={serverName} {loading} skeletonButtons={0} />
