<script lang="ts">
  import PaneHeader from '$lib/ui/PaneHeader.svelte';

  let { serverName }: { serverName: string } = $props();
</script>

<PaneHeader title={serverName} />
