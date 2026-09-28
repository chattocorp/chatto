<script lang="ts">
  import { PaneHeader } from '$lib/ui';

  let { serverName }: { serverName: string } = $props();
</script>

<PaneHeader title={serverName} />
