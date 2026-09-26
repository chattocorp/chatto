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
    type RoomMember,
    type QuoteInsertionContent
  } from '$lib/state/room';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { UserAvatarUserView } from '$lib/render/users';
  import { mapDirectoryMember } from '$lib/api-client/directoryMemberView';
  import { avatarUserFromDirectoryMember } from '$lib/state/server/rooms.svelte';

  const serverScope = useServerScope();
  const stores = $derived(serverScope.store);
  const notificationStore = $derived(stores.notifications);
  const serverInfo = $derived(stores.serverInfo);
  const activeCallRooms = $derived(stores.activeCallRooms);
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import MessageHoverBar from './MessageHoverBar.svelte';
  import MessageAttachments from './MessageAttachments.svelte';
  import MessageMetaBar from './MessageMetaBar.svelte';
  import { prefersTouchActions, supportsHoverActions } from '$lib/utils/inputCapabilities';
  import { formatMessageTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useMessageActions } from '$lib/hooks';
  import { toast } from '$lib/ui/toast';
  import { copyMessageLinkToClipboard } from '$lib/messageLinks';
  import { serverIdToSegment } from '$lib/navigation';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import { shouldHighlightCurrentUserMention } from './messageMentionHighlight';
  import { roomReplyTargetEventId } from './messageReplyTarget';
  import { selectedQuoteTextForMessageBody } from './selectedReplyQuote';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { isMessagePostedEvent } from '$lib/render/timelineEvents';
  import { m } from '$lib/i18n/messages';
  import MessageReplyAttribution from './MessageReplyAttribution.svelte';
  import MessageEventActionOverlays from './MessageEventActionOverlays.svelte';
  import { MessageEventInteractionState } from './messageEventInteractions.svelte';
  import {
    buildMessageReplyPreview,
    canEditMessage,
    embeddedMessageLinks,
    isDeletedMessage,
    resolveMessageEventReferences
  } from './messageEventModel';
  import { ThreadFollowState } from './threadFollowState.svelte';
  import { buildMessageActionModel } from './messageActionModel';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    event,
    compact = false,
    roomId,
    permalinkThreadRootEventId = null,
    messageStore = null,
    onOpenThread,
    onOpenUser,
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    event: TimelineEventView;
    compact?: boolean;
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    messageStore?: MessagesStore | null;
    onOpenThread?: OpenThreadHandler;
    onOpenUser?: (user: UserAvatarUserView | RoomMember, anchorRect: DOMRect | null) => void;
    threadingMode?: RoomThreadingMode;
  } = $props();

  const connection = () => serverScope.connection;
  const activeServerId = $derived(serverScope.serverId);
  const currentUser = $derived({ user: stores.viewerUser });
  const roomPermissions = $derived(getRoomPermissions());
  const composerContext = getComposerContext();
  const replyState = composerContext.replyState;
  const jumpState = composerContext.jumpState;
  const userSettings = $derived(timeFormatSettingsFor(currentUser.user?.settings));
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
  // Resolve every row against the live profile owner. Timeline includes can
  // arrive before profiles catch up after a reconnect.
  const actorId = $derived(event?.actorId || event?.actor?.id || '');
  const users = $derived(stores.projection.users);
  const deletedActor = $derived(
    event?.actorResolution === 'deleted' ||
      !!event?.actor?.deleted ||
      (!!actorId && users.isDeleted(actorId))
  );
  const actor = $derived.by(() => {
    if (deletedActor) return null;
    const current = actorId ? users.get(actorId) : undefined;
    return current
      ? avatarUserFromDirectoryMember(mapDirectoryMember(current))
      : (event?.actor ?? null);
  });
  const authorLoading = $derived(!actor && event?.actorResolution === 'loading');

  // The actor already uses the live profile when one is available.
  const displayName = $derived(
    actor
      ? actor.displayName || actor.login
      : deletedActor
        ? m('common.deleted_user')
        : m('common.unknown_user')
  );
  const actorCallPresence = $derived(
    actor ? activeCallRooms.getParticipantCallPresence(roomId, actor.id) : null
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
  const canEditAttachmentDescription = $derived(
    canEdit && serverInfo.supportsFeature('attachmentDescriptions')
  );

  const interactions = new MessageEventInteractionState();
  $effect(() => () => interactions.dispose());
  let messageBodySelectionRoot = $state<HTMLElement>();
  let selectedReplyQuoteSnapshot = $state<QuoteInsertionContent | null>(null);
  let contextLink = $state<{ eventId: string; url: string } | null>(null);
  let contextImage = $state<{ eventId: string; url: string } | null>(null);
  const contextLinkUrl = $derived(contextLink?.eventId === event?.id ? contextLink.url : null);
  const contextImageUrl = $derived(contextImage?.eventId === event?.id ? contextImage.url : null);
  // Virtualized rows can receive another message while their menu is still open.
  $effect(() => {
    if (contextLink && contextLink.eventId !== event?.id) contextLink = null;
    if (contextImage && contextImage.eventId !== event?.id) contextImage = null;
  });

  const messageActions = useMessageActions();

  // Touch handlers for mobile
  function handleTouchStart() {
    interactions.startLongPress();
  }

  function handleTouchEnd() {
    interactions.finishLongPress();
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
      interactions.finishLongPress();
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
    contextLink = null;
    contextImage = null;
    selectedReplyQuoteSnapshot ??= getSelectedReplyQuote();
    interactions.openContextMenuFromToolbar(e);
  }

  function openMenuFromMessage(e: MouseEvent) {
    e.preventDefault();
    // Browsers may synthesize this event during a touch long press, including
    // on hybrid devices; that gesture already owns the action sheet.
    if (interactions.hasActiveLongPressGesture) return;
    const mention =
      e.target instanceof Element ? e.target.closest<HTMLElement>('.mention[data-user-id]') : null;
    const mentionedUserId = mention?.dataset.userId;
    if (
      mention &&
      messageBodySelectionRoot?.contains(mention) &&
      mentionedUserId &&
      members.some((member) => member.id === mentionedUserId)
    ) {
      interactions.closeContextMenu();
      contextLink = null;
      contextImage = null;
      showPopoverForMember(mentionedUserId, mention.getBoundingClientRect());
      return;
    }
    const anchor = e.target instanceof Element ? e.target.closest('a[href]') : null;
    contextLink =
      anchor instanceof HTMLAnchorElement && messageBodySelectionRoot?.contains(anchor)
        ? { eventId: event.id, url: anchor.href }
        : null;
    const imageButton =
      e.target instanceof Element ? e.target.closest('[data-message-image-attachment]') : null;
    const image = imageButton?.querySelector('img');
    contextImage = image?.src ? { eventId: event.id, url: image.currentSrc || image.src } : null;
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
  const pinsStore = $derived(
    roomPermissions.canViewPinnedMessages ? stores.pinsForRoom(roomId) : null
  );
  const canPin = $derived(roomPermissions.canPinMessages && Boolean(pinsStore));
  const isPinned = $derived(
    pinsStore?.isPinned(editEventId, messageEvent?.pinned ?? false) ?? messageEvent?.pinned ?? false
  );
  const canReconcileChannelEcho = $derived(
    isAuthor &&
      !!editThreadRootEventId &&
      (!!editChannelEchoEventId ||
        (threadingMode !== RoomThreadingMode.DISABLED &&
          roomPermissions.canEchoMessage &&
          roomPermissions.canPostMessage))
  );

  // Common message data for rendering (body, attachments, reactions, updatedAt)
  const msg = $derived(messageEvent);

  const timestamp = $derived(
    event ? formatMessageTime(event.createdAt, userSettings, activeLocale) : ''
  );

  // Message links referenced in this message's body — rendered inline as previews.
  const messageBody = $derived(msg?.body);
  const messageLinks = $derived(embeddedMessageLinks(messageBody));

  async function copyMessageLink(e: MouseEvent) {
    if (!event) return;
    e.preventDefault();
    e.stopPropagation();
    await copyMessageLinkToClipboard(activeServerId, roomId, event.id, permalinkThreadRootEventId);
  }

  const isEdited = $derived(msg?.updatedAt != null);

  // Threading: check if this is a root message with replies (echoes never have replies)
  // Uses threadRootEventId (thread membership), not inReplyTo (attribution)
  const isRootMessage = $derived(!isEcho && messageEvent?.threadRootEventId == null);
  const hasReplies = $derived(isRootMessage && (messageEvent?.replyCount ?? 0) > 0);
  const hasThread = $derived(
    isRootMessage && ((messageEvent?.threadExists ?? false) || (messageEvent?.replyCount ?? 0) > 0)
  );
  const isInThreadPane = $derived(!!permalinkThreadRootEventId);
  const canReplyInThread = $derived(
    messageEvent?.canReplyInThread ?? roomPermissions.canPostInThread
  );
  const isEchoedToChannel = $derived(
    isInThreadPane && !isEcho && !!messageEvent?.channelEchoEventId
  );
  const replyInRoomActionLabel = $derived(
    isEcho ? m('room.message.actions.reply_thread') : m('room.message.actions.reply')
  );
  const replyThreadActionLabel = $derived(
    isEcho || (isRootMessage && threadingMode === RoomThreadingMode.DISABLED && hasThread)
      ? m('room.message.actions.open_thread')
      : m('room.message.actions.reply_thread')
  );
  const canUseReplyAction = $derived.by(() => {
    if (threadingMode === RoomThreadingMode.DISABLED && isInThreadPane) return false;
    if (isEcho) {
      return (
        threadingMode !== RoomThreadingMode.DISABLED &&
        canReplyInThread &&
        !!onOpenThread &&
        !!messageEvent?.echoFromThreadRootEventId
      );
    }
    if (isInThreadPane) return canReplyInThread;
    if (isRootMessage && threadingMode === RoomThreadingMode.REQUIRED) {
      return canReplyInThread && !!onOpenThread;
    }
    if (isRootMessage && threadingMode === RoomThreadingMode.ENCOURAGED) {
      return (canReplyInThread && !!onOpenThread) || roomPermissions.canPostMessage;
    }
    return roomPermissions.canPostMessage;
  });
  const canUseSecondaryRoomReply = $derived(
    isRootMessage &&
      !isInThreadPane &&
      threadingMode === RoomThreadingMode.ENCOURAGED &&
      canReplyInThread &&
      !!onOpenThread &&
      roomPermissions.canPostMessage
  );
  const canUseThreadAction = $derived(
    isEcho
      ? !!onOpenThread && !!messageEvent?.echoFromThreadRootEventId
      : threadingMode === RoomThreadingMode.DISABLED
        ? !permalinkThreadRootEventId && isRootMessage && hasThread && !!onOpenThread
        : canReplyInThread && !!onOpenThread
  );
  const actionModel = $derived(
    buildMessageActionModel({
      actions: messageActions,
      params: {
        serverId: activeServerId,
        roomId,
        messageEventId: event.id,
        eventId: editEventId,
        deleteEventId: event.id,
        messageBody: msg?.body ?? '',
        permalinkThreadRootEventId,
        threadRootEventId: editThreadRootEventId,
        channelEchoEventId: editChannelEchoEventId,
        canAddChannelEcho: canReconcileChannelEcho,
        messageStore
      },
      reactions: msg?.reactions ?? [],
      canReact: roomPermissions.canReact,
      canEdit,
      canDelete,
      canPin,
      isPinned,
      togglePin: async () => {
        const pins = pinsStore;
        if (!pins) return;
        try {
          if (pins.isPinned(editEventId, messageEvent?.pinned ?? false))
            await pins.remove(editEventId);
          else await pins.create(editEventId);
        } catch {
          toast.error(m('room.pins.update_failed'));
        }
      },
      replyInRoomLabel: replyInRoomActionLabel,
      replyThreadLabel: replyThreadActionLabel,
      replyInRoom: canUseReplyAction ? handleReply : undefined,
      replyThread: canUseThreadAction
        ? isEcho || threadingMode === RoomThreadingMode.DISABLED
          ? handleOpenThread
          : handleReplyInThread
        : undefined,
      secondaryReplyInRoomLabel: canUseSecondaryRoomReply
        ? m('room.message.actions.reply_room')
        : undefined,
      secondaryReplyInRoom: canUseSecondaryRoomReply ? handleReplyInCurrentComposer : undefined
    })
  );

  const threadFollow = new ThreadFollowState({
    getConnection: connection,
    getSnapshot: () => ({
      roomId,
      threadRootEventId: event.id,
      following: messageEvent ? (messageEvent.viewerIsFollowingThread ?? false) : null
    }),
    beginOptimistic: ({ threadRootEventId }, following) =>
      messageStore?.beginOptimisticThreadFollow(threadRootEventId, following),
    commit: ({ threadRootEventId }, following) =>
      messageStore?.setThreadRootFollowState(threadRootEventId, following)
  });

  function toggleThreadFollow(e: MouseEvent) {
    e.stopPropagation();
    void threadFollow.toggle();
  }

  const hasAttachments = $derived((msg?.attachments?.length ?? 0) > 0);
  const hasVisualEmbed = $derived(
    hasAttachments || !!messageEvent?.linkPreview || messageLinks.length > 0
  );

  // Message is "deleted" if it has no body AND no attachments.
  // Deleted rows that reach this component have visible context and render as
  // tombstones. EventList omits context-free tombstones before rendering.
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
      deletedName: m('common.deleted_user'),
      getDisplayName: (member) => getLiveDisplayName(member.id, member.displayName || member.login)
    });
  });

  // Check if this thread has pending reply notifications
  const hasThreadNotification = $derived(
    hasReplies && event && notificationStore.hasThreadNotification(event.id)
  );
  const hasThreadUnread = $derived(
    hasReplies &&
      event &&
      messageEvent?.viewerHasUnreadThread === true &&
      !stores.readViews.covers(roomId, event.id)
  );
  const hasMessageFooter = $derived(
    (isEcho && !!onOpenThread) ||
      (hasThread && !!onOpenThread) ||
      (msg?.reactions?.length ?? 0) > 0 ||
      isPinned ||
      ((isEdited || isEchoedToChannel) && !isDeleted)
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

  function showPopoverForActor(e: MouseEvent) {
    showPopoverForUser(actor, e);
  }

  function showPopoverForMember(userId: string, anchorRect: DOMRect) {
    const member = members.find((candidate) => candidate.id === userId);
    if (member) onOpenUser?.(member, anchorRect);
  }

  function showPopoverForReplyAuthor(e: MouseEvent) {
    showPopoverForUser(replyPreview?.actor ?? null, e);
  }

  function showPopoverForUser(user: UserAvatarUserView | RoomMember | null, event: MouseEvent) {
    if (!user) return;
    const button = (event.target as HTMLElement).closest('button');
    onOpenUser?.(user, button?.getBoundingClientRect() ?? null);
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
    // Every ConversationPane provides this context.
    jumpState.jumpToMessage(replyToId);
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

  function handleReply() {
    const quote = takeSelectedReplyQuote();
    const excerpt = (msg?.body ?? '').slice(0, 80);
    if (isEcho && messageEvent?.echoOfEventId && messageEvent.echoFromThreadRootEventId) {
      onOpenThread?.(messageEvent.echoFromThreadRootEventId, {
        highlightEventId: messageEvent.echoOfEventId,
        quoteText: quote ?? undefined,
        reply: {
          eventId: messageEvent.echoOfEventId,
          actorDisplayName: displayName,
          actorIdentity: actor ?? undefined,
          excerpt
        }
      });
      return;
    }

    if (isInThreadPane) {
      startReplyInCurrentComposer(quote);
      return;
    }

    if (
      isRootMessage &&
      (threadingMode === RoomThreadingMode.REQUIRED ||
        (threadingMode === RoomThreadingMode.ENCOURAGED && canReplyInThread && !!onOpenThread))
    ) {
      startReplyInThread(quote);
      return;
    }

    startReplyInCurrentComposer(quote);
  }

  function handleReplyInCurrentComposer() {
    startReplyInCurrentComposer(takeSelectedReplyQuote());
  }

  function startReplyInCurrentComposer(quote: QuoteInsertionContent | null) {
    const excerpt = (msg?.body ?? '').slice(0, 80);
    replyState.startReply(roomReplyTargetEventId(event), displayName, excerpt, actor ?? undefined);
    if (quote) {
      composerContext.quoteInsertionState.requestInsertQuote(quote);
    }
  }

  function handleReplyInThread() {
    startReplyInThread(takeSelectedReplyQuote());
  }

  function startReplyInThread(quote: QuoteInsertionContent | null) {
    onOpenThread?.(permalinkThreadRootEventId ?? event.id, {
      quoteText: quote ?? undefined,
      reply: {
        eventId: roomReplyTargetEventId(event),
        actorDisplayName: displayName,
        actorIdentity: actor ?? undefined,
        excerpt: (msg?.body ?? '').slice(0, 80)
      }
    });
  }

  function handleOpenThread() {
    if (onOpenThread) {
      // For echoes, use the original thread root event ID (not the echo's wrapper event ID)
      const threadRoot =
        (isEcho ? messageEvent?.echoFromThreadRootEventId : null) ??
        permalinkThreadRootEventId ??
        event.id;
      selectedReplyQuoteSnapshot = null;
      onOpenThread(threadRoot);
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
        kind === 'video' ? 'icon-[uil--video]' : 'icon-[uil--phone]'
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
    {authorLoading}
    missingActorIsDeleted={deletedActor}
    body={msg.body}
    deleted={isDeleted}
    viewerLogin={currentUser.user?.login}
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
          serverId: serverIdToSegment(activeServerId),
          roomId,
          messageId: event.id
        })}
        onclick={copyMessageLink}
        oncontextmenu={(e) => e.stopPropagation()}
        title={m('room.message.meta.copy_link_title')}
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
          serverId: serverIdToSegment(activeServerId),
          roomId,
          messageId: event.id
        })}
        onclick={copyMessageLink}
        oncontextmenu={(e) => e.stopPropagation()}
        title={m('room.message.meta.copy_link_title')}
        class="shrink-0 text-xs leading-none text-muted hover:underline"
      >
        {timestamp}
      </a>
    {/snippet}

    {#snippet afterBody()}
      <MessageAttachments
        attachments={msg.attachments ?? []}
        serverId={activeServerId}
        {roomId}
        eventId={isEcho ? messageEvent!.echoOfEventId! : event.id}
        canDeleteAttachment={isAuthor}
        {canEditAttachmentDescription}
      />

      {#if messageEvent?.linkPreview}
        <div class="mt-2">
          <LinkPreviewCard
            preview={messageEvent.linkPreview}
            showDismiss={false}
            canDelete={isAuthor}
            serverId={activeServerId}
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
          serverSegment={serverIdToSegment(activeServerId)}
          {threadRootEventId}
          reactions={msg?.reactions ?? []}
          edited={isEdited && !isDeleted}
          channelEchoEventId={isEchoedToChannel && !isDeleted
            ? messageEvent?.channelEchoEventId
            : null}
          action={actionModel}
          replyCount={messageEvent?.replyCount}
          threadExists={messageEvent?.threadExists}
          threadParticipants={messageEvent?.threadParticipants}
          {hasThreadNotification}
          {hasThreadUnread}
          isFollowingThread={threadFollow.following}
          isThreadFollowPending={threadFollow.pending}
          onToggleThreadFollow={hasThread ? toggleThreadFollow : undefined}
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
          action={actionModel}
          forceVisible={interactions.forceHoverActionsVisible}
          onOpenEmojiPicker={roomPermissions.canReact
            ? (event) => interactions.openEmojiPickerFromToolbar(event)
            : undefined}
          onOpenMenu={openMenuFromToolbar}
        />
      {/if}
    {/snippet}
  </MessageView>

  {#if !isDeleted}
    <MessageEventActionOverlays
      {interactions}
      action={actionModel}
      {roomId}
      messageEventId={event.id}
      reactions={msg?.reactions ?? []}
      linkUrl={contextLinkUrl}
      imageUrl={contextImageUrl}
      onClose={() => {
        contextLink = null;
        contextImage = null;
        discardSelectedReplyQuote();
      }}
    />
  {/if}
{/if}
