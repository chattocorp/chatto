<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { browser } from '$app/environment';
  import { saveReturnUrl } from '$lib/auth/returnNavigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import ServerScopeProvider from '$lib/state/server/ServerScopeProvider.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import Chrome from '$lib/components/chat/Chrome.svelte';

  let { children } = $props();

  // The root layout resolves the active instance from the URL and provides
  // it via context; we just consume it here.
  const serverId = $derived(getActiveServer());

  // Guard: if the instance ID couldn't be resolved (e.g., "-" with no origin
  // instance registered), redirect to /chat. This happens when an unauthenticated
  // user navigates directly to a /chat/-/* URL before the origin is registered.
  const serverStore = $derived(serverId ? serverRegistry.tryGetStore(serverId) : undefined);

  $effect(() => {
    if (!browser) return;
    if (!serverId || !serverStore) {
      // Don't redirect while the origin probe is still in progress —
      // the "-" segment can't resolve until probeOrigin() completes.
      if (!serverRegistry.originProbed) return;

      // Server not registered — save return URL and redirect
      const currentUrl = page.url.pathname + page.url.search;
      console.warn('[chat/[serverId] layout] redirect → /login: instance not registered', {
        urlSegment: page.params.serverId,
        resolvedInstanceId: serverId || '(empty)',
        hasStore: !!serverStore,
        originProbed: serverRegistry.originProbed,
        originServer: serverRegistry.originServer?.id,
        from: currentUrl
      });
      saveReturnUrl(currentUrl);
      goto(resolve('/login'), { replaceState: true });
    }
  });

  // Auth guard: redirect unauthenticated users to /login and save the return URL.
  const currentUserState = $derived(serverStore?.currentUser);
  const reauthRequired = $derived(
    !!serverStore && serverRegistry.getServer(serverId)?.reauthRequiredAt != null
  );
  $effect(() => {
    if (!browser) return;
    if (!currentUserState) return; // No store — already redirecting above
    if (currentUserState.loading) return; // Still loading, wait
    if (reauthRequired) return; // Session/token expired; AuthStatusNotice owns recovery.
    if (currentUserState.user) return; // Authenticated, allow

    // Not authenticated on this instance — save return URL and redirect
    const currentUrl = page.url.pathname + page.url.search;
    console.warn('[chat/[serverId] layout] redirect → /login: not authenticated on instance', {
      serverId,
      hasUser: !!currentUserState.user,
      loading: currentUserState.loading,
      from: currentUrl
    });
    saveReturnUrl(currentUrl);
    goto(resolve('/login'), { replaceState: true });
  });
</script>

<!-- Authentication replacement recreates same-ID server resources, so key by
     store identity rather than only by the URL-selected server ID. -->
{#key serverStore}
  {#if serverStore}
    <ServerScopeProvider
      {serverId}
      connection={serverConnectionManager.getClient(serverId)}
      store={serverStore}
    >
      {#if reauthRequired}
        <Chrome />
      {:else if currentUserState?.user}
        <Chrome>
          {@render children?.()}
        </Chrome>
      {:else if currentUserState && !currentUserState.loading}
        <!-- Unauthenticated: the $effect above redirects to /login -->
      {:else}
        <!-- Server store exists but user state is still resolving (e.g., remote server
             loading, or brief reactive update on origin). Render children to avoid a blank
             screen — child routes validate their own access (validateServer, useRoomData). -->
        {@render children?.()}
      {/if}
    </ServerScopeProvider>
  {/if}
{/key}
