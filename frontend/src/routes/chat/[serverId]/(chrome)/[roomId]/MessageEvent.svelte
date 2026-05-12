<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- timestamp hrefs use buildMessageLinkPath which already calls resolve() */
  import { startDMWith } from '$lib/dm/startDM';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import BottomSheet from '$lib/ui/BottomSheet.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { useFragment } from '$lib/gql/fragment-masking';
  import { graphql } from '$lib/gql';
  import { RoomEventViewFragmentDoc, type RoomEventViewFragment } from '$lib/gql/graphql';
  import { getRoomPermissions, getRoomMembers, getComposerContext, type RoomMember } from '$lib/state/room';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const stores = serverRegistry.getStore(getActiveServer());
  const notificationStore = stores.notifications;
  const serverInfo = stores.serverInfo;
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { isUserMentioned } from '$lib/mentions';
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
  import { useEvent, useMessageActions } from '$lib/hooks';
  import { emojiToName } from '$lib/emoji';
  import { toast } from '$lib/ui/toast';
  import { buildMessageLinkPath, buildMessageLinkURL, parseMessageLink, type MessageLink } from '$lib/messageLinks';
  import { extractURLs } from '$lib/linkPreview';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';

  // Long-press thresholds in milliseconds
  const HIGHLIGHT_DELAY_MS = 150; // Delay before showing visual feedback (avoids flicker on scroll)
  const LONG_PRESS_MS = 500; // Total time before action sheet appears

  let {
    event,
    compact = false,
    roomId,
    onOpenThread
  }: {
    event: RoomEventViewFragment;
    compact?: boolean;
    roomId: string;
    onOpenThread?: (threadRootEventId: string, highlightEventId?: string) => void;
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
  // Actor may be null if the user has been deleted.
  // Guard with event?. for Svelte 5 reactivity glitch during virtualizer data transitions.
  const actor = $derived(event?.actor ? useFragment(UserAvatarFragment, event.actor) : null);

  // Display name with live updates from profile cache
  const displayName = $derived(
    actor ? getLiveDisplayName(actor.id, actor.displayName || actor.login) : 'Deleted User'
  );

  // Permission checks for message actions
  const isAuthor = $derived(currentUser.user?.id === event?.actorId);
  const canEdit = $derived(
    (isAuthor &&
      roomPermissions.canEditOwnMessage &&
      event && Date.now() - new Date(event.createdAt).getTime() < serverInfo.messageEditWindowSeconds * 1000) ||
      roomPermissions.canEditAnyMessage
  );
  const canDelete = $derived(
    (isAuthor && roomPermissions.canDeleteOwnMessage) || roomPermissions.canDeleteAnyMessage
  );

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

  // Common message data for rendering (body, attachments, reactions, updatedAt)
  const msg = $derived(messageEvent);

  const timestamp = $derived(event ? formatMessageTime(event.createdAt, userSettings) : '');

  // Canonical link for this message (internal path for href, absolute URL for copy).
  const messageLinkPath = $derived(
    event ? buildMessageLinkPath(getActiveServer(), roomId, event.id) : ''
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
    try {
      await navigator.clipboard.writeText(buildMessageLinkURL(getActiveServer(), roomId, event.id));
      toast.success('Message link copied');
    } catch {
      toast.error('Failed to copy link');
    }
  }

  // Check if message has been edited (updatedAt is non-null)
  const isEdited = $derived(msg?.updatedAt != null);

  // Threading: check if this is a root message with replies (echoes never have replies)
  // Uses inThread (thread membership), not inReplyTo (attribution)
  const isRootMessage = $derived(!isEcho && messageEvent?.inThread == null);
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

  // Message is "deleted" if it has no body AND no attachments
  const isDeleted = $derived(!msg?.body && !hasAttachments);

  // A deleted message with no reactions and no thread replies should be hidden entirely
  const hasReactions = $derived((msg?.reactions?.length ?? 0) > 0);
  const isHidden = $derived(isDeleted && !hasReactions && !hasReplies);

  // Reply preview: per-message fetch of the replied-to event.
  let replyTarget = $state<RoomEventViewFragment | null>(null);

  function fetchReplyTarget(eventId: string) {
    connection().client
      .query(
        graphql(`
          query ReplyPreview($roomId: ID!, $eventId: ID!) {
            room(roomId: $roomId) {
              event(eventId: $eventId) {
                ...RoomEventView
              }
            }
          }
        `),
        { roomId, eventId }
      )
      .toPromise()
      .then((result) => {
        const ev = result.data?.room?.event;
        if (ev) {
          const fetched = useFragment(RoomEventViewFragmentDoc, ev);
          if (fetched) {
            replyTarget = fetched;
          }
        }
      });
  }

  // Fetch reply target when inReplyTo is set
  $effect(() => {
    const replyToId = messageEvent?.inReplyTo;
    if (!replyToId) {
      replyTarget = null;
      return;
    }

    // Reset on new reply target
    replyTarget = null;
    fetchReplyTarget(replyToId);
  });

  // Refetch reply target when the replied-to message is edited or deleted
  useEvent((spaceEvent) => {
    const replyToId = messageEvent?.inReplyTo;
    if (!replyToId || !replyTarget) return;

    const evt = spaceEvent.event;
    if (
      (evt?.__typename === 'MessageDeletedEvent' || evt?.__typename === 'MessageUpdatedEvent') &&
      evt.roomId === roomId
    ) {
      // Check if the deleted/updated message is our reply target
      if (replyTarget.id === evt.messageEventId) {
        fetchReplyTarget(replyToId);
      }
    }
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
    const body =
      typename === 'MessagePostedEvent'
        ? (replyTarget.event.body ?? null)
        : null;
    return { name, body, actor: repliedActor };
  });

  // Check if this thread has pending reply notifications
  const hasThreadNotification = $derived(
    hasReplies && event && notificationStore.hasThreadNotification(event.id)
  );

  // Check if current user is mentioned (but not by themselves)
  const isCurrentUserMentioned = $derived(
    currentUser.user?.login &&
      msg?.body &&
      event?.actorId !== currentUser.user.id &&
      isUserMentioned(msg.body, currentUser.user.login, members)
  );

  // User profile popover state
  const serverPerms = getServerPermissions();
  const canWriteDMs = $derived(serverPerms.current.canWriteDMs);
  let popoverUser = $state<RoomMember | null>(null);
  let popoverAnchorRect = $state<DOMRect | null>(null);

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
    if (isEcho && messageEvent?.inReplyTo && messageEvent.echoFromThreadRootEventId && onOpenThread) {
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

  function handleReplyInRoom() {
    const excerpt = (msg?.body ?? '').slice(0, 80);
    replyState.startReply(event.id, displayName, excerpt);
  }

  function handleOpenThread() {
    if (onOpenThread) {
      // For echoes, use the original thread root event ID (not the echo's wrapper event ID)
      const threadRoot = (isEcho ? messageEvent?.echoFromThreadRootEventId : null) ?? event.id;
      onOpenThread(threadRoot);
      // Note: Thread notifications are dismissed by ThreadPane's $effect when it mounts,
      // which also handles direct URL navigation to threads.
    }
  }
</script>

{#if msg && !isHidden}
  <div
    class={[
      'group relative hover:z-10',
      compact ? '' : 'mt-4',
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
            href={messageLinkPath}
            onclick={copyMessageLink}
            oncontextmenu={(e) => e.stopPropagation()}
            title="Click to copy link to this message"
            class="text-xs whitespace-nowrap text-muted opacity-0 hover:underline group-hover:opacity-100"
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
                class="shrink-0 cursor-pointer leading-none font-semibold hover:underline"
                onclick={showPopoverForActor}
                ontouchstart={(e) => e.stopPropagation()}
                oncontextmenu={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  showPopoverForActor(e);
                }}
              >
                {displayName}
              </button>
            {:else}
              <strong class="shrink-0 leading-none font-semibold text-muted">{displayName}</strong>
            {/if}
            <a
              href={messageLinkPath}
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
              </button>
            {:else}
              <div class="skeleton h-5 w-5 shrink-0 rounded-full"></div>
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
          <span class="text-muted/50">[Message deleted]</span>
        {:else if msg.body}
          <div class="pointer-fine:select-text">
            <MessageContent
              body={msg.body}
              {members}
              edited={isEdited}
              onMentionClick={showPopoverForMember}
            />
          </div>
        {/if}

        <!-- Message attachments -->
        <MessageAttachments
          attachments={msg.attachments ?? []}
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
          eventId={isEcho ? messageEvent!.echoOfEventId! : event.id}
          messageBody={msg.body ?? ''}
          reactions={msg?.reactions ?? []}
          canReact={roomPermissions.canReact}
          {canEdit}
          forceVisible={!!emojiPickerPos || !!contextMenuPos}
          onReplyInRoom={(
            onOpenThread
              ? roomPermissions.canPostMessage && roomPermissions.canReply
              : roomPermissions.canReplyInThread
          )
            ? handleReplyInRoom
            : undefined}
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
      canSendMessage={canWriteDMs}
      onSendMessage={() => startDMWith(getActiveServer(), popoverUser!.id)}
      onClose={closePopover}
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
        eventId={isEcho ? messageEvent!.echoOfEventId! : event.id}
        messageBody={msg.body ?? ''}
        reactions={msg?.reactions ?? []}
        canReact={roomPermissions.canReact}
        {canEdit}
        {canDelete}
        onReplyInRoom={(
          onOpenThread
            ? roomPermissions.canPostMessage && roomPermissions.canReply
            : roomPermissions.canReplyInThread
        )
          ? handleReplyInRoom
          : undefined}
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
        eventId={isEcho ? messageEvent!.echoOfEventId! : event.id}
        messageBody={msg.body ?? ''}
        reactions={msg?.reactions ?? []}
        canReact={roomPermissions.canReact}
        {canEdit}
        {canDelete}
        onReplyInRoom={(
          onOpenThread
            ? roomPermissions.canPostMessage && roomPermissions.canReply
            : roomPermissions.canReplyInThread
        )
          ? handleReplyInRoom
          : undefined}
        onReply={roomPermissions.canPostInThread && onOpenThread ? handleOpenThread : undefined}
        onOpenEmojiPicker={roomPermissions.canReact ? openEmojiPicker : undefined}
        onClose={() => (showActionSheet = false)}
      />
    </BottomSheet>
  {/if}
{/if}
