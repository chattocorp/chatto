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

  const roomId = $derived(page.params.roomId!);
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

  const room = $derived.by(() => {
    for (const group of layout.groups) {
      const match = group.rooms.find((candidate) => candidate.id === roomId);
      if (match) return match;
    }
    return null;
  });
  const pageTitle = $derived(room ? `Permissions — #${room.name}` : 'Room permissions');
</script>

<PageTitle title={`${pageTitle} | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={room ? `#${room.name}` : ''}
    subtitle="Per-room override permissions (layered on top of the room's group)"
    {backHref}
    backLabel="Back to rooms"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if layout.error}
      <Hint tone="danger">{layout.error}</Hint>
    {/if}
    <Hint>
      Per-room overrides for this room. Values set here take precedence over the group's
      and the server-wide defaults.
    </Hint>
    <PermissionMatrix {roomId} />
  </div>
</div>
