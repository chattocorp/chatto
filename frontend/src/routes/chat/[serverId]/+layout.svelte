<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { browser } from '$app/environment';
  import { setCurrentUser } from '$lib/auth/currentUser.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { provideConnection } from '$lib/state/server/connection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { provideServerEventBus } from '$lib/serverEventBus.svelte';

  let { children } = $props();

  // The root layout resolves the active instance from the URL and provides
  // it via context; we just consume it here.
  const getInstanceId = getActiveServer();
  const serverId = $derived(getInstanceId());

  // Guard: if the instance ID couldn't be resolved (e.g., "-" with no origin
  // instance registered), redirect to /chat. This happens when an unauthenticated
  // user navigates directly to a /chat/-/* URL before the origin is registered.
  const instanceStore = $derived(serverId ? serverRegistry.tryGetStore(serverId) : undefined);

  $effect(() => {
    if (!browser) return;
    if (!serverId || !instanceStore) {
      // Don't redirect while the origin probe is still in progress —
      // the "-" segment can't resolve until probeOrigin() completes.
      if (!serverRegistry.originProbed) return;

      // Instance not registered — save return URL and redirect
      const currentUrl = page.url.pathname + page.url.search;
      console.warn('[chat/[serverId] layout] redirect → /: instance not registered', {
        urlSegment: page.params.serverId,
        resolvedInstanceId: serverId || '(empty)',
        hasStore: !!instanceStore,
        originProbed: serverRegistry.originProbed,
        originServer: serverRegistry.originServer?.id,
        from: currentUrl
      });
      sessionStorage.setItem('returnUrl', currentUrl);
      goto(resolve('/'), { replaceState: true });
    }
  });

  // The active instance context is provided by the root layout. We just
  // override the parent's ConnectionProvider with the correct client for
  // this instance — origin paths get the origin client; hostname paths get
  // that instance's client.
  provideConnection(() => graphqlClientManager.getClient(serverId));

  // Override getCurrentUser() context with the per-instance current user.
  // The parent (chat/+layout.svelte) sets the home instance user. For remote
  // instances, we shadow it with the remote instance's CurrentUserState so that
  // all child components (message authorship, typing indicators, etc.) use the
  // correct user ID for this instance.
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime
  if (instanceStore) {
    setCurrentUser(instanceStore.currentUser);
  }

  // Provide this instance's event bus to child components via Svelte context.
  // The bus is already started at the chat layout level; this just exposes it
  // so space/room components can use onServerEvent() and related hooks.
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime
  if (serverId) {
    provideServerEventBus(serverId);
  }

  // Auth guard: redirect unauthenticated users to /chat and save the return URL.
  const currentUserState = $derived(instanceStore?.currentUser);
  $effect(() => {
    if (!browser) return;
    if (!currentUserState) return; // No store — already redirecting above
    if (currentUserState.loading) return; // Still loading, wait
    if (currentUserState.user) return; // Authenticated, allow

    // Not authenticated on this instance — save return URL and redirect
    const currentUrl = page.url.pathname + page.url.search;
    console.warn('[chat/[serverId] layout] redirect → /: not authenticated on instance', {
      serverId,
      hasUser: !!currentUserState.user,
      loading: currentUserState.loading,
      from: currentUrl
    });
    sessionStorage.setItem('returnUrl', currentUrl);
    goto(resolve('/'), { replaceState: true });
  });
</script>

{#if currentUserState?.user}
  {@render children?.()}
{:else if currentUserState && !currentUserState.loading}
  <!-- Unauthenticated: the $effect above redirects to /chat -->
{:else if instanceStore}
  <!-- Instance store exists but user state is still resolving (e.g., remote instance
       loading, or brief reactive update on origin). Render children to avoid a blank
       screen — child routes validate their own access (validateSpace, useRoomData). -->
  {@render children?.()}
{/if}
