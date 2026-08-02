<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { RemoveServerModalState } from '$lib/modal';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import * as m from '$lib/i18n/messages';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';

  let {
    modal,
    onclose
  }: {
    modal: RemoveServerModalState;
    onclose: () => void;
  } = $props();

  const activeServerId = $derived(getActiveServer());

  function removeServer() {
    const removingActiveServer = modal.serverId === activeServerId;
    clearLastRoom(modal.serverId);
    serverRegistry.removeServer(modal.serverId);

    if (!removingActiveServer) {
      onclose();
      return;
    }

    const originId = serverRegistry.originServer?.id;
    if (originId && originId !== modal.serverId) {
      goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(originId) }));
    } else {
      goto(resolve('/'));
    }
  }
</script>

<ConfirmDialog
  title={m['room.server.remove_title']()}
  actionLabel={m['room.server.remove_action']()}
  actionIcon="iconify uil--minus-circle"
  onconfirm={removeServer}
  {onclose}
>
  <p>{m['room.server.remove_prompt']({ server: modal.spaceName })}</p>
  <p class="mt-3 text-sm text-muted">
    {m['room.server.remove_account_prefix']()}
    <a
      href={resolve('/chat/[serverId]/settings/account', {
        serverId: serverIdToSegment(modal.serverId)
      })}
      class="link">{m['room.server.remove_account_link']()}</a
    >{m['room.server.remove_account_suffix']()}
  </p>
</ConfirmDialog>
