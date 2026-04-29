<script lang="ts">
  import { goto, pushState, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import VoiceCallButton from '$lib/components/voice/VoiceCallButton.svelte';
  import VoiceCallPanel from '$lib/components/voice/VoiceCallPanel.svelte';
  import { useRoomData, useRoomMembersSync, useRoomUnread, useSpaceEvent, createTypingIndicator } from '$lib/hooks';
  import { appState, sidebarNav } from '$lib/state/globals.svelte';
  import { createComposerContext, getRoomMembers, createRoomPermissions, DEFAULT_ROOM_PERMISSIONS } from '$lib/state/room';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { clearLastRoom, setLastRoom } from '$lib/storage/lastRoom';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { tick } from 'svelte';
  import RoomEventsPane from './RoomEventsPane.svelte';
  import RoomInfo from './RoomInfo.svelte';
  import ThreadPane from './ThreadPane.svelte';

  let { spaceId, roomId, threadId }: { spaceId: string; roomId: string; threadId?: string } =
    $props();

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const stores = instanceRegistry.getStore(getInstanceId());
  const instanceState = stores.instance;
  const notificationStore = stores.notifications;

  // Thread navigation functions (URL-driven state)
  let pendingThreadHighlight = $state<string | null>(null);

  function openThread(threadRootEventId: string, highlightEventId?: string) {
    pendingThreadHighlight = highlightEventId ?? null;
    goto(resolve('/chat/[instanceId]/[spaceId]/[roomId]/[threadId]', { instanceId: instanceSegment, spaceId, roomId, threadId: threadRootEventId }));
  }

  function closeThread() {
    goto(resolve('/chat/[instanceId]/[spaceId]/[roomId]', { instanceId: instanceSegment, spaceId, roomId }));
  }

  // Create context-based state (must be synchronous, before children render)
  const composerContext = createComposerContext({ scroll: true });
  const replyState = composerContext.replyState;
  const jumpState = composerContext.jumpState;
  const currentUser = getCurrentUser();

  // --- Extracted hooks ---
  const room = useRoomData(() => ({ spaceId, roomId }));

  useRoomMembersSync(() => ({
    spaceId,
    roomId,
    isDM: room.isDM,
    roomData: room.roomData,
    dmData: room.dmData
  }));

  const unread = useRoomUnread(() => ({ spaceId, roomId }));

  // Room permissions — derived reactively, no $effect needed
  const DM_PERMISSIONS = {
    canPostMessage: true,
    canPostInThread: false,
    canReply: true,
    canReplyInThread: false,
    canReact: true,
    canEditOwnMessage: true,
    canEditAnyMessage: false,
    canDeleteOwnMessage: true,
    canDeleteAnyMessage: false
  } as const;

  let permissions = $derived.by(() => {
    if (room.isDM && room.dmData) return DM_PERMISSIONS;
    if (room.roomData) return room.roomData;
    return DEFAULT_ROOM_PERMISSIONS;
  });

  createRoomPermissions(() => permissions);

  // roomData === null means the server returned a clean response with no room
  // (deleted, archived, no access). Transient network failures are filtered
  // upstream in useRoomData, so reaching this branch is genuine — clear
  // lastRoom so [spaceId]/+page.svelte's onMount doesn't redirect us right
  // back here in an infinite loop.
  $effect.pre(() => {
    if (room.roomData === null) {
      clearLastRoom(getInstanceId(), spaceId);

      if (room.isDM) {
        goto(resolve('/chat/dm'), { replaceState: true });
      } else {
        goto(resolve('/chat/[instanceId]/[spaceId]', { instanceId: instanceSegment, spaceId }), { replaceState: true });
      }
    }
  });

  // Get display title for room header
  let title = $derived.by(() => {
    if (!room.roomData) return '';

    if (!room.isDM) {
      return `# ${room.roomData.room.name}`;
    }

    if (!room.dmData || room.dmData.participants.length === 0) {
      return 'Direct Message';
    }

    const others = room.dmData.participants.filter((p) => p.id !== room.dmData!.currentUserId);
    if (others.length === 0) {
      const self = room.dmData.participants.find((p) => p.id === room.dmData!.currentUserId);
      return self?.displayName || self?.login || 'You';
    }
    return others.map((p) => getLiveDisplayName(p.id, p.displayName || p.login)).join(', ');
  });

  // Page title includes space name for regular rooms
  let pageTitle = $derived.by(() => {
    if (!room.roomData) return '';
    if (!room.isDM && room.roomData.spaceName) {
      return `#${room.roomData.room.name} - ${room.roomData.spaceName}`;
    }
    return title;
  });

  // Dismiss notifications when entering the room
  $effect(() => {
    if (!appState.isFocused) return;

    const currentRoomId = roomId;
    if (room.isDM) {
      notificationStore.dismissDMNotifications(currentRoomId);
    } else {
      notificationStore.dismissMentionNotifications(currentRoomId);
      notificationStore.dismissRoomReplyNotifications(currentRoomId);
    }
  });

  // Remember this room as the last visited for this space
  $effect(() => {
    if (room.roomData) {
      setLastRoom(getInstanceId(), spaceId, roomId);
    }
  });

  // Handle ?highlight=eventId query param (from notification clicks / message links)
  $effect(() => {
    const highlightEventId = page.url.searchParams.get('highlight');
    if (!highlightEventId || !room.roomData) return;

    // Room.svelte lives in +layout and is reused across roomId changes. During
    // in-place navigation between rooms the URL can update before useRoomData
    // resets roomData, so we'd otherwise fire jumpToMessage against the
    // previous room's store and strip ?highlight before the new room loads.
    if (room.roomData.room.id !== roomId) return;

    const cleanUrl = new URL(page.url);
    cleanUrl.searchParams.delete('highlight');
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- cleanUrl is derived from current page URL, already resolved
    replaceState(cleanUrl.pathname + cleanUrl.search, {});

    if (threadId) {
      // Thread is open — route the highlight to the thread pane
      pendingThreadHighlight = highlightEventId;
    } else {
      tick().then(() => {
        jumpState.jumpToMessage(highlightEventId);
      });
    }
  });

  // Mark as read when new messages arrive from OTHER users
  useSpaceEvent((event) => {
    if (!event.event) return;

    if (event.event.__typename === 'MessagePostedEvent' && event.event.roomId === roomId) {
      if (!event.event.inThread) {
        typingIndicator.removeTypingUser(event.actorId);
      }

      if (currentUser.user && event.actorId !== currentUser.user.id && appState.isFocused) {
        unread.markRoomAsRead(spaceId, roomId);
      }
    }
  });

  // Header action visibility — flat derivations keep the template clean
  let showVoiceCall = $derived(!!room.roomData && !!instanceState.livekitUrl);
  let showRoomSettings = $derived(!!room.roomData && !room.isDM && !!room.roomData.canManageRoom);
  let showLeaveRoom = $derived(!!room.roomData && !room.isDM);

  let leavingRoom = $state(false);

  // Drop zone state for drag-and-drop image uploads
  let isDraggingFiles = $state(false);
  let composerApi = $state<MessageComposerApi | null>(null);

  // Drop zone attachment - only active when user can post messages
  const roomDropZone = $derived(
    room.roomData?.canPostMessage
      ? dropZone({
          onDrop: (files) => composerApi?.addFiles(files),
          onDragStateChange: (dragging) => (isDraggingFiles = dragging),
          acceptedTypes: ['image/*', 'video/*', 'audio/*']
        })
      : undefined
  );

  // Typing indicator for main room (not thread)
  const typingIndicator = createTypingIndicator(() => ({
    spaceId,
    roomId,
    threadRootEventId: null,
    currentUserId: currentUser.user?.id ?? null
  }));
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && threadId && !e.defaultPrevented) {
      e.preventDefault();
      closeThread();
    }
  }}
  onpointerdown={(e) => {
    if (!threadId || e.button !== 0) return;
    const target = e.target as HTMLElement;
    if (target.closest('[data-testid="thread-pane"], dialog')) return;
    closeThread();
  }}
/>

{#if room.roomData}
  {#if pageTitle}
    <PageTitle title={pageTitle} />
  {/if}

  <div class="flex min-h-0 min-w-0 flex-1">
    <div class="relative flex min-h-0 min-w-0 flex-1 overflow-hidden">
      <div
        class={[
          'relative flex min-h-0 min-w-0 flex-1 flex-col transition-opacity duration-200',
          threadId ? 'opacity-30' : ''
        ]}
        inert={threadId ? true : undefined}
        {@attach roomDropZone}
      >
        <DropZoneOverlay visible={isDraggingFiles} />

        <PaneHeader {title} loading={room.isRoomLoading} showMobileNav>
          {#snippet afterTitle()}
            {#if !sidebarNav.isOpen && !room.isDM && room.roomData?.spaceName}
              <span class="text-sm text-muted">{room.roomData.spaceName}</span>
            {/if}
          {/snippet}
          {#snippet actions()}
            {#if showVoiceCall}
              <VoiceCallButton {spaceId} {roomId} livekitUrl={instanceState.livekitUrl!} />
            {/if}
            {#if showRoomSettings}
              <a
                href={resolve('/chat/[instanceId]/[spaceId]/[roomId]/settings', { instanceId: instanceSegment, spaceId, roomId })}
                class="iconify cursor-pointer text-muted uil--setting hover:text-text"
                title="Room settings"
              >
              </a>
            {/if}
            {#if showLeaveRoom}
              <button
                class="iconify cursor-pointer text-muted uil--sign-out-alt hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
                onclick={() =>
                  pushState('', {
                    modal: {
                      type: 'leaveRoom',
                      spaceId,
                      roomId,
                      roomName: room.roomData!.room.name
                    }
                  })}
                disabled={leavingRoom}
                title="Leave room"
              >
              </button>
            {/if}
          {/snippet}
        </PaneHeader>

        {#if instanceState.livekitUrl}
          <VoiceCallPanel {spaceId} {roomId} livekitUrl={instanceState.livekitUrl} />
        {/if}

        <RoomEventsPane
          {spaceId}
          {roomId}
          unreadAfterTime={unread.unreadAfterTime}
          unreadBeforeTime={unread.unreadBeforeTime}
          onOpenThread={openThread}
          typingUserIds={typingIndicator.userIds}
          typingMembers={getRoomMembers()}
        />

        <MessageComposer
          {spaceId}
          {roomId}
          canPost={room.roomData?.canPostMessage ?? true}
          inReplyTo={replyState.messageEventId ?? undefined}
          replyDisplayName={replyState.actorDisplayName || undefined}
          replyExcerpt={replyState.excerpt || undefined}
          onCancelReply={() => replyState.cancelReply()}
          autoFocus={!threadId}
          onReady={(api) => (composerApi = api)}
          onTyping={() => typingIndicator?.sendTypingIndicator()}
          onMessageSent={() => typingIndicator?.resetDebounce()}
        />
      </div>

      {#if threadId && room.roomData}
        <ThreadPane
          {spaceId}
          {roomId}
          roomName={room.roomData.room.name}
          threadRootEventId={threadId}
          onClose={closeThread}
          canPostInThread={room.roomData.canPostInThread}
          canEchoMessage={room.roomData.canEchoMessage && room.roomData.canPostMessage}
          highlightEventId={pendingThreadHighlight}
          onHighlightComplete={() => {
            pendingThreadHighlight = null;
          }}
        />
      {/if}
    </div>

    {#if !room.isDM}
      <div class="hidden lg:flex">
        <RoomInfo loading={room.isRoomLoading} />
      </div>
    {/if}
  </div>
{/if}
