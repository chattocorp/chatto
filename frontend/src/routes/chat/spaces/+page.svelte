<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import SpaceDirectory from '$lib/SpaceDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const homeInstanceId = $derived(instanceRegistry.originInstance?.id ?? '');

  function handleSpaceJoined() {
    goto(resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(homeInstanceId) }));
  }
</script>

<PageTitle title="Browse Spaces" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Browse Spaces" showMobileNav />

  <div class="flex-1 overflow-auto p-6">
    <SpaceDirectory onspacejoined={handleSpaceJoined} />
  </div>
</div>
