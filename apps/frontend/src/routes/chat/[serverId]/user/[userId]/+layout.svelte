<!-- @component A user's page, with the viewer's conversation and profile. -->
<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { createRoomCommandAPI } from '@chatto/client/api/rooms';
  import { m } from '$lib/i18n/messages';
  import { userPath } from '$lib/navigation/conversationPath';
  import Room from '../../[roomId]/Room.svelte';
  import { resolveAndRedirect } from '$lib/navigation/resolveMessageDestination';
  import { createRoomTimelineAPI } from '@chatto/client/api/roomTimeline';
  import { serverUi } from '$lib/state/server/serverUi';
  import { recentQuickSwitcher } from '$lib/state/recentQuickSwitcher.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState, LoadingPage, PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  const scope = useServerScope();
  let failed = $state(false);
  let attempt = $state(0);
  let conversation = $state<{ userId: string; roomId: string } | null>(null);
  // Thread navigation replaces page.params without changing the recipient.
  const recipientId = $derived(page.params.userId!);
  const roomId = $derived(
    conversation && conversation.userId === recipientId ? conversation.roomId : null
  );
  const messageLink = $derived(page.route.id === '/chat/[serverId]/user/[userId]/m/[messageId]');
  // The self-conversation check needs the accepted account. The server enforces access.
  const viewerId = $derived(scope.store.accountId);

  // Each recipient or retry owns one request. Cleanup prevents late results
  // from changing the page after the user leaves or selects a different recipient.
  $effect(() => {
    const userId = recipientId;
    const retry = attempt;
    const currentViewerId = viewerId;
    if (!currentViewerId) return;
    let cancelled = false;
    const isCurrent = () =>
      !cancelled && scope.isCurrent() && attempt === retry && recipientId === userId;

    failed = false;
    conversation = null;
    async function openConversation() {
      try {
        const room = await scope.connection
          .getAPI(createRoomCommandAPI)
          .startDM(userId === currentViewerId ? [] : [userId]);
        if (!isCurrent()) return;
        if (!room?.id) throw new Error('Conversation is unavailable');
        await scope.store.ensureRoomAvailable(room.id);
        if (!isCurrent()) return;
        scope.store.loadRoomMembers(room.id);
        conversation = { userId, roomId: room.id };
        recentQuickSwitcher.record(userPath(scope.serverId, userId));
      } catch {
        if (isCurrent()) failed = true;
      }
    }
    untrack(() => void openConversation());
    return () => {
      cancelled = true;
    };
  });

  $effect(() => {
    const id = roomId;
    const userId = page.params.userId!;
    const messageId = page.params.messageId;
    if (!id || !messageLink || !messageId) return;
    let cancelled = false;
    void resolveAndRedirect(
      scope.connection.getAPI(createRoomTimelineAPI),
      serverUi(scope.store).pendingHighlights,
      (threadId) => userPath(scope.serverId, userId, { threadId }),
      id,
      messageId,
      () => !cancelled && scope.isCurrent()
    );
    return () => {
      cancelled = true;
    };
  });
</script>

<PageTitle />

{#if failed}
  <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.generic')}>
    <Button variant="secondary" onclick={() => attempt++}>{m('common.retry')}</Button>
  </EmptyState>
{:else if roomId && !messageLink}
  <Room {roomId} threadId={page.params.threadId} routeMessageId={page.params.messageId} />
{:else}
  <LoadingPage />
{/if}
