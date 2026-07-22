<script lang="ts">
  import { untrack } from 'svelte';
  import { startDMWith } from '$lib/dm/startDM';
  import { resolve } from '$app/paths';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import UserAvatar, { UserAvatarViewData } from '$lib/components/UserAvatar.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import BanRoomMemberModal from '$lib/components/moderation/BanRoomMemberModal.svelte';
  import BottomSheet from '$lib/ui/BottomSheet.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { useRenderData } from '$lib/render/data';
  import type { RoomEventView } from '$lib/render/types';
  import {
    getRoomPermissions,
    getRoomMembers,
    getMentionRoles,
    getComposerContext,
    type MessagesStore,
    type QuoteInsertionContent,
    type RoomMember
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
  import MessageActionSheet from './MessageActionSheet.svelte';
  import MessageContextMenu from '$lib/components/menus/MessageContextMenu.svelte';
  import MessageHoverBar from './MessageHoverBar.svelte';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import MessageAttachments from './MessageAttachments.svelte';
  import MessageMetaBar from './MessageMetaBar.svelte';
  import { prefersTouchActions, supportsHoverActions } from '$lib/utils/inputCapabilities';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatMessageTime } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useMessageActions } from '$lib/hooks';
  import { emojiToName } from '$lib/emoji';
  import { toast } from '$lib/ui/toast';
  import {
    copyMessageLinkToClipboard,
    parseMessageLink,
    type MessageLink
  } from '$lib/messageLinks';
  import { serverIdToSegment } from '$lib/navigation';
  import { extractURLs } from '$lib/linkPreview';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import DeletedUserLabel from '$lib/components/DeletedUserLabel.svelte';
  import { shouldHighlightCurrentUserMention } from './messageMentionHighlight';
  import { roomReplyTargetEventId } from './messageReplyTarget';
  import { selectedQuoteTextForMessageBody } from './selectedReplyQuote';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { createThreadAPI } from '$lib/api-client/threads';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { isMessagePostedEvent } from '$lib/render/eventKinds';
  import * as m from '$lib/i18n/messages';

  // Long-press thresholds in milliseconds
  const HIGHLIGHT_DELAY_MS = 150; // Delay before showing visual feedback (avoids flicker on scroll)
  const LONG_PRESS_MS = 500; // Total time before action sheet appears

  let {
    event,
    compact = false,
    roomId,
    permalinkThreadRootEventId = null,
    messageStore = null,
    onOpenThread
  }: {
    event: RoomEventView;
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
  const actor = $derived(event?.actor ? useRenderData(UserAvatarViewData, event.actor) : null);
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
    (isAuthor &&
      event &&
      Date.now() - new Date(event.createdAt).getTime() <
        serverInfo.messageEditWindowSeconds * 1000) ||
      roomPermissions.canManageOthersMessage
  );
  const canDelete = $derived(isAuthor || roomPermissions.canManageOthersMessage);

  // Mobile action sheet state
  let showActionSheet = $state(false);
  let longPressActive = $state(false);
  let highlightTimer: ReturnType<typeof setTimeout> | null = null;
  let longPressTimer: ReturnType<typeof setTimeout> | null = null;

  // Desktop context menu state
  let contextMenuPos = $state<{
    x: number;
    y: number;
    alignRight?: boolean;
    centerX?: boolean;
  } | null>(null);
  let messageBodySelectionRoot = $state<HTMLElement>();
  let selectedReplyQuoteSnapshot = $state<QuoteInsertionContent | null>(null);

  // Emoji picker state (position doubles as visibility flag in floating mode)
  let emojiPickerPos = $state<{ x: number; y: number } | null>(null);
  let emojiPickerPresentation = $state<'auto' | 'sheet'>('auto');
  const emojiActions = useMessageActions();

  function openEmojiPicker(presentation: 'auto' | 'sheet' = 'auto') {
    emojiPickerPresentation = presentation;
    // Capture context menu position before it closes. Sheet presentation ignores
    // this fallback, but ContextMenu still requires a visibility anchor.
    emojiPickerPos = contextMenuPos ?? { x: 0, y: 0 };
  }

  function openEmojiPickerFromEvent(e: MouseEvent) {
    emojiPickerPresentation = 'auto';
    emojiPickerPos = { x: e.clientX, y: e.clientY };
  }

  function openEmojiPickerFromToolbar(e: MouseEvent) {
    emojiPickerPresentation = 'auto';
    const button = e.currentTarget as HTMLElement;
    const rect = button.getBoundingClientRect();
    emojiPickerPos = { x: rect.left, y: rect.bottom + 4 };
  }

  async function handleEmojiSelect(emoji: string) {
    emojiPickerPresentation = 'auto';
    emojiPickerPos = null;

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

  function closeEmojiPicker() {
    emojiPickerPresentation = 'auto';
    emojiPickerPos = null;
  }

  function startLongPress() {
    // Stage 1: Show highlight after short delay (avoids flicker during scroll)
    highlightTimer = setTimeout(() => {
      longPressActive = true;
    }, HIGHLIGHT_DELAY_MS);

    // Stage 2: Show action sheet after full long-press duration
    longPressTimer = setTimeout(() => {
      showActionSheet = true;
      longPressActive = false;
    }, LONG_PRESS_MS);
  }

  function cancelLongPress() {
    longPressActive = false;
    if (highlightTimer) {
      clearTimeout(highlightTimer);
      highlightTimer = null;
    }
    if (longPressTimer) {
      clearTimeout(longPressTimer);
      longPressTimer = null;
    }
  }

  // Touch handlers for mobile
  function handleTouchStart() {
    startLongPress();
  }

  function handleTouchEnd() {
    cancelLongPress();
  }

  function handleTouchMove() {
    // Cancel long-press if user moves finger (scrolling)
    cancelLongPress();
  }

  // Mouse fallback for pure touch-primary devices. Hybrid devices with a hover-capable
  // pointer use the normal hover toolbar instead.
  function handleMouseDown(e: MouseEvent) {
    if (!prefersTouch || canUseHoverActions) return;
    // Only handle left mouse button
    if (e.button !== 0) return;
    startLongPress();
  }

  function handleMouseUp() {
    cancelLongPress();
  }

  function handleMouseLeave() {
    cancelLongPress();
  }

  // Open context menu from the toolbar's "more actions" button,
  // positioned to cover the toolbar exactly.
  function openMenuFromToolbar(e: MouseEvent) {
    selectedReplyQuoteSnapshot = getSelectedReplyQuote();
    const button = e.currentTarget as HTMLElement;
    const toolbar = button.closest('[role="toolbar"]') as HTMLElement;
    const rect = toolbar?.getBoundingClientRect() ?? button.getBoundingClientRect();
    contextMenuPos = { x: rect.right, y: rect.top, alignRight: true };
  }

  // MessagePostedEvent-specific data (threading, inReplyTo, etc.)
  // Guard with event?. for Svelte 5 reactivity glitch during virtualizer data transitions
  const messageEvent = $derived(isMessagePostedEvent(event?.event) ? event.event : null);

  // Check if this is an echo (MessagePostedEvent with echoOfEventId set)
  const isEcho = $derived(messageEvent?.echoOfEventId != null);

  const editEventId = $derived(isEcho ? messageEvent!.echoOfEventId! : event.id);
  const editThreadRootEventId = $derived(
    isEcho
      ? (messageEvent?.echoFromThreadRootEventId ?? null)
      : (messageEvent?.threadRootEventId ?? null)
  );
  const editChannelEchoEventId = $derived(
    isEcho ? event.id : (messageEvent?.channelEchoEventId ?? null)
  );
  const threadRootEventId = $derived(
    isEcho ? (messageEvent?.echoFromThreadRootEventId ?? null) : event.id
  );
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
  const embeddedMessageLinks = $derived.by<MessageLink[]>(() => {
    if (!msg?.body) return [];
    return extractURLs(msg.body, 5)
      .map(parseMessageLink)
      .filter((link): link is MessageLink => link !== null);
  });

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
      const conn = connection();
      const api = createThreadAPI({
        serverId: conn.serverId ?? getActiveServer(),
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
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
    hasAttachments || !!messageEvent?.linkPreview || embeddedMessageLinks.length > 0
  );

  // Message is "deleted" if it has no body AND no attachments.
  // Deleted messages always render as a tombstone — hiding them entirely opened up
  // moderation-evading and inconsistency vectors (e.g. event numbering gaps, lost
  // reply-attribution context, deleted-then-reacted-to messages disappearing).
  const isDeleted = $derived(!msg?.body && !hasAttachments);

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

    if (!replyTarget) {
      return { name: 'a message', body: null as string | null, actor: null, deleted: false };
    }

    const repliedActor = replyTarget.actor
      ? useRenderData(UserAvatarViewData, replyTarget.actor)
      : null;
    const activeRepliedActor = repliedActor && !repliedActor.deleted ? repliedActor : null;
    const name = activeRepliedActor
      ? getLiveDisplayName(
          activeRepliedActor.id,
          activeRepliedActor.displayName || activeRepliedActor.login
        )
      : m['common.deleted_user']();
    const body = isMessagePostedEvent(replyTarget.event) ? (replyTarget.event.body ?? null) : null;
    return { name, body, actor: activeRepliedActor, deleted: !activeRepliedActor };
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

  // User profile popover state
  const serverPerms = getServerPermissions();
  const canStartDMs = $derived(serverPerms.current.canStartDMs);
  let popoverUser = $state<RoomMember | null>(null);
  let popoverAnchorRect = $state<DOMRect | null>(null);
  let banningMemberId = $state<string | null>(null);
  let banDialogUser = $state<RoomMember | null>(null);
  let banError = $state<string | null>(null);

  const canBanPopoverUser = $derived.by(() => {
    if (
      !popoverUser ||
      popoverUser.deleted ||
      !roomPermissions.canBanRoomMembers ||
      popoverUser.id === currentUser.user?.id
    ) {
      return false;
    }
    const targetUserId = popoverUser.id;
    return members.some((member) => member.id === targetUserId);
  });

  function openBanDialog(member: RoomMember) {
    if (member.deleted) return;

    banDialogUser = member;
    banError = null;
    closePopover();
  }

  async function banFromRoom(member: RoomMember, reason: string, expiresAt: string | null) {
    if (banningMemberId) return;

    banningMemberId = member.id;
    banError = null;
    const displayName = member.displayName || member.login;
    try {
      const conn = connection();
      const api = createRoomCommandAPI({
        serverId: conn.serverId ?? getActiveServer(),
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      await api.banMember({ roomId, userId: member.id, reason, expiresAt });
    } catch (error) {
      banningMemberId = null;
      banError = m['room.sidebar.ban_failed']();
      toast.error(banError);
      console.error('Failed to ban member from room:', error);
      return;
    }
    banningMemberId = null;

    toast.success(m['room.sidebar.ban_success']({ name: displayName }));
    banDialogUser = null;
  }

  function showPopoverForActor(e: MouseEvent) {
    if (!actor) return;
    // Capture the bounding rect of the clicked button for popover positioning
    const button = (e.target as HTMLElement).closest('button');
    popoverAnchorRect = button?.getBoundingClientRect() ?? null;

    // Look up the full member from the members list (includes live presence)
    const member = members.find((m) => m.id === actor.id);
    if (member) {
      popoverUser = member;
    } else {
      // Actor not in current members list (e.g., left the room) — use actor data directly
      popoverUser = {
        id: actor.id,
        login: actor.login,
        displayName: actor.displayName,
        avatarUrl: actor.avatarUrl,
        presenceStatus: actor.presenceStatus
      };
    }
  }

  function showPopoverForMember(userId: string, anchorRect: DOMRect) {
    const member = members.find((m) => m.id === userId);
    if (member) {
      popoverUser = member;
      popoverAnchorRect = anchorRect;
    }
  }

  function showPopoverForReplyAuthor(e: MouseEvent) {
    const replyActor = replyPreview?.actor;
    if (!replyActor) return;

    const button = (e.target as HTMLElement).closest('button');
    popoverAnchorRect = button?.getBoundingClientRect() ?? null;

    const member = members.find((m) => m.id === replyActor.id);
    if (member) {
      popoverUser = member;
    } else {
      popoverUser = {
        id: replyActor.id,
        login: replyActor.login,
        displayName: replyActor.displayName,
        avatarUrl: replyActor.avatarUrl,
        presenceStatus: replyActor.presenceStatus
      };
    }
  }

  function closePopover() {
    popoverUser = null;
    popoverAnchorRect = null;
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
    rowClass={longPressActive || showActionSheet || contextMenuPos ? 'bg-surface' : ''}
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
        {@const replyJumpText =
          replyPreview.body ??
          (replyPreview.actor || replyPreview.deleted
            ? m['room.message.meta.reply_preview_fallback']()
            : replyPreview.name)}
        {@const replyJumpLabel = `${m['room.message.meta.in_reply_to']()} ${replyJumpText}`}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <div
          data-testid="reply-attribution"
          aria-label={m['room.message.meta.in_reply_to']()}
          title={m['room.message.meta.in_reply_to']()}
          class={[
            'group/reply relative flex min-w-0 cursor-pointer items-center gap-1.5 py-0.5 text-xs leading-none text-muted',
            compact ? '' : '-ml-[39px] pl-[39px]'
          ]}
          onclick={scrollToReplyTarget}
          onmousedown={(e) => e.stopPropagation()}
        >
          {#if compact}
            <span
              aria-hidden="true"
              class="h-3 w-5 shrink-0 rounded-tl-md border-t-2 border-l-2 border-surface-strong/30 transition-colors group-hover/reply:border-surface-strong/55"
            ></span>
          {:else}
            <span
              aria-hidden="true"
              class="absolute top-[11px] left-0 h-7 w-[39px] rounded-tl-md border-t-2 border-l-2 border-surface-strong/30 transition-colors group-hover/reply:border-surface-strong/55"
            ></span>
          {/if}
          {#if replyPreview.actor}
            {@const replyCallPresence = activeCallRooms.getParticipantCallPresence(
              roomId,
              replyPreview.actor.id
            )}
            <button
              type="button"
              data-testid="reply-attribution-author"
              class="inline-flex max-w-[45%] min-w-0 shrink-0 cursor-pointer items-center gap-1 hover:underline"
              onclick={(e) => {
                e.stopPropagation();
                showPopoverForReplyAuthor(e);
              }}
            >
              <UserAvatar user={replyPreview.actor} size="xs" />
              <strong class="truncate font-medium">{replyPreview.name}</strong>
              {@render callPresenceIcon(replyCallPresence)}
            </button>
          {:else if replyPreview.deleted}
            <strong class="max-w-[45%] shrink-0 truncate font-medium"><DeletedUserLabel /></strong>
          {/if}
          <button
            type="button"
            class="min-w-0 flex-1 cursor-pointer truncate text-left opacity-75 hover:text-text"
            aria-label={replyJumpLabel}
            title={replyJumpLabel}
          >
            {replyJumpText}
          </button>
        </div>
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

      {#each embeddedMessageLinks as link, i (link.messageId + ':' + i)}
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
          onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPickerFromEvent : undefined}
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
          forceVisible={!!emojiPickerPos || !!contextMenuPos}
          replyInRoomLabel={replyInRoomActionLabel}
          replyThreadLabel={replyThreadActionLabel}
          onReplyInRoom={canUseReplyAction ? handleReplyInRoom : undefined}
          onReply={canUseThreadAction ? handleOpenThread : undefined}
          onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPickerFromToolbar : undefined}
          onOpenMenu={openMenuFromToolbar}
        />
      {/if}
    {/snippet}
  </MessageView>

  <!-- User profile popover (outside article div to avoid content-visibility containment) -->
  {#if popoverUser && popoverAnchorRect}
    <UserContextMenu
      user={popoverUser}
      anchorRect={popoverAnchorRect}
      canSendMessage={canStartDMs && !popoverUser.deleted}
      canBanFromRoom={canBanPopoverUser}
      banningFromRoom={banningMemberId === popoverUser.id}
      onSendMessage={() => startDMWith(getActiveServer(), popoverUser!.id)}
      onBanFromRoom={() => openBanDialog(popoverUser!)}
      onClose={closePopover}
    />
  {/if}

  {#if banDialogUser}
    <BanRoomMemberModal
      user={banDialogUser}
      submitting={banningMemberId === banDialogUser.id}
      error={banError}
      onconfirm={(reason, expiresAt) => banFromRoom(banDialogUser!, reason, expiresAt)}
      onclose={() => (banDialogUser = null)}
    />
  {/if}

  <!-- Desktop context menu (via toolbar "more actions" button) -->
  {#if contextMenuPos && !isDeleted}
    <ContextMenu
      position={contextMenuPos}
      class="min-w-72"
      onclose={() => {
        contextMenuPos = null;
      }}
    >
      <MessageContextMenu
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
        reactions={msg?.reactions ?? []}
        canReact={roomPermissions.canReact}
        {canEdit}
        {canDelete}
        replyInRoomLabel={replyInRoomActionLabel}
        replyThreadLabel={replyThreadActionLabel}
        onReplyInRoom={canUseReplyAction ? handleReplyInRoom : undefined}
        onReply={canUseThreadAction ? handleOpenThread : undefined}
        onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPicker : undefined}
        onClose={() => (contextMenuPos = null)}
      />
    </ContextMenu>
  {/if}

  <!-- Emoji picker (ContextMenu handles floating vs sheet presentation) -->
  {#if emojiPickerPos && !isDeleted}
    <ContextMenu
      position={emojiPickerPos}
      presentation={emojiPickerPresentation}
      scrollDismissal="user"
      onclose={closeEmojiPicker}
    >
      <EmojiPicker
        serverId={getActiveServer()}
        onSelect={handleEmojiSelect}
        onClose={closeEmojiPicker}
      />
    </ContextMenu>
  {/if}

  <!-- Mobile action sheet (long-press menu, mounted on demand) -->
  {#if showActionSheet && !isDeleted}
    <BottomSheet bind:visible={showActionSheet}>
      <MessageActionSheet
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
        reactions={msg?.reactions ?? []}
        canReact={roomPermissions.canReact}
        {canEdit}
        {canDelete}
        replyInRoomLabel={replyInRoomActionLabel}
        replyThreadLabel={replyThreadActionLabel}
        onReplyInRoom={canUseReplyAction ? handleReplyInRoom : undefined}
        onReply={canUseThreadAction ? handleOpenThread : undefined}
        onOpenEmojiPicker={roomPermissions.canReact ? () => openEmojiPicker('sheet') : undefined}
        onClose={() => (showActionSheet = false)}
      />
    </BottomSheet>
  {/if}
{/if}
