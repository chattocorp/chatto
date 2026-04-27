<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { browser } from '$app/environment';
  import { setCurrentUser } from '$lib/auth/currentUser.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { provideConnection } from '$lib/state/instance/connection.svelte';
  import { setActiveInstance } from '$lib/state/activeInstance.svelte';
  import { segmentToInstanceId } from '$lib/navigation';
  import { provideInstanceEventBus } from '$lib/instanceEventBus.svelte';

  let { children } = $props();

  // Derive the instance ID from the URL param.
  // "-" → origin instance, hostname → look up matching remote instance.
  const instanceId = $derived(
    segmentToInstanceId(page.params.instanceId ?? '-') ?? instanceRegistry.originInstance?.id ?? ''
  );

  // Guard: if the instance ID couldn't be resolved (e.g., "-" with no origin
  // instance registered), redirect to /chat. This happens when an unauthenticated
  // user navigates directly to a /chat/-/* URL before the origin is registered.
  const instanceStore = $derived(instanceId ? instanceRegistry.tryGetStore(instanceId) : undefined);

  $effect(() => {
    if (!browser) return;
    if (!instanceId || !instanceStore) {
      // Don't redirect while the origin probe is still in progress —
      // the "-" segment can't resolve until probeOrigin() completes.
      if (!instanceRegistry.originProbed) return;

      // Instance not registered — save return URL and redirect
      const currentUrl = page.url.pathname + page.url.search;
      console.warn('[chat/[instanceId] layout] redirect → /: instance not registered', {
        urlSegment: page.params.instanceId,
        resolvedInstanceId: instanceId || '(empty)',
        hasStore: !!instanceStore,
        originProbed: instanceRegistry.originProbed,
        originInstance: instanceRegistry.originInstance?.id,
        from: currentUrl
      });
      sessionStorage.setItem('returnUrl', currentUrl);
      goto(resolve('/'), { replaceState: true });
    }
  });

  // Provide the instance ID to all child components via Svelte context.
  // This is the single source of truth for "which instance am I on".
  setActiveInstance(() => instanceId);

  // Override the parent's ConnectionProvider with the correct client for
  // this instance. Home instance paths get the home client; remote
  // hostname paths get that instance's client.
  provideConnection(() => graphqlClientManager.getClient(instanceId));

  // Override getCurrentUser() context with the per-instance current user.
  // The parent (chat/+layout.svelte) sets the home instance user. For remote
  // instances, we shadow it with the remote instance's CurrentUserState so that
  // all child components (message authorship, typing indicators, etc.) use the
  // correct user ID for this instance.
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - instanceId is stable per component lifetime
  if (instanceStore) {
    setCurrentUser(instanceStore.currentUser);
  }

  // Provide this instance's event bus to child components via Svelte context.
  // The bus is already started at the chat layout level; this just exposes it
  // so space/room components can use onInstanceEvent() and related hooks.
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - instanceId is stable per component lifetime
  if (instanceId) {
    provideInstanceEventBus(instanceId);
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
    console.warn('[chat/[instanceId] layout] redirect → /: not authenticated on instance', {
      instanceId,
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
