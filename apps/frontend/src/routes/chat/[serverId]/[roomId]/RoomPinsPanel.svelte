<!--
@component

Channel pinned messages rendered with the same untruncated MessageView used by
the room timeline. Pin metadata and navigation remain sidebar-specific.
-->
<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import type { RoomPinsStore } from '$lib/state/room';
  import type { UserAvatarUserView } from '$lib/render/users';
  import { messagePostedPayload } from '$lib/api-client/roomTimeline';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState, ScrollFader } from '$lib/ui';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { toast } from '$lib/ui/toast';
  import MessageAttachments from './MessageAttachments.svelte';

  let {
    store,
    canManage = false,
    onOpenPin
  }: {
    store: RoomPinsStore;
    canManage?: boolean;
    onOpenPin?: (messageEventId: string, threadRootEventId: string | null) => void;
  } = $props();

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());

  function displayName(item: PinnedMessage): string {
    return item.actor?.displayName || item.actor?.login || m('common.unknown');
  }

  function pinActorName(item: PinnedMessage): string {
    return item.pinnedBy?.displayName || item.pinnedBy?.login || m('common.unknown');
  }

  function actorView(item: PinnedMessage): UserAvatarUserView | null {
    const actor = item.actor;
    if (!actor) return null;
    return {
      id: actor.id,
      login: actor.login,
      displayName: actor.displayName,
      deleted: actor.deleted,
      avatarUrl: actor.avatarUrl,
      presenceStatus: PresenceStatus.OFFLINE,
      customStatus: actor.customStatus
        ? {
            emoji: actor.customStatus.emoji,
            text: actor.customStatus.text,
            expiresAt: actor.customStatus.expiresAt?.toDate().toISOString() ?? null
          }
        : null
    };
  }

  function formatTimestamp(value: PinnedMessage['pinnedAt']): string {
    return value ? formatDateTime(value.toDate().toISOString(), userSettings, activeLocale) : '';
  }

  function openPin(item: PinnedMessage): void {
    if (!item.message) return;
    onOpenPin?.(item.message.id, item.message.threadRootEventId || null);
  }

  async function unpin(messageEventId: string): Promise<void> {
    try {
      await store.remove(messageEventId);
    } catch {
      toast.error(m('room.pins.update_failed'));
    }
  }

  const loadMoreWhenVisible: Attachment = (element) => {
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) void store.loadMore();
    });
    observer.observe(element);
    return () => observer.disconnect();
  };
</script>

<ScrollFader top bottom keyboardFocusable={false} class="min-h-0 flex-1">
  <div class="flex min-h-full flex-col" aria-live="polite">
    {#if store.error && store.items.length === 0}
      <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('room.pins.error_title')}>
        {m('room.pins.error_description')}
      </EmptyState>
    {:else if store.isInitialLoading && store.items.length === 0}
      <div class="flex min-h-32 flex-1 items-center justify-center p-4 text-sm text-muted">
        <span class="iconify me-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"></span>
        {m('room.pins.loading')}
      </div>
    {:else if store.items.length === 0}
      <EmptyState icon="icon-[mdi--pin]" title={m('room.pins.empty_title')}>
        {m('room.pins.empty_description')}
      </EmptyState>
    {:else}
      <ol class="selectable-list gap-3 py-2">
        {#each store.items as item (item.id)}
          {@const message = item.message}
          {#if message}
            {@const renderedMessage = messagePostedPayload(message, {})}
            <li>
              <div data-room-pin-id={item.id} class="group/pin selectable-list-item">
                <MessageView
                  eventId={message.id}
                  actor={actorView(item)}
                  displayName={displayName(item)}
                  missingActorIsDeleted={false}
                  body={message.body}
                  deleted={Boolean(message.deletedAt)}
                  edited={Boolean(message.updatedAt)}
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
                        {formatTimestamp(message.createdAt)}
                      </time>
                    {/if}
                  {/snippet}
                  {#snippet afterBody()}
                    <MessageAttachments
                      attachments={renderedMessage.attachments}
                      serverId={serverScope.serverId}
                      roomId={message.roomId}
                      eventId={message.id}
                      canDeleteAttachment={false}
                    />

                    {#if renderedMessage.linkPreview}
                      <div class="mt-2">
                        <LinkPreviewCard
                          preview={renderedMessage.linkPreview}
                          showDismiss={false}
                          canDelete={false}
                          serverId={serverScope.serverId}
                          roomId={message.roomId}
                          eventId={message.id}
                        />
                      </div>
                    {/if}

                    <div class="mt-2 flex items-start justify-between gap-2 text-xs text-muted">
                      <div class="min-w-0">
                        <div class="flex items-start gap-1">
                          <span class="iconify mt-0.5 icon-[mdi--pin] shrink-0" aria-hidden="true"
                          ></span>
                          <span>
                            {m('room.pins.pinned_by', { name: pinActorName(item) })}
                          </span>
                        </div>
                        {#if item.pinnedAt}
                          <time
                            class="mt-0.5 block ps-5"
                            datetime={item.pinnedAt.toDate().toISOString()}
                          >
                            {formatTimestamp(item.pinnedAt)}
                          </time>
                        {/if}
                      </div>
                      <div class="flex shrink-0 items-center gap-1">
                        <button
                          type="button"
                          class="pane-header-icon-button shrink-0"
                          title={m('room.pins.jump_to_message')}
                          aria-label={m('room.pins.jump_to_message')}
                          onclick={() => openPin(item)}
                        >
                          <span
                            class="iconify icon-[uil--arrow-up-right] pane-header-icon-glyph"
                            aria-hidden="true"
                          ></span>
                        </button>
                        {#if canManage}
                          <button
                            type="button"
                            class="pane-header-icon-button shrink-0"
                            title={m('room.pins.unpin')}
                            aria-label={m('room.pins.unpin')}
                            onclick={() => void unpin(message.id)}
                          >
                            <span
                              class="iconify icon-[uil--times] pane-header-icon-glyph"
                              aria-hidden="true"
                            ></span>
                          </button>
                        {/if}
                      </div>
                    </div>
                  {/snippet}
                </MessageView>
              </div>
            </li>
          {/if}
        {/each}
      </ol>
      {#if store.hasMore}
        <div class="flex justify-center py-4" {@attach loadMoreWhenVisible}>
          <span class="iconify icon-[uil--spinner-alt] animate-spin text-muted" aria-hidden="true"
          ></span>
        </div>
      {/if}
    {/if}
  </div>
</ScrollFader>
