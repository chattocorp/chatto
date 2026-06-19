<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  const groupId = $derived(page.params.groupId!);
  const activeServerId = $derived(getActiveServer());
  const stores = $derived(serverRegistry.getStore(activeServerId));
  const layout = $derived(stores.adminRoomLayout);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })
  );

  onMount(() => {
    if (!layout.initialized) void layout.refresh();
  });

  const group = $derived(layout.groups.find((g) => g.id === groupId) ?? null);
  const pageTitle = $derived(group ? `Permissions — ${group.name}` : 'Group permissions');
</script>

<PageTitle title={`${pageTitle} | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={group?.name ?? ''}
    subtitle="Per-group role permission grants and denials"
    {backHref}
    backLabel="Back to rooms"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if layout.error}
      <Hint tone="danger">{layout.error}</Hint>
    {/if}
    <Hint>
      Per-group overrides for the channel rooms in this group. Defaults inherited from the server scope.
      Individual rooms can further override permissions from their
      own permissions page.
    </Hint>
    <PermissionMatrix {groupId} />
  </div>
</div>
