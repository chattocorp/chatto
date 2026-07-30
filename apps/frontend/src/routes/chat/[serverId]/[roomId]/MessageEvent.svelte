<script lang="ts">
  import { untrack } from 'svelte';
  import { resolve } from '$app/paths';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import type { TimelineEventView } from '$lib/render/timelineEvents';
  import {
    getRoomPermissions,
    getRoomMembers,
    getMentionRoles,
    getComposerContext,
    type MessagesStore,
    type QuoteInsertionContent
  } from '$lib/state/room';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const stores = serverRegistry.getStore(getActiveServer());
  const notificationStore = stores.notifications;
  const serverInfo = stores.serverInfo;
  const activeCallRooms = stores.activeCallRooms;
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import MessageHoverBar from './MessageHoverBar.svelte';
  import MessageAttachments from './MessageAttachments.svelte';
  import MessageMetaBar from './MessageMetaBar.svelte';
  import { prefersTouchActions, supportsHoverActions } from '$lib/utils/inputCapabilities';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatMessageTime } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useMessageActions } from '$lib/hooks';
  import { emojiToName } from '$lib/emoji';
  import { toast } from '$lib/ui/toast';
  import { copyMessageLinkToClipboard } from '$lib/messageLinks';
  import { serverIdToSegment } from '$lib/navigation';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import { shouldHighlightCurrentUserMention } from './messageMentionHighlight';
  import { roomReplyTargetEventId } from './messageReplyTarget';
  import { selectedQuoteTextForMessageBody } from './selectedReplyQuote';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { createThreadAPI } from '$lib/api-client/threads';
  import { isMessagePostedEvent } from '$lib/render/timelineEvents';
  import * as m from '$lib/i18n/messages';
  import MessageReplyAttribution from './MessageReplyAttribution.svelte';
  import MessageEventActionOverlays from './MessageEventActionOverlays.svelte';
  import { MessageEventInteractionState } from './messageEventInteractions.svelte';
  import MessageUserOverlays from './MessageUserOverlays.svelte';
  import { MessageUserInteractionState } from './messageUserInteractions.svelte';
  import {
    buildMessageReplyPreview,
    canEditMessage,
    embeddedMessageLinks,
    isDeletedMessage,
    resolveMessageEventReferences
  } from './messageEventModel';

  let {
    event,
    compact = false,
    roomId,
    permalinkThreadRootEventId = null,
    messageStore = null,
    onOpenThread
  }: {
    event: TimelineEventView;
    compact?: boolean;
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    messageStore?: MessagesStore | null;
    onOpenThread?: OpenThreadHandler;
  } = $props();

  const connection = useConnection();
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const roomPermissions = $derived(getRoomPermissions());
  const composerContext = getComposerContext();
  const replyState = composerContext.replyState;
  const jumpState = composerContext.jumpState;
  const userSettings = getUserSettings();
  const activeLocale = $derived(getLocale());
  const prefersTouch = prefersTouchActions();
  const canUseHoverActions = supportsHoverActions();
  // Wrap in $derived to ensure reactivity when the member list changes
  const members = $derived(getRoomMembers());
  const mentionRoleHandles = $derived(
    getMentionRoles()
      .filter((role) => role.pingable && role.name !== 'everyone')
      .map((role) => role.name)
  );
  // Deleted actors may be absent or retained as a deleted reference.
  // Guard with event?. for Svelte 5 reactivity glitch during virtualizer data transitions.
  const actor = $derived(event?.actor ?? null);
  const deletedActor = $derived(!actor || actor.deleted);

  // Display name with live updates from profile cache
  const displayName = $derived(
    !deletedActor && actor
      ? getLiveDisplayName(actor.id, actor.displayName || actor.login)
      : m['common.deleted_user']()
  );
  const actorCallPresence = $derived(
    !deletedActor && actor ? activeCallRooms.getParticipantCallPresence(roomId, actor.id) : null
  );

  // Permission checks for message actions. Authors can always edit (within
  // the edit window) and delete their own messages; managing other users'
  // messages requires message.manage.
  const isAuthor = $derived(currentUser.user?.id === event?.actorId);
  const canEdit = $derived(
    canEditMessage({
      isAuthor,
      createdAt: event.createdAt,
      now: Date.now(),
      editWindowSeconds: serverInfo.messageEditWindowSeconds,
      canManageOthersMessage: roomPermissions.canManageOthersMessage
    })
  );
  const canDelete = $derived(isAuthor || roomPermissions.canManageOthersMessage);

  const interactions = new MessageEventInteractionState();
  $effect(() => () => interactions.dispose());
  const userInteractions = new MessageUserInteractionState(() => members);
  let messageBodySelectionRoot = $state<HTMLElement>();
  let selectedReplyQuoteSnapshot = $state<QuoteInsertionContent | null>(null);

  const emojiActions = useMessageActions();

  async function handleEmojiSelect(emoji: string) {
    if (!msg) return;

    const params = {
      serverId: getActiveServer(),
      roomId,
      messageEventId: event.id,
      eventId: isEcho ? messageEvent!.echoOfEventId! : event.id,
      messageBody: msg.body ?? '',
      messageStore
    };
    const name = emojiToName(emoji);
    const alreadyReacted = msg.reactions.some((r) => r.emoji === name && r.hasReacted);
    await emojiActions.toggleReaction(params, emoji, alreadyReacted);
  }

  // Touch handlers for mobile
  function handleTouchStart() {
    interactions.startLongPress();
  }

  function handleTouchEnd() {
    interactions.cancelLongPress();
  }

  function handleTouchMove() {
    // Cancel long-press if user moves finger (scrolling)
    interactions.cancelLongPress();
  }

  // Mouse fallback for pure touch-primary devices. Hybrid devices with a hover-capable
  // pointer use the normal hover toolbar instead.
  function handleMouseDown(e: MouseEvent) {
    // Capture selected quote text before a right-click can collapse the browser selection.
    if (e.button === 2) {
      selectedReplyQuoteSnapshot ??= getSelectedReplyQuote();
      return;
    }
    if (!prefersTouch || canUseHoverActions) return;
    // Only handle left mouse button
    if (e.button !== 0) return;
    interactions.startLongPress();
  }

  function handleMouseUp(event: MouseEvent) {
    if (event.button !== 0) return;
    if (prefersTouch && !canUseHoverActions) {
      interactions.cancelLongPress();
    }
    if (!(event.target instanceof Element && event.target.closest('[role="toolbar"]'))) {
      selectedReplyQuoteSnapshot = getSelectedReplyQuote();
    }
  }

  function handleMouseLeave() {
    if (prefersTouch && !canUseHoverActions) {
      interactions.cancelLongPress();
    }
    if (!interactions.hasOpenActionSurface) {
      selectedReplyQuoteSnapshot = null;
    }
  }

  // Open context menu from the toolbar's "more actions" button,
  // positioned to cover the toolbar exactly.
  function openMenuFromToolbar(e: MouseEvent) {
    selectedReplyQuoteSnapshot ??= getSelectedReplyQuote();
    interactions.openContextMenuFromToolbar(e);
  }

  function openMenuFromMessage(e: MouseEvent) {
    e.preventDefault();
    // Browsers may synthesize this event during a touch long press, including
    // on hybrid devices; that gesture already owns the action sheet.
    if (interactions.hasActiveLongPressGesture) return;
    selectedReplyQuoteSnapshot ??= getSelectedReplyQuote();
    interactions.openContextMenuAtPointer(e);
  }

  // MessagePostedEvent-specific data (threading, inReplyTo, etc.)
  // Guard with event?. for Svelte 5 reactivity glitch during virtualizer data transitions
  const messageEvent = $derived(isMessagePostedEvent(event?.event) ? event.event : null);

  const eventReferences = $derived(
    messageEvent ? resolveMessageEventReferences(event.id, messageEvent) : null
  );
  const isEcho = $derived(eventReferences?.isEcho ?? false);
  const editEventId = $derived(eventReferences?.editEventId ?? event.id);
  const editThreadRootEventId = $derived(eventReferences?.editThreadRootEventId ?? null);
  const editChannelEchoEventId = $derived(eventReferences?.editChannelEchoEventId ?? null);
  const threadRootEventId = $derived(eventReferences?.threadRootEventId ?? null);
  const canReconcileChannelEcho = $derived(
    isAuthor &&
      !!editThreadRootEventId &&
      (!!editChannelEchoEventId ||
        (roomPermissions.canEchoMessage && roomPermissions.canPostMessage))
  );

  // Common message data for rendering (body, attachments, reactions, updatedAt)
  const msg = $derived(messageEvent);

  const timestamp = $derived(
    event ? formatMessageTime(event.createdAt, userSettings, activeLocale) : ''
  );

  // Message links referenced in this message's body — rendered inline as previews.
  const messageLinks = $derived(embeddedMessageLinks(msg?.body));

  async function copyMessageLink(e: MouseEvent) {
    if (!event) return;
    e.preventDefault();
    e.stopPropagation();
    await copyMessageLinkToClipboard(
      getActiveServer(),
      roomId,
      event.id,
      permalinkThreadRootEventId
    );
  }

  // Check if message has been edited (updatedAt is non-null)
  const isEdited = $derived(msg?.updatedAt != null);

  // Threading: check if this is a root message with replies (echoes never have replies)
  // Uses threadRootEventId (thread membership), not inReplyTo (attribution)
  const isRootMessage = $derived(!isEcho && messageEvent?.threadRootEventId == null);
  const hasReplies = $derived(isRootMessage && (messageEvent?.replyCount ?? 0) > 0);
  const replyInRoomActionLabel = $derived(
    isEcho ? m['room.message.actions.reply_thread']() : m['room.message.actions.reply']()
  );
  const replyThreadActionLabel = $derived(
    isEcho ? m['room.message.actions.open_thread']() : m['room.message.actions.reply_thread']()
  );
  const canUseReplyAction = $derived(
    isEcho
      ? roomPermissions.canPostInThread &&
          !!onOpenThread &&
          !!messageEvent?.echoFromThreadRootEventId
      : roomPermissions.canPostMessage
  );
  const canUseThreadAction = $derived(
    isEcho
      ? !!onOpenThread && !!messageEvent?.echoFromThreadRootEventId
      : roomPermissions.canPostInThread && !!onOpenThread
  );

  // Overridable derived state: backing event data is the default, while
  // mutations/live events can update the row immediately.
  let isFollowingThread = $derived(messageEvent?.viewerIsFollowingThread ?? false);
  let threadFollowRequestId = 0;
  let isThreadFollowPending = $state(false);

  function setThreadFollowState(value: boolean) {
    if (!event) return;
    threadFollowRequestId += 1;
    isThreadFollowPending = false;
    isFollowingThread = value;
    messageStore?.setThreadRootFollowState(event.id, value);
  }

  async function toggleThreadFollow(e: MouseEvent) {
    e.stopPropagation();
    if (!event || isThreadFollowPending) return;

    const wasFollowing = isFollowingThread;
    const nextFollowing = !wasFollowing;
    const requestId = ++threadFollowRequestId;
    const optimistic = messageStore?.beginOptimisticThreadFollow(event.id, nextFollowing);

    isThreadFollowPending = true;
    isFollowingThread = nextFollowing;

    try {
      const api = connection().getAPI(createThreadAPI);
      const input = { roomId, threadRootEventId: event.id };
      const result = wasFollowing ? await api.unfollowThread(input) : await api.followThread(input);
      if (threadFollowRequestId !== requestId) return;
      setThreadFollowState(result.following);
    } catch {
      if (threadFollowRequestId !== requestId) return;
      isThreadFollowPending = false;
      isFollowingThread = wasFollowing;
      optimistic?.rollback();
    }
  }

  // Check if message has attachments
  const hasAttachments = $derived((msg?.attachments?.length ?? 0) > 0);
  const hasVisualEmbed = $derived(
    hasAttachments || !!messageEvent?.linkPreview || messageLinks.length > 0
  );

  // Message is "deleted" if it has no body AND no attachments.
  // Deleted messages always render as a tombstone — hiding them entirely opened up
  // moderation-evading and inconsistency vectors (e.g. event numbering gaps, lost
  // reply-attribution context, deleted-then-reacted-to messages disappearing).
  const isDeleted = $derived(msg ? isDeletedMessage(msg) : true);

  const replyTarget = $derived.by(() => {
    const replyToId = messageEvent?.inReplyTo;
    if (!replyToId) return null;
    return messageStore?.getEventById(replyToId);
  });

  // Fetch reply target only when it is outside the already-loaded event window.
  $effect(() => {
    const replyToId = messageEvent?.inReplyTo;
    if (!replyToId) return;
    if (!messageStore) return;
    untrack(() => messageStore.ensureEvent(replyToId));
  });

  // Derive reply preview from locally fetched target
  const replyPreview = $derived.by(() => {
    const replyToId = messageEvent?.inReplyTo;
    if (!replyToId) return null;

    return buildMessageReplyPreview({
      target: replyTarget,
      missingName: 'a message',
      deletedName: m['common.deleted_user'](),
      getDisplayName: (member) => getLiveDisplayName(member.id, member.displayName || member.login)
    });
  });

  // Check if this thread has pending reply notifications
  const hasThreadNotification = $derived(
    hasReplies && event && notificationStore.hasThreadNotification(event.id)
  );
  const hasMessageFooter = $derived(
    (isEcho && !!onOpenThread) ||
      (hasReplies && !!onOpenThread) ||
      (msg?.reactions?.length ?? 0) > 0
  );

  // Check if current user is mentioned (but not by themselves)
  const isCurrentUserMentioned = $derived(
    shouldHighlightCurrentUserMention({
      actorId: event?.actorId,
      body: msg?.body,
      currentUserId: currentUser.user?.id,
      currentUserLogin: currentUser.user?.login,
      members
    })
  );

  const serverPerms = getServerPermissions();
  const canStartDMs = $derived(serverPerms.current.canStartDMs);

  function showPopoverForActor(e: MouseEvent) {
    userInteractions.showUserFromEvent(actor, e);
  }

  function showPopoverForMember(userId: string, anchorRect: DOMRect) {
    userInteractions.showMember(userId, anchorRect);
  }

  function showPopoverForReplyAuthor(e: MouseEvent) {
    userInteractions.showUserFromEvent(replyPreview?.actor ?? null, e);
  }

  function scrollToReplyTarget() {
    // For echo events, open the thread and highlight the replied-to message there
    if (
      isEcho &&
      messageEvent?.inReplyTo &&
      messageEvent.echoFromThreadRootEventId &&
      onOpenThread
    ) {
      onOpenThread(messageEvent.echoFromThreadRootEventId, {
        highlightEventId: messageEvent.inReplyTo
      });
      return;
    }

    const replyToId = messageEvent?.inReplyTo;
    if (!replyToId) return;

    // Use jump-to-message state which works with the virtualizer.
    // Both Room (main view) and ThreadPane provide this context.
    if (jumpState) {
      jumpState.jumpToMessage(replyToId);
    } else {
      toast.info('Message is not loaded. Scroll up to find it.');
    }
  }

  function getSelectedReplyQuote(): QuoteInsertionContent | null {
    return selectedQuoteTextForMessageBody(
      typeof window === 'undefined' ? null : window.getSelection(),
      messageBodySelectionRoot
    );
  }

  function takeSelectedReplyQuote(): QuoteInsertionContent | null {
    const quote = selectedReplyQuoteSnapshot ?? getSelectedReplyQuote();
    selectedReplyQuoteSnapshot = null;
    return quote;
  }

  function discardSelectedReplyQuote(): void {
    selectedReplyQuoteSnapshot = null;
    if (getSelectedReplyQuote()) {
      window.getSelection()?.removeAllRanges();
    }
  }

  function handleReplyInRoom() {
    const quote = takeSelectedReplyQuote();
    const excerpt = (msg?.body ?? '').slice(0, 80);
    if (isEcho && messageEvent?.echoOfEventId && messageEvent.echoFromThreadRootEventId) {
      onOpenThread?.(messageEvent.echoFromThreadRootEventId, {
        highlightEventId: messageEvent.echoOfEventId,
        quoteText: quote ?? undefined,
        reply: {
          eventId: messageEvent.echoOfEventId,
          actorDisplayName: displayName,
          excerpt
        }
      });
      return;
    }
    replyState.startReply(roomReplyTargetEventId(event), displayName, excerpt);
    if (quote) {
      composerContext.quoteInsertionState.requestInsertQuote(quote);
    }
  }

  function handleOpenThread() {
    if (onOpenThread) {
      // For echoes, use the original thread root event ID (not the echo's wrapper event ID)
      const threadRoot = (isEcho ? messageEvent?.echoFromThreadRootEventId : null) ?? event.id;
      if (isEcho) {
        selectedReplyQuoteSnapshot = null;
        onOpenThread(threadRoot);
        return;
      }
      const quote = takeSelectedReplyQuote();
      onOpenThread(threadRoot, { quoteText: quote ?? undefined });
      // Note: Thread notifications are dismissed by ThreadPane's $effect when it mounts,
      // which also handles direct URL navigation to threads.
    }
  }
</script>

{#snippet callPresenceIcon(kind: 'voice' | 'video' | null)}
  {#if kind}
    <span
      class={[
        'iconify shrink-0 text-xs leading-none text-action',
        kind === 'video' ? 'uil--video' : 'uil--phone'
      ]}
      title={kind === 'video' ? 'In a video call' : 'In a voice call'}
      aria-label={kind === 'video' ? 'In a video call' : 'In a voice call'}
      data-testid={`user-call-presence-${kind}`}
    ></span>
  {/if}
{/snippet}

{#if msg}
  <MessageView
    eventId={event.id}
    {actor}
    {displayName}
    body={msg.body}
    deleted={isDeleted}
    edited={isEdited}
    {compact}
    avatarOffset={!!replyPreview}
    hasFooter={hasMessageFooter}
    class={[
      compact ? (hasVisualEmbed ? 'mt-1.5' : '') : 'mt-4',
      isCurrentUserMentioned ? 'bg-warning/10' : ''
    ]}
    rowClass={interactions.longPressActive || interactions.hasOpenActionSurface ? 'bg-surface' : ''}
    {members}
    roleHandles={mentionRoleHandles}
    timestampSettings={userSettings}
    timestampLocale={activeLocale}
    onMentionClick={showPopoverForMember}
    onActorClick={showPopoverForActor}
    onActorTouchStart={(e) => e.stopPropagation()}
    onActorContextMenu={(e) => {
      e.preventDefault();
      e.stopPropagation();
      showPopoverForActor(e);
    }}
    ontouchstart={handleTouchStart}
    ontouchend={handleTouchEnd}
    ontouchmove={handleTouchMove}
    ontouchcancel={handleTouchEnd}
    oncontextmenu={isDeleted ? undefined : openMenuFromMessage}
    onmousedown={handleMouseDown}
    onmouseup={handleMouseUp}
    onmouseleave={handleMouseLeave}
    bind:bodyElement={messageBodySelectionRoot}
  >
    {#snippet compactLeading()}
      <a
        href={resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
          serverId: serverIdToSegment(getActiveServer()),
          roomId,
          messageId: event.id
        })}
        onclick={copyMessageLink}
        oncontextmenu={(e) => e.stopPropagation()}
        title={m['room.message.meta.copy_link_title']()}
        class="text-xs whitespace-nowrap text-muted opacity-0 group-hover:opacity-100 hover:underline"
      >
        {timestamp}
      </a>
    {/snippet}

    {#snippet prelude()}
      {#if replyPreview}
        <MessageReplyAttribution
          preview={replyPreview}
          {compact}
          callPresence={replyPreview.actor
            ? activeCallRooms.getParticipantCallPresence(roomId, replyPreview.actor.id)
            : null}
          onJump={scrollToReplyTarget}
          onAuthorClick={showPopoverForReplyAuthor}
        />
      {/if}
    {/snippet}

    {#snippet authorSuffix()}
      {@render callPresenceIcon(actorCallPresence)}
    {/snippet}

    {#snippet headerMeta()}
      <a
        href={resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
          serverId: serverIdToSegment(getActiveServer()),
          roomId,
          messageId: event.id
        })}
        onclick={copyMessageLink}
        oncontextmenu={(e) => e.stopPropagation()}
        title={m['room.message.meta.copy_link_title']()}
        class="shrink-0 text-xs leading-none text-muted hover:underline"
      >
        {timestamp}
      </a>
    {/snippet}

    {#snippet afterBody()}
      <MessageAttachments
        attachments={msg.attachments ?? []}
        serverId={getActiveServer()}
        {roomId}
        eventId={isEcho ? messageEvent!.echoOfEventId! : event.id}
        canDeleteAttachment={isAuthor}
      />

      {#if messageEvent?.linkPreview}
        <div class="mt-2">
          <LinkPreviewCard
            preview={messageEvent.linkPreview}
            showDismiss={false}
            canDelete={isAuthor}
            {roomId}
            eventId={event.id}
          />
        </div>
      {/if}

      {#each messageLinks as link, i (link.messageId + ':' + i)}
        <div class="mt-2">
          <MessagePreviewCard {link} />
        </div>
      {/each}

      {#if hasMessageFooter}
        <MessageMetaBar
          {roomId}
          messageEventId={event.id}
          serverSegment={serverIdToSegment(getActiveServer())}
          {threadRootEventId}
          reactions={msg?.reactions ?? []}
          replyCount={messageEvent?.replyCount}
          threadParticipants={messageEvent?.threadParticipants}
          {hasThreadNotification}
          canReact={roomPermissions.canReact}
          {messageStore}
          {isFollowingThread}
          {isThreadFollowPending}
          onToggleThreadFollow={hasReplies ? toggleThreadFollow : undefined}
          onOpenThread={onOpenThread ? handleOpenThread : undefined}
          onOpenEmojiPicker={roomPermissions.canReact
            ? (event) => interactions.openEmojiPickerFromEvent(event)
            : undefined}
          isEchoEvent={isEcho}
        />
      {/if}
    {/snippet}

    {#snippet actions()}
      {#if !isDeleted && canUseHoverActions}
        <MessageHoverBar
          serverId={getActiveServer()}
          {roomId}
          messageEventId={event.id}
          eventId={editEventId}
          deleteEventId={event.id}
          messageBody={msg.body ?? ''}
          threadRootEventId={editThreadRootEventId}
          channelEchoEventId={editChannelEchoEventId}
          canAddChannelEcho={canReconcileChannelEcho}
          {messageStore}
          reactions={msg?.reactions ?? []}
          canReact={roomPermissions.canReact}
          {canEdit}
          forceVisible={interactions.forceHoverActionsVisible}
          replyInRoomLabel={replyInRoomActionLabel}
          replyThreadLabel={replyThreadActionLabel}
          onReplyInRoom={canUseReplyAction ? handleReplyInRoom : undefined}
          onReply={canUseThreadAction ? handleOpenThread : undefined}
          onOpenEmojiPicker={roomPermissions.canReact
            ? (event) => interactions.openEmojiPickerFromToolbar(event)
            : undefined}
          onOpenMenu={openMenuFromToolbar}
        />
      {/if}
    {/snippet}
  </MessageView>

  <MessageUserOverlays
    interactions={userInteractions}
    serverId={getActiveServer()}
    {roomId}
    currentUserId={currentUser.user?.id}
    {canStartDMs}
    canBanRoomMembers={roomPermissions.canBanRoomMembers}
  />

  {#if !isDeleted}
    <MessageEventActionOverlays
      {interactions}
      serverId={getActiveServer()}
      {roomId}
      messageEventId={event.id}
      eventId={editEventId}
      deleteEventId={event.id}
      messageBody={msg.body ?? ''}
      {permalinkThreadRootEventId}
      threadRootEventId={editThreadRootEventId}
      channelEchoEventId={editChannelEchoEventId}
      canAddChannelEcho={canReconcileChannelEcho}
      {messageStore}
      reactions={msg.reactions}
      canReact={roomPermissions.canReact}
      {canEdit}
      {canDelete}
      replyInRoomLabel={replyInRoomActionLabel}
      replyThreadLabel={replyThreadActionLabel}
      onReplyInRoom={canUseReplyAction ? handleReplyInRoom : undefined}
      onReply={canUseThreadAction ? handleOpenThread : undefined}
      onEmojiSelect={handleEmojiSelect}
      onClose={discardSelectedReplyQuote}
    />
  {/if}
{/if}
