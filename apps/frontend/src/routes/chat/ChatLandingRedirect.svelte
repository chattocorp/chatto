<!-- @component Coordinates chat landing navigation while the origin server registration settles. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import { hasPendingReturnNavigation } from '$lib/auth/returnNavigation';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { resolveLastPosition } from '$lib/storage/lastRoom';

  let { user, welcome }: { user: CurrentUser | null; welcome: boolean } = $props();

  $effect(() => {
    if (!user) {
      void goto(resolve('/'), { replaceState: true });
      return;
    }

    if (hasPendingReturnNavigation()) return;

    if (serverRegistry.servers.length === 0) {
      void goto(resolve('/login'), { replaceState: true });
      return;
    }

    const homeId = serverRegistry.originServer?.id ?? '';
    if (!homeId) return;

    const lastPos = welcome ? null : resolveLastPosition(homeId);
    if (lastPos) {
      void goto(lastPos, { replaceState: true });
      return;
    }

    // Land in the server's chrome — its +page redirects to the user's room
    // (or to /chat/spaces / welcome state) once the primary spaceId resolves.
    // Issue #330 / ADR-027: with auto-join, every authenticated user is in
    // the server, so /chat/spaces is no longer the right default landing.
    void goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(homeId) }), {
      replaceState: true,
      state: welcome ? { welcome: true } : {}
    });
  });
</script>
