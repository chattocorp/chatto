<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getServerPermissions } from '$lib/state/instance/permissions.svelte';
  import { resolveLastPosition } from '$lib/storage/lastRoom';

  let { data } = $props();

  const instancePerms = getServerPermissions();

  // Unauthenticated → redirect immediately (no $effect needed)
  // svelte-ignore state_referenced_locally
  if (!data.user) {
    goto(resolve('/login'), { replaceState: true });
  }

  // Authenticated → use $effect to wait for reactive state (instances, permissions)
  $effect(() => {
    if (!data.user) return;
    if (sessionStorage.getItem('returnUrl')) return;

    if (instanceRegistry.instances.length === 0) {
      goto(resolve('/login'), { replaceState: true });
      return;
    }

    const homeId = instanceRegistry.originInstance?.id ?? '';
    if (!homeId) return;

    const lastPos = resolveLastPosition(homeId);
    if (lastPos) {
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- lastPos from resolveLastPosition() is already resolved
      goto(lastPos, { replaceState: true });
      return;
    }

    if (!instancePerms.current.loaded) return;

    // Land in the server's chrome — its +page redirects to the user's room
    // (or to /chat/spaces / welcome state) once the primary spaceId resolves.
    // Issue #330 / ADR-027: with auto-join, every authenticated user is in
    // the server, so /chat/spaces is no longer the right default landing.
    goto(resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(homeId) }), {
      replaceState: true
    });
  });
</script>

<!-- Redirect in progress -->
