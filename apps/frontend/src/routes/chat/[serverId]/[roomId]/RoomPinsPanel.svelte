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
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import {
    getMentionRoles,
    getRoomMembers,
    type RoomMember,
    type RoomPinsStore
  } from '$lib/state/room';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { getUserSummaryCache } from '$lib/state/userSummaries.svelte';
  import type { UserSummary } from '$lib/api-client/users';
  import type { UserAvatarUserView } from '$lib/render/users';
  import { messagePostedPayload } from '$lib/api-client/roomTimeline';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState, ScrollFader } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { toast } from '$lib/ui/toast';
  import MessageAttachments from './MessageAttachments.svelte';
  import MessageMetaBar from './MessageMetaBar.svelte';
  import MessageReplyAttribution from './MessageReplyAttribution.svelte';
  import { buildMessageReplyPreview, embeddedMessageLinks } from './messageEventModel';

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
  const userSummaries = getUserSummaryCache(serverScope.serverId);
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  const members = $derived(getRoomMembers());
  const mentionRoleHandles = $derived(
    getMentionRoles()
      .filter((role) => role.pingable && role.name !== 'everyone')
      .map((role) => role.name)
  );
  const messageStore = $derived(serverScope.store.messagesForRoom(store.roomId));

  $effect(() => {
    for (const item of store.items) {
      if (item.message?.inReplyTo) void messageStore.ensureEvent(item.message.inReplyTo);
    }
  });

  function user(userId: string): RoomMember | UserSummary | null {
    return members.find((member) => member.id === userId) ?? userSummaries.get(userId);
  }

  function displayName(item: PinnedMessage): string {
    const actor = user(item.message?.actorId ?? '');
    return actor
      ? getLiveDisplayName(actor.id, actor.displayName || actor.login)
      : m('common.unknown');
  }

  function pinActorName(item: PinnedMessage): string {
    const actor = user(item.pinnedByUserId);
    return actor
      ? getLiveDisplayName(actor.id, actor.displayName || actor.login)
      : m('common.unknown');
  }

  function actorView(item: PinnedMessage): UserAvatarUserView | null {
    const actor = user(item.message?.actorId ?? '');
    if (!actor) return null;
    return {
      id: actor.id,
      login: actor.login,
      displayName: actor.displayName,
      deleted: actor.deleted ?? false,
      avatarUrl: actor.avatarUrl,
      presenceStatus: 'presenceStatus' in actor ? actor.presenceStatus : PresenceStatus.OFFLINE,
      customStatus: 'customStatus' in actor ? actor.customStatus : null
    };
  }

  function formatTimestamp(value: PinnedMessage['pinnedAt']): string {
    return value ? formatDateTime(value.toDate().toISOString(), userSettings, activeLocale) : '';
  }

  function openPin(item: PinnedMessage): void {
    if (!item.message) return;
    onOpenPin?.(item.message.id, item.message.threadRootEventId || null);
  }

  function replyPreview(message: NonNullable<PinnedMessage['message']>) {
    if (!message.inReplyTo) return null;
    return buildMessageReplyPreview({
      target: messageStore.getEventById(message.inReplyTo),
      missingName: 'a message',
      deletedName: m('common.deleted_user'),
      getDisplayName: (member) => getLiveDisplayName(member.id, member.displayName || member.login)
    });
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
      <EmptyState icon="icon-[mdi--pin]" title={m('room.pins.empty_title')}>
        {m('room.pins.empty_description')}
      </EmptyState>
    {:else}
      <ol class="selectable-list gap-3 py-2">
        {#each store.items as item (item.message?.id)}
          {@const message = item.message}
          {#if message}
            {@const renderedMessage = messagePostedPayload(message, {})}
            {@const renderedReply = replyPreview(message)}
            {@const messageLinks = embeddedMessageLinks(message.body)}
            <li>
              <div data-room-pin-id={message.id} class="group/pin selectable-list-item">
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
                  {members}
                  roleHandles={mentionRoleHandles}
                >
                  {#snippet prelude()}
                    {#if renderedReply}
                      <MessageReplyAttribution
                        preview={renderedReply}
                        onJump={() =>
                          onOpenPin?.(message.inReplyTo, message.threadRootEventId || null)}
                      />
                    {/if}
                  {/snippet}
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

                    {#each messageLinks as link, i (link.messageId + ':' + i)}
                      <div class="mt-2">
                        <MessagePreviewCard {link} />
                      </div>
                    {/each}

                    {#if renderedMessage.reactions.length > 0 || renderedMessage.threadExists}
                      <MessageMetaBar
                        roomId={message.roomId}
                        serverSegment={serverIdToSegment(serverScope.serverId)}
                        threadRootEventId={message.threadRootEventId || message.id}
                        reactions={renderedMessage.reactions}
                        replyCount={renderedMessage.replyCount}
                        threadExists={renderedMessage.threadExists}
                        threadParticipants={renderedMessage.threadParticipants}
                        onOpenThread={onOpenPin ? () => openPin(item) : undefined}
                      />
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
