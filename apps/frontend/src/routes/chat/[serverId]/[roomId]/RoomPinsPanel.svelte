<!--
@component

Channel pinned messages rendered through the room timeline's canonical
MessageEvent component. Each message row itself opens the original message.
-->
<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import type { Message } from '@chatto/api-types/api/v1/message_types_pb';
  import type { User } from '@chatto/api-types/api/v1/users_pb';
  import { m } from '$lib/i18n/messages';
  import { messageToTimelineEvent } from '$lib/api-client/roomTimeline';
  import { getRoomMembers, type RoomMember, type RoomPinsStore } from '$lib/state/room';
  import { getUserSummaryCache } from '$lib/state/userSummaries.svelte';
  import type { UserSummary } from '$lib/api-client/users';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState, ScrollFader } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import MessageEvent from './MessageEvent.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';

  let {
    store,
    onOpenPin
  }: {
    store: RoomPinsStore;
    onOpenPin?: (messageEventId: string, threadRootEventId: string | null) => void;
  } = $props();

  const serverScope = useServerScope();
  const userSummaries = getUserSummaryCache(serverScope.serverId);
  const members = $derived(getRoomMembers());
  const messageStore = $derived(serverScope.store.messagesForRoom(store.roomId));

  function user(userId: string): RoomMember | UserSummary | null {
    return members.find((member) => member.id === userId) ?? userSummaries.get(userId);
  }

  function renderableEvent(message: Message) {
    const users: Record<string, User> = {};
    const userIds = new Set([
      message.actorId,
      ...(message.thread?.participantPreviewUserIds ?? []),
      ...message.reactions.flatMap((reaction) => reaction.previewUserIds)
    ]);
    for (const userId of userIds) {
      const summary = user(userId);
      if (!summary) continue;
      users[userId] = {
        id: summary.id,
        login: summary.login,
        displayName: summary.displayName,
        deleted: summary.deleted ?? false,
        avatarUrl: summary.avatarUrl ?? ''
      } as User;
    }
    return messageToTimelineEvent(message, users);
  }

  function openPin(message: Message): void {
    onOpenPin?.(message.id, message.threadRootEventId || null);
  }

  function isInteractiveTarget(target: EventTarget | null, pinTarget: EventTarget | null): boolean {
    if (!(target instanceof Element)) return false;
    const interactive = target.closest(
      'a, button, input, select, textarea, [role="button"], [role="link"]'
    );
    return interactive !== null && interactive !== pinTarget;
  }

  function openPinFromPointer(event: MouseEvent, message: Message): void {
    if (event.defaultPrevented || isInteractiveTarget(event.target, event.currentTarget)) return;
    openPin(message);
  }

  function openPinFromKeyboard(event: KeyboardEvent, message: Message): void {
    if (event.target !== event.currentTarget || event.key !== 'Enter') return;
    event.preventDefault();
    openPin(message);
  }

  const openThread: OpenThreadHandler = (threadRootEventId, options) => {
    onOpenPin?.(options?.highlightEventId ?? threadRootEventId, threadRootEventId);
  };

  const loadMoreWhenVisible: Attachment = (element) => {
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting && !store.loadMoreError) void store.loadMore();
    });
    observer.observe(element);
    return () => observer.disconnect();
  };
</script>

<ScrollFader top bottom keyboardFocusable={false} class="min-h-0 flex-1">
  <div class="flex min-h-full flex-col" aria-live="polite">
    {#if store.error && store.items.length === 0}
      <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('room.pins.error_title')}>
        <p>{m('room.pins.error_description')}</p>
        <div class="mt-4">
          <Button variant="secondary" onclick={() => store.retry()}>{m('common.retry')}</Button>
        </div>
      </EmptyState>
    {:else if store.isInitialLoading && store.items.length === 0}
      <div class="flex min-h-32 flex-1 items-center justify-center p-4 text-sm text-muted">
        <span class="iconify me-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"></span>
        {m('room.pins.loading')}
      </div>
    {:else if store.items.length === 0}
      <EmptyState icon="icon-[mdi--pin-outline]" title={m('room.pins.empty_title')}>
        {m('room.pins.empty_description')}
      </EmptyState>
    {:else}
      <ol class="flex flex-col py-2">
        {#each store.items as item (item.message?.id)}
          {@const message = item.message}
          {@const event = message ? renderableEvent(message) : null}
          {#if message && event}
            <li>
              <div
                role="link"
                tabindex="0"
                aria-label={`${event.actor?.displayName || event.actor?.login || m('common.unknown')}: ${message.body}`}
                data-room-pin-id={message.id}
                class="cursor-pointer rounded-md focus-visible:outline-2 focus-visible:outline-action"
                onclick={(pointerEvent) => openPinFromPointer(pointerEvent, message)}
                onkeydown={(keyboardEvent) => openPinFromKeyboard(keyboardEvent, message)}
              >
                <MessageEvent
                  {event}
                  roomId={store.roomId}
                  {messageStore}
                  onOpenThread={openThread}
                />
              </div>
            </li>
          {/if}
        {/each}
      </ol>
      {#if store.hasMore}
        <div class="flex justify-center py-4" {@attach loadMoreWhenVisible}>
          {#if store.loadMoreError}
            <Button variant="secondary" onclick={() => void store.loadMore()}>
              {m('common.retry')}
            </Button>
          {:else}
            <span class="iconify icon-[uil--spinner-alt] animate-spin text-muted" aria-hidden="true"
            ></span>
          {/if}
        </div>
      {/if}
    {/if}
  </div>
</ScrollFader>
