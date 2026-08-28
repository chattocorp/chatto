<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getLastRoom } from '$lib/storage/lastRoom';

  const serverScope = useServerScope();

  // Re-entry into a server lands on the user's last visited room when one
  // is known; otherwise it falls through to the Overview page. Keeps the
  // Overview itself reachable via its own URL.
  $effect(() => {
    const serverId = serverScope.serverId;
    const lastRoomId = getLastRoom(serverId);
    if (lastRoomId) {
      goto(
        resolve('/chat/[serverId]/[roomId]', {
          serverId: serverIdToSegment(serverId),
          roomId: lastRoomId
        }),
        { replaceState: true }
      );
      return;
    }
    goto(resolve('/chat/[serverId]/overview', { serverId: serverIdToSegment(serverId) }), {
      replaceState: true
    });
  });
</script>

<!-- Redirect in progress -->
