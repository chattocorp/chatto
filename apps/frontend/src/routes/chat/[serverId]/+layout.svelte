<script lang="ts">
  import { page } from '$app/state';
  import { serverRegistry, serverConnectionManager } from '$lib/client';
  import ServerScopeProvider from '$lib/state/server/ServerScopeProvider.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import Chrome from '$lib/components/chat/Chrome.svelte';
  import ServerUnavailable from '$lib/components/chat/ServerUnavailable.svelte';
  import ServerSignedOut from '$lib/components/chat/ServerSignedOut.svelte';
  import { showsServerSignedOut } from '$lib/components/chat/serverSignedOut';
  import { PageTitle } from '$lib/ui';
  import { isInlineSignInRequested } from '$lib/auth/remoteSignIn.svelte';

  let { children } = $props();

  // Resolve the URL here and provide one stable server scope to its route subtree.
  const serverId = $derived(getActiveServer());

  // Guard: if the instance ID couldn't be resolved (e.g., "-" with no origin
  // instance registered), the layout load redirects before this component mounts.
  const serverStore = $derived(serverId ? serverRegistry.tryGetStore(serverId) : undefined);

  // Only discovery failures and releases below the connection floor block the
  // route. Limited and unknown releases attempt the normal UI with a warning.
  const compatibilityProblem = $derived(
    serverStore?.serverInfo.blockingCompatibilityProblem ?? null
  );
  // Discovery is public. Keep its report accessible without a usable chat session.
  const isInformationPage = $derived(page.route.id === '/chat/[serverId]/info');

  // A known remote server without a usable session. Explain this instead of
  // server chrome that would stay empty.
  const registration = $derived(serverRegistry.getServer(serverId));
  const signedOutRegistration = $derived(
    registration &&
      serverStore &&
      (isInlineSignInRequested(serverId) ||
        showsServerSignedOut(registration, {
          isOrigin: serverRegistry.isOriginServer(serverId),
          hasDisplayableView: serverStore.realtimeSync.hasDisplayableView
        }))
      ? registration
      : null
  );
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
      {#if isInformationPage && (compatibilityProblem || signedOutRegistration || !serverStore.realtimeSync.hasDisplayableView)}
        {@render children?.()}
      {:else if compatibilityProblem}
        <ServerUnavailable
          reason={compatibilityProblem}
          {registration}
          removable={!serverRegistry.isOriginServer(serverId)}
          onretry={() => serverRegistry.recoverServer(serverId)}
        />
      {:else if signedOutRegistration}
        <ServerSignedOut registration={signedOutRegistration} />
      {:else}
        <Chrome>
          {#if serverStore.realtimeSync.hasDisplayableView}
            <div class="contents" aria-busy={serverStore.realtimeSync.isRecoveringSnapshot}>
              {@render children?.()}
            </div>
          {:else}
            <PageTitle />
          {/if}
        </Chrome>
      {/if}
    </ServerScopeProvider>
  {:else}
    <PageTitle />
  {/if}
{/key}
