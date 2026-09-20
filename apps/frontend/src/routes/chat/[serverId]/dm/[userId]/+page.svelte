<!-- @component Opens a DM after navigation and waits for authoritative room data. -->
<script lang="ts">
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { recentQuickSwitcher } from '$lib/state/recentQuickSwitcher.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import LoadingPage from '$lib/ui/LoadingPage.svelte';
  import Button from '$lib/ui/form/Button.svelte';

  const scope = useServerScope();
  let failed = $state(false);
  let attempt = $state(0);

  // Each recipient or retry owns one request. Cleanup prevents late results
  // from navigating after the user leaves or selects a different recipient.
  $effect(() => {
    const userId = page.params.userId!;
    const serverSegment = page.params.serverId;
    const retry = attempt;
    let cancelled = false;
    const isCurrent = () =>
      !cancelled && scope.isCurrent() && attempt === retry &&
      page.params.userId === userId && page.params.serverId === serverSegment;

    failed = false;
    async function openConversation() {
      try {
        const room = await scope.connection.getAPI(createRoomCommandAPI).startDM(
          userId === scope.store.currentUser.user?.id ? [] : [userId]
        );
        if (!isCurrent()) return;
        if (!room?.id) throw new Error('Conversation is unavailable');
        await scope.store.ensureRoomAvailable(room.id);
        if (!isCurrent()) return;
        const url = resolve('/chat/[serverId]/[roomId]', {
          serverId: serverIdToSegment(scope.serverId),
          roomId: room.id
        });
        await goto(url, { replaceState: true });
        recentQuickSwitcher.record(url);
      } catch {
        if (isCurrent()) failed = true;
      }
    }
    untrack(() => void openConversation());
    return () => { cancelled = true; };
  });
</script>

{#if failed}
  <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.generic')}>
    <Button variant="secondary" onclick={() => attempt++}>{m('common.retry')}</Button>
  </EmptyState>
{:else}
  <LoadingPage />
{/if}
