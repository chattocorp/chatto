<script lang="ts">
  import { goto, pushState, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import VoiceCallButton from '$lib/components/voice/VoiceCallButton.svelte';
  import VoiceCallPanel from '$lib/components/voice/VoiceCallPanel.svelte';
  import { useRoomData, useRoomMembersSync, useRoomUnread, useEvent, createTypingIndicator } from '$lib/hooks';
  import { appState, sidebarNav } from '$lib/state/globals.svelte';
  import { createComposerContext, getRoomMembers, createRoomPermissions, DEFAULT_ROOM_PERMISSIONS } from '$lib/state/room';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { clearLastRoom, setLastRoom } from '$lib/storage/lastRoom';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { tick } from 'svelte';
  import RoomEventsPane from './RoomEventsPane.svelte';
  import RoomSidebar from './RoomSidebar.svelte';
  import ThreadPane from './ThreadPane.svelte';

  let { roomId, threadId }: { roomId: string; threadId?: string } = $props();

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const stores = serverRegistry.getStore(getActiveServer());
  const serverInfo = stores.serverInfo;
  const notificationStore = stores.notifications;

  // Thread navigation functions (URL-driven state)
  let pendingThreadHighlight = $state<string | null>(null);

  function openThread(threadRootEventId: string, highlightEventId?: string) {
    pendingThreadHighlight = highlightEventId ?? null;
    goto(resolve('/chat/[serverId]/[roomId]/[threadId]', { serverId: serverSegment, roomId, threadId: threadRootEventId }));
  }

  function closeThread() {
    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  // Create context-based state (must be synchronous, before children render)
  const composerContext = createComposerContext({ scroll: true });
  const replyState = composerContext.replyState;
  const jumpState = composerContext.jumpState;
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);

  // --- Extracted hooks ---
  const room = useRoomData(() => ({ roomId }));

  useRoomMembersSync(() => ({
    roomId,
    isDM: room.isDM,
    roomData: room.roomData,
    dmData: room.dmData
  }));

  const unread = useRoomUnread(() => ({ roomId }));

  // Room permissions — derived reactively, no $effect needed
  let permissions = $derived(room.roomData ?? DEFAULT_ROOM_PERMISSIONS);

  createRoomPermissions(() => permissions);

  // roomData === null means the server returned a clean response with no room
  // (deleted, archived, no access). Transient network failures are filtered
  // upstream in useRoomData, so reaching this branch is genuine — clear
  // lastRoom so [spaceId]/+page.svelte's onMount doesn't redirect us right
  // back here in an infinite loop.
  $effect.pre(() => {
    if (room.roomData === null) {
      clearLastRoom(getActiveServer());
      goto(resolve('/chat/[serverId]', { serverId: serverSegment }), { replaceState: true });
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
      notificationStore.dismissRoomMessageNotifications(currentRoomId);
    }
  });

  // Remember this room as the last visited (for the chat-root → last-room
  // auto-redirect). DM rooms are deliberately excluded: their lifecycle is
  // user-driven (start a conversation, post a message), not "the room I was
  // last in," and auto-landing on a DM after returning to the instance is
  // surprising — channels are the implicit destination.
  $effect(() => {
    if (room.roomData && !room.isDM) {
      setLastRoom(getActiveServer(), roomId);
    }
  });

  // Resolve the pending highlight once room data has loaded for the
  // current roomId. Two sources, in priority order:
  //   1. PendingHighlightStore — set by in-app navigations (notification
  //      clicks, message-link redirects). One-shot, consumed-on-success.
  //   2. ?highlight= URL param — for shareable permalinks. Stripped after
  //      consumption so a refresh doesn't re-fire it.
  $effect(() => {
    if (!room.roomData) return;
    // Room.svelte lives in +layout and is reused across roomId changes; bail
    // until the new room's data has actually loaded.
    if (room.roomData.room.id !== roomId) return;

    const pending = stores.pendingHighlights.consume(roomId, threadId ?? null);
    if (pending) {
      applyHighlight(pending);
      return;
    }

    const fromUrl = page.url.searchParams.get('highlight');
    if (!fromUrl) return;

    if (threadId) {
      replaceState(
        resolve('/chat/[serverId]/[roomId]/[threadId]', {
          serverId: serverSegment,
          roomId,
          threadId
        }),
        {}
      );
    } else {
      replaceState(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }), {});
    }
    applyHighlight(fromUrl);
  });

  function applyHighlight(eventId: string): void {
    if (threadId) {
      pendingThreadHighlight = eventId;
    } else {
      tick().then(() => {
        jumpState.jumpToMessage(eventId);
      });
    }
  }

  // Keep the read cursor in sync with incoming root messages:
  // - Other users' messages mark the room read (with explicit event ID, so
  //   the server cursor matches what the client rendered) while the user is
  //   actually present (focused + visible).
  // - The user's own posts already auto-mark the room read server-side, so
  //   we just mirror that onto the local cursor — without it, backgrounding
  //   the tab would strand the user's own latest message below the unread
  //   separator.
  useEvent((event) => {
    if (!event.event) return;

    if (event.event.__typename === 'MessagePostedEvent' && event.event.roomId === roomId) {
      const actorId = event.actorId;

      if (!event.event.threadRootEventId) {
        if (actorId) {
          typingIndicator.removeTypingUser(actorId);
        }
      }

      if (!event.event.threadRootEventId && currentUser.user) {
        if (actorId === currentUser.user.id) {
          unread.noteReadCursor(event.createdAt);
        } else if (appState.isPresent) {
          unread.markRoomAsRead(roomId, event.id);
        }
      }
    }
  });

  // Header action visibility — flat derivations keep the template clean
  let showVoiceCall = $derived(!!room.roomData && !!serverInfo.livekitUrl);
  // Channel rooms can always be left. DMs are permanent (no leave action).
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

<!--
  Render the layout shell whether or not roomData has loaded. EventList
  already manages its own skeleton via the messages store's
  isInitialLoading flag, and stays mounted across roomId changes — so it
  becomes the single skeleton element throughout the loading transition,
  with no remount and no shimmer-phase reset.

  roomData === null triggers a redirect via $effect.pre above, so we skip
  rendering in that case to avoid a flash of the previous room's UI under
  the new (empty) data.
-->
{#if room.roomData !== null}
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

        <PaneHeader {title} loading={!room.roomData}>
          {#snippet afterTitle()}
            {#if !sidebarNav.isOpen && !room.isDM && room.roomData?.spaceName}
              <span class="text-sm text-muted">{room.roomData.spaceName}</span>
            {/if}
          {/snippet}
          {#snippet actions()}
            {#if showVoiceCall}
              <VoiceCallButton {roomId} livekitUrl={serverInfo.livekitUrl!} />
            {/if}
            {#if showLeaveRoom}
              <button
                class="iconify cursor-pointer text-muted uil--sign-out-alt hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
                onclick={() =>
                  pushState('', {
                    modal: {
                      type: 'leaveRoom',
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

        {#if room.roomData && serverInfo.livekitUrl}
          <VoiceCallPanel {roomId} livekitUrl={serverInfo.livekitUrl} />
        {/if}

        <RoomEventsPane
          {roomId}
          unreadAfterTime={unread.unreadAfterTime}
          unreadBeforeTime={unread.unreadBeforeTime}
          onOpenThread={openThread}
          typingUserIds={typingIndicator.userIds}
          typingMembers={getRoomMembers()}
        />

        <MessageComposer
          {roomId}
          canPost={permissions.canPostMessage}
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
        <RoomSidebar
          {roomId}
          loading={room.isRoomLoading}
          canBanRoomMembers={room.roomData?.canBanRoomMembers ?? false}
          currentUserId={currentUser.user?.id ?? null}
        />
      </div>
    {/if}
  </div>
{/if}
