<script lang="ts">
  import { untrack } from 'svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { resolve } from '$app/paths';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import {
    getRoomMembers,
    getMentionRoles,
    getComposerContext,
    type MessagesStore,
    type RoomMember,
    type QuoteInsertionContent
  } from '$lib/state/room';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';

  const serverScope = useServerScope();
  const stores = serverScope.store;
  const activeCallRooms = $derived(serverUi(stores).activeCallRooms);
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import MessageHoverBar from './MessageHoverBar.svelte';
  import MessageAttachments from './MessageAttachments.svelte';
  import MessageMetaBar from './MessageMetaBar.svelte';
  import { prefersTouchActions, supportsHoverActions } from '$lib/utils/inputCapabilities';
  import { formatMessageTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { copyMessageLinkToClipboard } from '$lib/messageLinks';
  import { serverIdToSegment } from '$lib/navigation';
  import LazyMessagePreviewCard from '$lib/components/LazyMessagePreviewCard.svelte';
  import { shouldHighlightCurrentUserMention } from './messageMentionHighlight';
  import { selectedQuoteTextForMessageBody } from './selectedReplyQuote';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { m } from '$lib/i18n/messages';
  import MessageReplyAttribution from './MessageReplyAttribution.svelte';
  import type {
    MessageActionOverlay,
    MessageActionOverlayState
  } from './messageActionOverlayState.svelte';
  import { MessageActionTarget } from './messageActionTarget.svelte';
  import { MessageLongPressGesture } from './messageLongPress.svelte';
  import {
    buildMessageReplyPreview,
    embeddedMessageLinks,
    isDeletedMessage
  } from './messageEventModel';
  import { ThreadFollowState } from './threadFollowState.svelte';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

  let {
    event,
    compact = false,
    roomId,
    permalinkThreadRootEventId = null,
    messageStore = null,
    actionOverlays,
    onOpenThread,
    onOpenUser,
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    event: TimelineEventView;
    compact?: boolean;
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    messageStore?: MessagesStore | null;
    /** The timeline's message action overlays. The row requests overlays from it. */
    actionOverlays: MessageActionOverlayState;
    onOpenThread?: OpenThreadHandler;
    onOpenUser?: (user: UserAvatarUserView | RoomMember, anchorRect: DOMRect | null) => void;
    threadingMode?: RoomThreadingMode;
  } = $props();

  const connection = () => serverScope.connection;
  const activeServerId = serverScope.serverId;
  const currentUser = $derived({ user: stores.viewerUser });
  const composerContext = getComposerContext();
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

  const target = new MessageActionTarget(() => ({
    event,
    roomId,
    permalinkThreadRootEventId,
    messageStore,
    onOpenThread,
    threadingMode,
    takeReplyQuote: takeSelectedReplyQuote
  }));
  const authorLoading = $derived(!target.actor && event?.actorResolution === 'loading');
  const actorCallPresence = $derived(
    target.actor ? activeCallRooms.getParticipantCallPresence(roomId, target.actor.id) : null
  );

  let messageBodySelectionRoot = $state<HTMLElement>();
  let selectedReplyQuoteSnapshot = $state<QuoteInsertionContent | null>(null);

  // The timeline renders this message's overlays outside the row; see MessageActionOverlays.
  const actionOverlayKind = $derived(actionOverlays.kindFor(event.id));
  const actionMenuOpen = $derived(actionOverlayKind === 'menu' || actionOverlayKind === 'sheet');
  const longPress = new MessageLongPressGesture(() => openActionOverlay({ kind: 'sheet' }));
  $effect(() => () => longPress.dispose());

  // Clear the reply quote and its selection when this message's overlay closes.
  let hadOpenActionOverlay = false;
  $effect(() => {
    const open = actionOverlayKind !== null;
    if (hadOpenActionOverlay && !open) untrack(discardSelectedReplyQuote);
    hadOpenActionOverlay = open;
  });

  function openActionOverlay(
    overlay: MessageActionOverlay,
    pointer: { linkUrl?: string | null; imageUrl?: string | null } = {}
  ) {
    actionOverlays.open(event.id, overlay, {
      ...pointer,
      replyQuote: selectedReplyQuoteSnapshot ?? getSelectedReplyQuote()
    });
    selectedReplyQuoteSnapshot = null;
  }

  // Touch handlers for mobile
  function handleTouchStart() {
    if (actionOverlayKind === 'sheet') return;
    longPress.start();
  }

  function handleTouchEnd() {
    longPress.finish();
  }

  function handleTouchMove() {
    // Cancel long-press if user moves finger (scrolling)
    longPress.cancel();
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
    handleTouchStart();
  }

  function handleMouseUp(event: MouseEvent) {
    if (event.button !== 0) return;
    if (prefersTouch && !canUseHoverActions) {
      longPress.finish();
    }
    if (!(event.target instanceof Element && event.target.closest('[role="toolbar"]'))) {
      selectedReplyQuoteSnapshot = getSelectedReplyQuote();
    }
  }

  function handleMouseLeave() {
    if (prefersTouch && !canUseHoverActions) {
      longPress.cancel();
    }
    if (!actionMenuOpen) {
      selectedReplyQuoteSnapshot = null;
    }
  }

  // Open context menu from the toolbar's "more actions" button,
  // positioned to cover the toolbar exactly.
  function openMenuFromToolbar(e: MouseEvent) {
    const button = e.currentTarget as HTMLElement;
    const toolbar = button.closest('[role="toolbar"]') as HTMLElement | null;
    const rect = toolbar?.getBoundingClientRect() ?? button.getBoundingClientRect();
    openActionOverlay({
      kind: 'menu',
      position: { x: rect.right, y: rect.top, alignRight: true }
    });
  }

  function openEmojiPickerAt(position: { x: number; y: number }) {
    openActionOverlay({ kind: 'emoji', position, presentation: 'auto' });
  }

  function openMenuFromMessage(e: MouseEvent) {
    e.preventDefault();
    // Browsers may synthesize this event during a touch long press, including
    // on hybrid devices; that gesture already owns the action sheet.
    if (longPress.pending || actionOverlayKind === 'sheet') return;
    const mention =
      e.target instanceof Element ? e.target.closest<HTMLElement>('.mention[data-user-id]') : null;
    const mentionedUserId = mention?.dataset.userId;
    if (
      mention &&
      messageBodySelectionRoot?.contains(mention) &&
      mentionedUserId &&
      members.some((member) => member.id === mentionedUserId)
    ) {
      actionOverlays.close({ kind: 'menu', eventId: event.id });
      showPopoverForMember(mentionedUserId, mention.getBoundingClientRect());
      return;
    }
    const anchor = e.target instanceof Element ? e.target.closest('a[href]') : null;
    const imageButton =
      e.target instanceof Element ? e.target.closest('[data-message-image-attachment]') : null;
    const image = imageButton?.querySelector('img');
    openActionOverlay(
      { kind: 'menu', position: { x: e.clientX, y: e.clientY } },
      {
        linkUrl:
          anchor instanceof HTMLAnchorElement && messageBodySelectionRoot?.contains(anchor)
            ? anchor.href
            : null,
        imageUrl: image?.src ? image.currentSrc || image.src : null
      }
    );
  }

  // The posted message: body, attachments, reactions, threading, and reply attribution.
  const msg = $derived(target.messageEvent);

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

  const threadFollow = new ThreadFollowState({
    getConnection: connection,
    getSnapshot: () => ({
      roomId,
      threadRootEventId: event.id,
      following: msg ? (msg.viewerIsFollowingThread ?? false) : null
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
  const hasVisualEmbed = $derived(hasAttachments || !!msg?.linkPreview || messageLinks.length > 0);

  // Message is "deleted" if it has no body AND no attachments.
  // Deleted rows that reach this component have visible context and render as
  // tombstones. EventList omits context-free tombstones before rendering.
  const isDeleted = $derived(msg ? isDeletedMessage(msg) : true);

  const replyTarget = $derived.by(() => {
    const replyToId = msg?.inReplyTo;
    if (!replyToId) return null;
    return messageStore?.getEventById(replyToId);
  });

  // Fetch reply target only when it is outside the already-loaded event window.
  $effect(() => {
    const replyToId = msg?.inReplyTo;
    if (!replyToId) return;
    if (!messageStore) return;
    untrack(() => messageStore.ensureEvent(replyToId));
  });

  // Derive reply preview from locally fetched target
  const replyPreview = $derived.by(() => {
    const replyToId = msg?.inReplyTo;
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
    target.hasReplies && event && serverUi(stores).attention.hasThreadNotification(event.id)
  );
  const hasThreadUnread = $derived(
    target.hasReplies &&
      event &&
      msg?.viewerHasUnreadThread === true &&
      !serverUi(stores).readViews.covers(roomId, event.id)
  );
  const hasMessageFooter = $derived(
    (target.isEcho && !!onOpenThread) ||
      (target.hasThread && !!onOpenThread) ||
      (msg?.reactions?.length ?? 0) > 0 ||
      target.isPinned ||
      ((isEdited || target.isEchoedToChannel) && !isDeleted)
  );

  // Check if current user is mentioned (but not by themselves)
  const isCurrentUserMentioned = $derived(
    shouldHighlightCurrentUserMention({
      actorId: event?.actorId,
      body: msg?.body,
      currentUserId: stores.viewerId,
      currentUserLogin: currentUser.user?.login,
      members
    })
  );

  function showPopoverForActor(e: MouseEvent) {
    showPopoverForUser(target.actor, e);
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
    if (target.isEcho && msg?.inReplyTo && msg.echoFromThreadRootEventId && onOpenThread) {
      onOpenThread(msg.echoFromThreadRootEventId, {
        highlightEventId: msg.inReplyTo
      });
      return;
    }

    const replyToId = msg?.inReplyTo;
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
</script>

{#snippet callPresenceIcon(kind: 'voice' | 'video' | null)}
  {#if kind}
    <span
      class={[
        'iconify shrink-0 text-xs leading-none text-action',
        kind === 'video' ? 'icon-[uil--video]' : 'icon-[uil--phone]'
      ]}
      role="img"
      title={kind === 'video' ? m('room.sidebar.in_video_call') : m('room.sidebar.in_voice_call')}
      aria-label={kind === 'video'
        ? m('room.sidebar.in_video_call')
        : m('room.sidebar.in_voice_call')}
      data-testid={`user-call-presence-${kind}`}
    ></span>
  {/if}
{/snippet}

{#if msg}
  <MessageView
    eventId={event.id}
    actor={target.actor}
    displayName={target.displayName}
    {authorLoading}
    missingActorIsDeleted={target.deletedActor}
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
    rowClass={longPress.active || actionMenuOpen ? 'bg-surface' : ''}
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
        eventId={target.isEcho ? msg!.echoOfEventId! : event.id}
        canDeleteAttachment={target.isAuthor}
        canEditAttachmentDescription={target.canEdit}
      />

      {#if msg?.linkPreview}
        <div class="mt-2">
          <LinkPreviewCard
            preview={msg.linkPreview}
            showDismiss={false}
            canDelete={target.isAuthor}
            serverId={activeServerId}
            {roomId}
            eventId={event.id}
          />
        </div>
      {/if}

      {#each messageLinks as link, i (link.messageId + ':' + i)}
        <div class="mt-2">
          <LazyMessagePreviewCard {link} />
        </div>
      {/each}

      {#if hasMessageFooter}
        <MessageMetaBar
          {roomId}
          serverSegment={serverIdToSegment(activeServerId)}
          threadRootEventId={target.threadRootEventId}
          reactions={msg?.reactions ?? []}
          edited={isEdited && !isDeleted}
          channelEchoEventId={target.isEchoedToChannel && !isDeleted
            ? msg?.channelEchoEventId
            : null}
          action={target.action}
          replyCount={msg?.replyCount}
          threadExists={msg?.threadExists}
          threadParticipants={msg?.threadParticipants}
          {hasThreadNotification}
          {hasThreadUnread}
          isFollowingThread={threadFollow.following}
          isThreadFollowPending={threadFollow.pending}
          onToggleThreadFollow={target.hasThread ? toggleThreadFollow : undefined}
          onOpenThread={onOpenThread ? () => target.openThread() : undefined}
          onOpenEmojiPicker={target.permissions.canReact
            ? (event) => openEmojiPickerAt({ x: event.clientX, y: event.clientY })
            : undefined}
          isEchoEvent={target.isEcho}
        />
      {/if}
    {/snippet}

    {#snippet actions()}
      {#if !isDeleted && canUseHoverActions}
        <MessageHoverBar
          action={target.action}
          forceVisible={actionOverlayKind === 'menu' || actionOverlayKind === 'emoji'}
          onOpenEmojiPicker={target.permissions.canReact
            ? (event) => {
                const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
                openEmojiPickerAt({ x: rect.left, y: rect.bottom + 4 });
              }
            : undefined}
          onOpenMenu={openMenuFromToolbar}
        />
      {/if}
    {/snippet}
  </MessageView>
{/if}
