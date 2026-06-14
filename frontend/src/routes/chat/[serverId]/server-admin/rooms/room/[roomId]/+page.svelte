<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  const roomId = $derived(page.params.roomId!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })
  );

  // Lightweight lookup for the room's display name; the matrix itself
  // fetches its own data via admin.rbac.rolePermissionTierMatrix.
  const RoomNameQuery = graphql(`
    query AdminRoomPermissionsName($roomId: ID!) {
      room(roomId: $roomId) {
        id
        name
      }
    }
  `);

  const nameQuery = useQuery(RoomNameQuery, () => ({ roomId }));
  const room = $derived(nameQuery.data?.room ?? null);
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
    <Hint>
      Per-room overrides for this room. Values set here take precedence over the group's
      and the server-wide defaults.
    </Hint>
    <PermissionMatrix {roomId} />
  </div>
</div>
