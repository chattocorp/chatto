<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { clientAccount, type ClientAccountNavigation } from '$lib/state/clientAccount';
  import { hardRedirectAfterSignOut } from '$lib/auth/signOut';
  import { m } from '$lib/i18n/messages';
  import Dialog from '$lib/ui/Dialog.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

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

  function applyNavigation(navigation: ClientAccountNavigation): void {
    if (navigation.kind === 'hard') {
      hardNavigateToServerOrRoot(navigation.serverId);
      return;
    }
    routeToServerOrRoot(navigation.serverId);
  }

  async function handleSignOutCurrentServer() {
    const signedOutServerId = currentViewedServerId;
    if (!activeSignOutServer || !signedOutServerId) return;

    signingOutCurrent = true;
    try {
      const navigation = await clientAccount.signOutCurrentServer(signedOutServerId);
      if (navigation) applyNavigation(navigation);
    } catch {
      signingOutCurrent = false;
      toast.error(m('common.error.network'));
    }
  }

  async function handleSignOutAllServers() {
    signingOutAll = true;
    try {
      applyNavigation(await clientAccount.signOutAllServers());
    } catch {
      signingOutAll = false;
      toast.error(m('common.error.network'));
    }
  }
</script>

<Dialog visible title={m('chat.sign_out.title')} size="md" {onclose}>
  {#snippet footer()}
    <div class="flex flex-wrap justify-end gap-2">
      <Button variant="secondary" onclick={onclose}>{m('common.cancel')}</Button>
      <Button
        variant="action"
        loading={signingOutCurrent}
        disabled={signingOutAll || !canSignOutCurrentServer}
        onclick={handleSignOutCurrentServer}
      >
        <span class="iconify icon-[uil--sign-out-alt]"></span>
        {m('chat.sign_out.current_server')}
      </Button>
      <Button
        variant="danger"
        loading={signingOutAll}
        disabled={signingOutCurrent && canSignOutCurrentServer}
        onclick={handleSignOutAllServers}
      >
        <span class="iconify icon-[uil--signout]"></span>
        {m('chat.sign_out.all_servers')}
      </Button>
    </div>
  {/snippet}

  <p class="text-muted">
    {m('chat.sign_out.description')}
  </p>
</Dialog>
