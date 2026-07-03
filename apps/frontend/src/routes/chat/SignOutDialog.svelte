<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { notifyLogout } from '$lib/auth/sessionChannel';
  import {
    beginExplicitSignOutRedirect,
    hardRedirectAfterSignOut,
    signOutServer,
    signOutServers
  } from '$lib/auth/signOut';
  import * as m from '$lib/i18n/messages';
  import Dialog from '$lib/ui/Dialog.svelte';
  import { Button } from '$lib/ui/form';

  let {
    onclose
  }: {
    onclose: () => void;
  } = $props();

  const activeInstanceId = $derived(getActiveServer());
  const currentViewedServerId = $derived(page.params.serverId ? activeInstanceId : '');
  const activeSignOutServer = $derived(
    currentViewedServerId ? serverRegistry.getServer(currentViewedServerId) : undefined
  );
  const canSignOutCurrentServer = $derived(Boolean(activeSignOutServer));

  let signingOutCurrent = $state(false);
  let signingOutAll = $state(false);

  function firstRemainingAuthenticatedServerId(excludedId: string): string | undefined {
    const originId = serverRegistry.originServer?.id;
    if (originId && originId !== excludedId && serverRegistry.isAuthenticated(originId)) {
      return originId;
    }

    return serverRegistry.servers.find(
      (server) => server.id !== excludedId && serverRegistry.isAuthenticated(server.id)
    )?.id;
  }

  function routeToServerOrRoot(serverId: string | undefined) {
    if (serverId) {
      goto(
        resolve('/chat/[serverId]', {
          serverId: serverIdToSegment(serverId)
        })
      );
      return;
    }

    goto(resolve('/'));
  }

  function hardNavigateToServerOrRoot(serverId: string | undefined) {
    hardRedirectAfterSignOut(
      serverId ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }) : '/'
    );
  }

  async function handleSignOutCurrentServer() {
    const signedOutServerId = currentViewedServerId;
    const server = activeSignOutServer;

    if (!server || !signedOutServerId) {
      return;
    }

    signingOutCurrent = true;

    if (serverRegistry.isOriginServer(signedOutServerId)) {
      beginExplicitSignOutRedirect();
    }

    await signOutServer(server, serverRegistry.isOriginServer(signedOutServerId)).catch(() => {});

    clearLastRoom(signedOutServerId);

    if (serverRegistry.isOriginServer(signedOutServerId)) {
      serverRegistry.clearServerAuthentication(signedOutServerId);
      notifyLogout();
      hardNavigateToServerOrRoot(firstRemainingAuthenticatedServerId(signedOutServerId));
    } else {
      serverRegistry.removeServer(signedOutServerId);
      routeToServerOrRoot(firstRemainingAuthenticatedServerId(signedOutServerId));
    }
  }

  async function handleSignOutAllServers() {
    signingOutAll = true;
    beginExplicitSignOutRedirect();
    await signOutServers([...serverRegistry.servers], (serverId) =>
      serverRegistry.isOriginServer(serverId)
    );
    serverRegistry.removeAll();
    notifyLogout();
    hardRedirectAfterSignOut('/');
  }
</script>

<Dialog visible title={m['chat.sign_out.title']()} size="md" {onclose}>
  {#snippet footer()}
    <div class="flex flex-wrap justify-end gap-2">
      <Button variant="secondary" onclick={onclose}>{m['common.cancel']()}</Button>
      <Button
        variant="accent"
        loading={signingOutCurrent}
        disabled={signingOutAll || !canSignOutCurrentServer}
        onclick={handleSignOutCurrentServer}
      >
        <span class="iconify uil--sign-out-alt"></span>
        {m['chat.sign_out.current_server']()}
      </Button>
      <Button
        variant="danger"
        loading={signingOutAll}
        disabled={signingOutCurrent && canSignOutCurrentServer}
        onclick={handleSignOutAllServers}
      >
        <span class="iconify uil--signout"></span>
        {m['chat.sign_out.all_servers']()}
      </Button>
    </div>
  {/snippet}

  <p class="text-muted">
    {m['chat.sign_out.description']()}
  </p>
</Dialog>
