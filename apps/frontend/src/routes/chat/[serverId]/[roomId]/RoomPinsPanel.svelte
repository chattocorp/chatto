<!--
@component

Channel pinned messages rendered through the room timeline's canonical
message presentation. Each message row itself opens the original message.
-->
<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { Attachment } from 'svelte/attachments';
  import type { Message } from '@chatto/api-types/api/v1/message_types_pb';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import type { UserAvatarUserView } from '$lib/render/users';
  import { getRoomMembers, type RoomMember, type RoomPinsStore } from '$lib/state/room';
  import { getUserSummaryCache } from '$lib/state/userSummaries.svelte';
  import type { UserSummary } from '$lib/api-client/users';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { EmptyState, ScrollFader } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import ClampedMessagePreview from './ClampedMessagePreview.svelte';

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
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());

  function user(userId: string): RoomMember | UserSummary | null {
    return members.find((member) => member.id === userId) ?? userSummaries.get(userId);
  }

  function messageActor(message: Message): UserAvatarUserView | null {
    const summary = user(message.actorId);
    if (!summary) return null;
    return {
      id: summary.id,
      login: summary.login,
      displayName: summary.displayName,
      deleted: summary.deleted ?? false,
      isBot: summary.isBot,
      avatarUrl: summary.avatarUrl,
      presenceStatus: PresenceStatus.OFFLINE
    };
  }

  function formatTimestamp(message: Message): string {
    const createdAt = message.createdAt?.toDate().toISOString() ?? '';
    return createdAt ? formatDateTime(createdAt, userSettings, activeLocale) : '';
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
      <ol class="selectable-list gap-3 py-2">
        {#each store.items as item (item.message?.id)}
          {@const message = item.message}
          {@const actor = message ? messageActor(message) : null}
          {#if message}
            <li>
              <div
                role="link"
                tabindex="0"
                aria-label={`${actor?.displayName || actor?.login || m('common.unknown')}: ${message.body || ''}`}
                data-room-pin-id={message.id}
                class="group/search-result cursor-pointer selectable-list-item"
                onclick={(pointerEvent) => openPinFromPointer(pointerEvent, message)}
                onkeydown={(keyboardEvent) => openPinFromKeyboard(keyboardEvent, message)}
              >
                <div class="pointer-events-none" inert data-room-pin-preview>
                  <ClampedMessagePreview>
                    <MessageView
                      eventId={message.id}
                      {actor}
                      displayName={actor?.displayName || actor?.login || m('common.unknown')}
                      missingActorIsDeleted={false}
                      body={message.body || null}
                      viewerLogin={serverScope.store.currentUser.user?.login}
                      timestampSettings={userSettings}
                      timestampLocale={activeLocale}
                      rowClass="hover:bg-transparent md:mx-0 md:pe-2"
                    >
                      {#snippet headerMeta()}
                        {#if message.createdAt}
                          <time
                            class="text-xs text-muted"
                            datetime={message.createdAt.toDate().toISOString()}
                          >
                            {formatTimestamp(message)}
                          </time>
                        {/if}
                      {/snippet}

                      {#snippet afterBody()}
                        {#if message.attachments.length > 0}
                          <p class="inline-flex items-center gap-1 text-xs text-muted">
                            <span class="iconify icon-[uil--paperclip]" aria-hidden="true"></span>
                            {m('search.attachments', { count: message.attachments.length })}
                          </p>
                        {/if}
                      {/snippet}
                    </MessageView>
                  </ClampedMessagePreview>
                </div>
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
