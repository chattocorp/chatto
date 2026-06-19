<script lang="ts">
  import { untrack } from 'svelte';
  import { startDMWith } from '$lib/dm/startDM';
  import { resolve } from '$app/paths';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import BanRoomMemberModal from '$lib/components/moderation/BanRoomMemberModal.svelte';
  import BottomSheet from '$lib/ui/BottomSheet.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { useFragment } from '$lib/gql/fragment-masking';
  import { graphql } from '$lib/gql';
  import type { RoomEventViewFragment } from '$lib/gql/graphql';
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
  import { isTouchDevice } from '$lib/utils/isTouchDevice';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatMessageTime } from '$lib/utils/formatTime';
  import { onThreadFollowChanged } from '$lib/eventBus.svelte';
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
  import { shouldHighlightCurrentUserMention } from './messageMentionHighlight';
  import { selectedQuoteTextForMessageBody } from './selectedReplyQuote';

  // Long-press thresholds in milliseconds
  const HIGHLIGHT_DELAY_MS = 150; // Delay before showing visual feedback (avoids flicker on scroll)
  const LONG_PRESS_MS = 500; // Total time before action sheet appears

  let {
    event,
    compact = false,
    roomId,
    messageStore = null,
    onOpenThread
  }: {
    event: RoomEventViewFragment;
    compact?: boolean;
    roomId: string;
    messageStore?: MessagesStore | null;
    onOpenThread?: (
      threadRootEventId: string,
      highlightEventId?: string,
      quoteText?: QuoteInsertionContent
    ) => void;
  } = $props();

  const connection = useConnection();
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const roomPermissions = $derived(getRoomPermissions());
  const composerContext = getComposerContext();
  const replyState = composerContext.replyState;
  const jumpState = composerContext.jumpState;
  const userSettings = getUserSettings();
  const isTouch = isTouchDevice();
  // Wrap in $derived to ensure reactivity when the member list changes
  const members = $derived(getRoomMembers());
  const mentionRoleHandles = $derived(
    getMentionRoles()
      .filter((role) => role.pingable && role.name !== 'everyone')
      .map((role) => role.name)
  );
  // Actor may be null if the user has been deleted.
  // Guard with event?. for Svelte 5 reactivity glitch during virtualizer data transitions.
  const actor = $derived(event?.actor ? useFragment(UserAvatarFragment, event.actor) : null);

  // Display name with live updates from profile cache
  const displayName = $derived(
    actor ? getLiveDisplayName(actor.id, actor.displayName || actor.login) : 'Deleted User'
  );
  const actorCallPresence = $derived(
    actor ? activeCallRooms.getParticipantCallPresence(roomId, actor.id) : null
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

  // Emoji picker state (position doubles as visibility flag; on mobile ContextMenu ignores it)
  let emojiPickerPos = $state<{ x: number; y: number } | null>(null);
  const emojiActions = useMessageActions();

  function openEmojiPicker() {
    // Capture context menu position before it closes
    // On mobile, position is ignored by ContextMenu (renders as BottomSheet)
    emojiPickerPos = contextMenuPos ?? { x: 0, y: 0 };
  }

  function openEmojiPickerFromEvent(e: MouseEvent) {
    emojiPickerPos = { x: e.clientX, y: e.clientY };
  }

  function openEmojiPickerFromToolbar(e: MouseEvent) {
    const button = e.currentTarget as HTMLElement;
    const rect = button.getBoundingClientRect();
    emojiPickerPos = { x: rect.left, y: rect.bottom + 4 };
  }

  async function handleEmojiSelect(emoji: string) {
    emojiPickerPos = null;

    if (!msg) return;

    const params = {
      serverId: getActiveServer(),
      roomId,
      messageEventId: event.id,
      eventId: isEcho ? messageEvent!.echoOfEventId! : event.id,
      messageBody: msg.body ?? ''
    };
    const name = emojiToName(emoji);
    const alreadyReacted = msg.reactions.some((r) => r.emoji === name && r.hasReacted);
    await emojiActions.toggleReaction(params, emoji, alreadyReacted);
  }

  function closeEmojiPicker() {
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

  // Mouse handlers for touch devices that also have mouse input (e.g., tablet with mouse)
  function handleMouseDown(e: MouseEvent) {
    // Skip on desktop - context menu handles mouse interaction
    if (!isTouch) return;
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
  const messageEvent = $derived(
    event?.event?.__typename === 'MessagePostedEvent' ? event.event : null
  );

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

  const timestamp = $derived(event ? formatMessageTime(event.createdAt, userSettings) : '');

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
    await copyMessageLinkToClipboard(getActiveServer(), roomId, event.id);
  }

  // Check if message has been edited (updatedAt is non-null)
  const isEdited = $derived(msg?.updatedAt != null);

  // Threading: check if this is a root message with replies (echoes never have replies)
  // Uses threadRootEventId (thread membership), not inReplyTo (attribution)
  const isRootMessage = $derived(!isEcho && messageEvent?.threadRootEventId == null);
  const hasReplies = $derived(isRootMessage && (messageEvent?.replyCount ?? 0) > 0);

  // Thread follow state — managed as plain $state.
  // Seed once per message identity, then update only via mutations / live events.
  let isFollowingThread = $state(false);
  let _followSeededForEvent = '';

  $effect(() => {
    if (!event) return;
    const id = event.id;
    const value = messageEvent?.viewerIsFollowingThread ?? false;
    if (_followSeededForEvent !== id) {
      _followSeededForEvent = id;
      isFollowingThread = value;
    } else if (value && !isFollowingThread) {
      // Sync auto-follow (false→true only, preserves optimistic unfollows)
      isFollowingThread = true;
    }
  });

  const followThreadMutation = graphql(`
    mutation FollowThread($input: FollowThreadInput!) {
      followThread(input: $input)
    }
  `);

  const unfollowThreadMutation = graphql(`
    mutation UnfollowThread($input: UnfollowThreadInput!) {
      unfollowThread(input: $input)
    }
  `);

  async function toggleThreadFollow(e: MouseEvent) {
    e.stopPropagation();
    const wasFollowing = isFollowingThread;
    isFollowingThread = !wasFollowing;

    const mutation = wasFollowing ? unfollowThreadMutation : followThreadMutation;
    const result = await connection().client.mutation(mutation, {
      input: { roomId, threadRootEventId: event.id }
    });

    if (result.error) {
      isFollowingThread = wasFollowing;
    }
  }

  // Sync thread follow state from live events (auto-follow on reply, cross-tab sync).
  $effect(() =>
    onThreadFollowChanged((update) => {
      if (update.threadRootEventId === event.id) {
        isFollowingThread = update.isFollowing;
      }
    })
  );

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

    if (!replyTarget) return { name: 'a message', body: null as string | null, actor: null };

    const repliedActor = replyTarget.actor
      ? useFragment(UserAvatarFragment, replyTarget.actor)
      : null;
    const name = repliedActor
      ? getLiveDisplayName(repliedActor.id, repliedActor.displayName || repliedActor.login)
      : 'Deleted User';
    const typename = replyTarget.event?.__typename;
    const body = typename === 'MessagePostedEvent' ? (replyTarget.event.body ?? null) : null;
    return { name, body, actor: repliedActor };
  });

  // Check if this thread has pending reply notifications
  const hasThreadNotification = $derived(
    hasReplies && event && notificationStore.hasThreadNotification(event.id)
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

  const BanRoomMemberMutation = graphql(`
    mutation BanRoomMemberFromMessageEvent($input: BanRoomMemberInput!) {
      banRoomMember(input: $input)
    }
  `);

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
    const result = await connection().client.mutation(BanRoomMemberMutation, {
      input: { roomId, userId: member.id, reason, expiresAt }
    });
    banningMemberId = null;

    if (result.error) {
      banError = 'Failed to ban member from room';
      toast.error(banError);
      console.error('Failed to ban member from room:', result.error);
      return;
    }

    toast.success(`Banned ${displayName} from room`);
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
      onOpenThread(messageEvent.echoFromThreadRootEventId, messageEvent.inReplyTo);
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
    replyState.startReply(event.id, displayName, excerpt);
    if (quote) {
      composerContext.quoteInsertionState.requestInsertQuote(quote);
    }
  }

  function handleOpenThread() {
    if (onOpenThread) {
      const quote = takeSelectedReplyQuote();
      // For echoes, use the original thread root event ID (not the echo's wrapper event ID)
      const threadRoot = (isEcho ? messageEvent?.echoFromThreadRootEventId : null) ?? event.id;
      onOpenThread(threadRoot, undefined, quote ?? undefined);
      // Note: Thread notifications are dismissed by ThreadPane's $effect when it mounts,
      // which also handles direct URL navigation to threads.
    }
  }
</script>

{#snippet callPresenceIcon(kind: 'voice' | 'video' | null)}
  {#if kind}
    <span
      class={[
        'iconify shrink-0 text-xs leading-none text-accent',
        kind === 'video' ? 'uil--video' : 'uil--phone'
      ]}
      title={kind === 'video' ? 'In a video call' : 'In a voice call'}
      aria-label={kind === 'video' ? 'In a video call' : 'In a voice call'}
      data-testid={`user-call-presence-${kind}`}
    ></span>
  {/if}
{/snippet}

{#if msg}
  <div
    class={[
      'group relative hover:z-10',
      compact ? (hasVisualEmbed ? 'mt-1.5' : '') : 'mt-4',
      isCurrentUserMentioned ? 'bg-warning/10' : ''
    ]}
    role="article"
    data-event-id={event.id}
  >
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class={[
        'group/msg group/badges relative flex gap-4 px-2 py-1 select-none hover:bg-surface-100 md:mx-2 md:rounded-md md:pr-8',
        compact && msg?.body ? 'items-baseline' : 'items-start',
        longPressActive || showActionSheet || contextMenuPos ? 'bg-surface-100' : ''
      ]}
      ontouchstart={handleTouchStart}
      ontouchend={handleTouchEnd}
      ontouchmove={handleTouchMove}
      ontouchcancel={handleTouchEnd}
      onmousedown={handleMouseDown}
      onmouseup={handleMouseUp}
      onmouseleave={handleMouseLeave}
    >
      <!-- Left column: timestamp (compact) or avatar (full) -->
      {#if compact}
        <div class="flex w-11 shrink-0 items-center justify-center">
          <a
            href={resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
              serverId: serverIdToSegment(getActiveServer()),
              roomId,
              messageId: event.id
            })}
            onclick={copyMessageLink}
            oncontextmenu={(e) => e.stopPropagation()}
            title="Click to copy link to this message"
            class="text-xs whitespace-nowrap text-muted opacity-0 group-hover:opacity-100 hover:underline"
          >
            {timestamp}
          </a>
        </div>
      {:else}
        <!-- Spacer maintains left column width; avatar is absolutely positioned
					 so it doesn't inflate row height for short (single-line) messages -->
        <div class="w-11 shrink-0"></div>
        {#if actor}
          <button
            type="button"
            class="absolute top-1 left-2 cursor-pointer"
            onclick={showPopoverForActor}
            ontouchstart={(e) => e.stopPropagation()}
            oncontextmenu={(e) => {
              e.preventDefault();
              e.stopPropagation();
              showPopoverForActor(e);
            }}
          >
            <UserAvatar
              user={actor}
              size="md"
              showPresence={false}
              class="!h-11 !w-11 shadow-md ring-1 ring-surface-200/30"
            />
          </button>
        {:else}
          <!-- Deleted user placeholder avatar -->
          <div
            class="absolute top-1 left-2 flex h-11 w-11 items-center justify-center rounded-full bg-surface-200 text-muted shadow-md ring-1 ring-surface-200/30"
          >
            <span class="iconify text-xl uil--user-times"></span>
          </div>
        {/if}
      {/if}

      <!-- Message content column -->
      <div class="min-w-0 flex-1 space-y-1">
        <!-- Author, timestamp, and reply attribution -->
        {#if !compact}
          <div class="flex min-w-0 items-center gap-2">
            {#if actor}
              <button
                type="button"
                class="inline-flex shrink-0 cursor-pointer items-center gap-1.5 leading-none font-semibold hover:underline"
                onclick={showPopoverForActor}
                ontouchstart={(e) => e.stopPropagation()}
                oncontextmenu={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  showPopoverForActor(e);
                }}
              >
                <span>{displayName}</span>
                {@render callPresenceIcon(actorCallPresence)}
              </button>
            {:else}
              <strong class="shrink-0 leading-none font-semibold text-muted">{displayName}</strong>
            {/if}
            <a
              href={resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
                serverId: serverIdToSegment(getActiveServer()),
                roomId,
                messageId: event.id
              })}
              onclick={copyMessageLink}
              oncontextmenu={(e) => e.stopPropagation()}
              title="Click to copy link to this message"
              class="shrink-0 text-xs leading-none text-muted hover:underline"
            >
              {timestamp}
            </a>
          </div>
        {/if}

        {#if replyPreview}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            data-testid="reply-attribution"
            class="flex min-w-0 cursor-pointer items-center gap-1 text-xs leading-none text-muted hover:text-text"
            onclick={scrollToReplyTarget}
            onmousedown={(e) => e.stopPropagation()}
          >
            <span class="shrink-0">in reply to</span>
            {#if replyPreview.actor}
              {@const replyCallPresence = activeCallRooms.getParticipantCallPresence(
                roomId,
                replyPreview.actor.id
              )}
              <button
                type="button"
                data-testid="reply-attribution-author"
                class="flex shrink-0 cursor-pointer items-center gap-1 hover:underline"
                onclick={(e) => {
                  e.stopPropagation();
                  showPopoverForReplyAuthor(e);
                }}
              >
                <UserAvatar user={replyPreview.actor} size="xs" showPresence={false} />
                <strong class="font-medium">{replyPreview.name}</strong>
                {@render callPresenceIcon(replyCallPresence)}
              </button>
            {:else}
              <strong class="shrink-0 font-medium">{replyPreview.name}</strong>
            {/if}
            {#if replyPreview.body}
              <span class="min-w-0 truncate opacity-70">{replyPreview.body}</span>
            {/if}
          </div>
        {/if}

        <!-- Message body - re-enable text selection on desktop (pointer-fine variant) -->
        {#if isDeleted}
          <!-- Message deleted or encryption key removed -->
          <span class="text-muted/50 italic">This message has been deleted.</span>
        {:else if msg.body}
          <div bind:this={messageBodySelectionRoot} class="pointer-fine:select-text">
            <MessageContent
              body={msg.body}
              {members}
              roleHandles={mentionRoleHandles}
              edited={isEdited}
              onMentionClick={showPopoverForMember}
            />
          </div>
        {/if}

        <!-- Message attachments -->
        <MessageAttachments
          attachments={msg.attachments ?? []}
          serverId={getActiveServer()}
          {roomId}
          eventId={isEcho ? messageEvent!.echoOfEventId! : event.id}
          canDeleteAttachment={isAuthor}
        />

        <!-- Link preview (only for MessagePostedEvent) -->
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

        <!-- Embedded Chatto message link previews -->
        {#each embeddedMessageLinks as link, i (link.messageId + ':' + i)}
          <div class="mt-2">
            <MessagePreviewCard {link} />
          </div>
        {/each}

        <!-- Thread echo indicator, thread replies, and reactions -->
        {#if (isEcho && onOpenThread) || (hasReplies && onOpenThread) || (msg?.reactions?.length ?? 0) > 0}
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
            {isFollowingThread}
            onToggleThreadFollow={hasReplies ? toggleThreadFollow : undefined}
            onOpenThread={onOpenThread ? handleOpenThread : undefined}
            onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPickerFromEvent : undefined}
            isEchoEvent={isEcho}
          />
        {/if}
      </div>
      <!-- Quick actions toolbar (desktop only — mobile uses long-press action sheet) -->
      {#if !isDeleted && !isTouch}
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
          reactions={msg?.reactions ?? []}
          canReact={roomPermissions.canReact}
          {canEdit}
          forceVisible={!!emojiPickerPos || !!contextMenuPos}
          onReplyInRoom={roomPermissions.canPostMessage ? handleReplyInRoom : undefined}
          onReply={roomPermissions.canPostInThread && onOpenThread ? handleOpenThread : undefined}
          onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPickerFromToolbar : undefined}
          onOpenMenu={openMenuFromToolbar}
        />
      {/if}
    </div>
  </div>

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
        threadRootEventId={editThreadRootEventId}
        channelEchoEventId={editChannelEchoEventId}
        canAddChannelEcho={canReconcileChannelEcho}
        reactions={msg?.reactions ?? []}
        canReact={roomPermissions.canReact}
        {canEdit}
        {canDelete}
        onReplyInRoom={roomPermissions.canPostMessage ? handleReplyInRoom : undefined}
        onReply={roomPermissions.canPostInThread && onOpenThread ? handleOpenThread : undefined}
        onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPicker : undefined}
        onClose={() => (contextMenuPos = null)}
      />
    </ContextMenu>
  {/if}

  <!-- Emoji picker (ContextMenu handles desktop popup vs mobile BottomSheet) -->
  {#if emojiPickerPos && !isDeleted}
    <ContextMenu position={emojiPickerPos} onclose={closeEmojiPicker}>
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
        threadRootEventId={editThreadRootEventId}
        channelEchoEventId={editChannelEchoEventId}
        canAddChannelEcho={canReconcileChannelEcho}
        reactions={msg?.reactions ?? []}
        canReact={roomPermissions.canReact}
        {canEdit}
        {canDelete}
        onReplyInRoom={roomPermissions.canPostMessage ? handleReplyInRoom : undefined}
        onReply={roomPermissions.canPostInThread && onOpenThread ? handleOpenThread : undefined}
        onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPicker : undefined}
        onClose={() => (showActionSheet = false)}
      />
    </BottomSheet>
  {/if}
{/if}
