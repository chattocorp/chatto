<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { resolveLastPosition } from '$lib/storage/lastRoom';

  let { data } = $props();

  const serverPerms = getServerPermissions();

  // Unauthenticated → redirect immediately (no $effect needed)
  // svelte-ignore state_referenced_locally
  if (!data.user) {
    goto(resolve('/login'), { replaceState: true });
  }

  // Authenticated → use $effect to wait for reactive state (instances, permissions)
  $effect(() => {
    if (!data.user) return;
    if (sessionStorage.getItem('returnUrl')) return;

    if (serverRegistry.servers.length === 0) {
      goto(resolve('/login'), { replaceState: true });
      return;
    }

    const homeId = serverRegistry.originServer?.id ?? '';
    if (!homeId) return;

    const lastPos = resolveLastPosition(homeId);
    if (lastPos) {
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- lastPos from resolveLastPosition() is already resolved
      goto(lastPos, { replaceState: true });
      return;
    }

    if (!serverPerms.current.loaded) return;

    // Land in the server's chrome — its +page redirects to the user's room
    // (or to /chat/spaces / welcome state) once the primary spaceId resolves.
    // Issue #330 / ADR-027: with auto-join, every authenticated user is in
    // the server, so /chat/spaces is no longer the right default landing.
    goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(homeId) }), {
      replaceState: true
    });
  });
</script>

<!-- Redirect in progress -->
