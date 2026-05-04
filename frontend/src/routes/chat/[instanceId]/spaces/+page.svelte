<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';

  const getInstanceId = getActiveInstance();
  import SpaceDirectory from '$lib/SpaceDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const instancePerms = getInstancePermissions();
  let canBrowseSpaces = $derived(
    !instancePerms.current.loaded ? true : instancePerms.current.canListSpaces
  );

  function handleSpaceJoined() {
    goto(
      resolve('/chat/[instanceId]', {
        instanceId: instanceIdToSegment(getInstanceId())
      })
    );
  }
</script>

<PageTitle title="Browse Spaces" />

{#if !canBrowseSpaces}
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader title="Browse Spaces" showMobileNav />
    <div class="flex flex-1 items-center justify-center p-6">
      <div class="text-center">
        <p class="text-lg text-muted">Access Denied</p>
        <p class="mt-2 text-sm text-muted">You do not have permission to browse spaces.</p>
      </div>
    </div>
  </div>
{:else}
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader title="Browse Spaces" showMobileNav />

    <div class="flex-1 overflow-auto p-6">
      <div class="max-w-5xl">
        <SpaceDirectory onspacejoined={handleSpaceJoined} />
      </div>
    </div>
  </div>
{/if}
