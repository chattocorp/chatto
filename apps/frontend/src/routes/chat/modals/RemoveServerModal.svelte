<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { RemoveServerModalState } from '$lib/modal';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { m } from '$lib/i18n/messages';
  import { unsubscribeBeforeLeaving as unsubscribePushBeforeLeaving } from '$lib/notifications/pushNotifications';
  import { ConfirmDialog } from '$lib/ui';
  import { toast } from '$lib/ui/toast';

  let {
    modal,
    onclose
  }: {
    modal: RemoveServerModalState;
    onclose: () => void;
  } = $props();

  const activeServerId = $derived(getActiveServer());
  let removing = $state(false);

  async function removeServer() {
    if (removing) return;
    removing = true;
    try {
      const removingActiveServer = modal.serverId === activeServerId;
      await unsubscribePushBeforeLeaving(modal.serverId);
      clearLastRoom(modal.serverId);
      serverRegistry.removeServer(modal.serverId);

      if (!removingActiveServer) {
        onclose();
        return;
      }

      const originId = serverRegistry.originServer?.id;
      if (originId && originId !== modal.serverId) {
        await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(originId) }));
      } else {
        await goto(resolve('/'));
      }
    } catch {
      removing = false;
      toast.error(m('common.error.network'));
    }
  }
</script>

<ConfirmDialog
  title={m('room.server.remove_title')}
  actionLabel={m('room.server.remove_action')}
  actionIcon="iconify icon-[uil--minus-circle]"
  loading={removing}
  onconfirm={removeServer}
  {onclose}
>
  <p>{m('room.server.remove_prompt', { server: modal.spaceName })}</p>
  <p class="mt-3 text-sm text-muted">
    {m('room.server.remove_account_prefix')}
    <a
      href={resolve('/chat/[serverId]/settings/account', {
        serverId: serverIdToSegment(modal.serverId)
      })}
      class="link">{m('room.server.remove_account_link')}</a
    >{m('room.server.remove_account_suffix')}
  </p>
</ConfirmDialog>
