<script lang="ts">
  import { untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { MediaQuery } from 'svelte/reactivity';
  import { goto, pushState, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import {
    useRoomData,
    useRoomUnread,
    useProjectionEvent,
    usePresenceChange,
    createTypingIndicator
  } from '$lib/hooks';
  import { appState } from '$lib/state/globals.svelte';
  import * as m from '$lib/i18n/messages';
  import {
    createComposerContext,
    createMentionRoles,
    getRoomMembers,
    RoomMembersStore,
    setRoomMembersStore,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import { onRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { clearLastRoom, setLastRoom } from '$lib/storage/lastRoom';
  import {
    consumePendingRoomSidebarPanel,
    roomSidebarPanelStorageSuffix,
    type RoomSidebarPanel
  } from '$lib/storage/roomSidebarPanel';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { toast } from '$lib/ui/toast';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { tick } from 'svelte';
  import RoomEventsPane from './RoomEventsPane.svelte';
  import RoomSidebarPane from './RoomSidebarPane.svelte';
  import RoomSidebarToggle from './RoomSidebarToggle.svelte';
  import {
    canBanMembersFromRoomSidebar,
    roomSidebarPanelForRoom,
    roomSidebarPanelsForRoom,
    visibleRoomSidebarPanel
  } from './roomSidebarBehavior';
  import { RoomNavigationState } from './roomNavigationState.svelte';
  import { buildRoomPresentation } from './roomPresentation';
  import ThreadPane from './ThreadPane.svelte';
  import type { ThreadOpenOptions } from './threadOpenOptions';

  let {
    roomId,
    threadId,
    routeMessageId
  }: { roomId: string; threadId?: string; routeMessageId?: string } = $props();

  const connection = useConnection();
  const roomMembersStore = setRoomMembersStore(new RoomMembersStore(connection()));
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const stores = serverRegistry.getStore(getActiveServer());
  const roomFilesStore = $derived(stores.filesForRoom(roomId));
  const serverInfo = stores.serverInfo;
  const appUi = getAppUiState();
  const desktopRoomLayout = new MediaQuery('(min-width: 1024px)', false);

  const navigation = new RoomNavigationState();

  function openThread(threadRootEventId: string, options: ThreadOpenOptions = {}) {
    navigation.prepareThreadOpen(threadRootEventId, options);
    goto(
      resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: serverSegment,
        roomId,
        threadId: threadRootEventId
      })
    );
  }

  function closeThread() {
    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  // Create context-based state (must be synchronous, before children render)
  const composerContext = createComposerContext({ scroll: true });
  createMentionRoles(() => stores.mentionRoles.roles);
  const replyState = composerContext.replyState;
  let replyStateRoomId: string | null = null;
  const jumpState = composerContext.jumpState;
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const roomMessageStore = $derived(stores.messagesForRoom(roomId));

  $effect(() => {
    const selectedRoomId = roomId;
    untrack(() => stores.restoreProjectedRoomWindow(selectedRoomId));
    return () => {
      // Invalidate any historical-window request before this room becomes
      // inactive. Its late response must not replace the retained latest
      // projection while another room is being rendered.
      untrack(() => stores.restoreProjectedRoomWindow(selectedRoomId));
    };
  });

  $effect(() =>
    onRoomMessageMutated((detail) => {
      if (detail.roomId !== roomId) return;
      if (detail.reason === 'message-deleted') {
        roomMessageStore.applyLocalMessageDeletion(detail.eventId);
        return;
      }
      const anchorEventId = roomMessageStore.refreshAnchorForMessageMutation(detail.eventId);
      if (!anchorEventId) return;
      void roomMessageStore.refreshCurrentWindow(anchorEventId);
    })
  );

  // --- Extracted hooks ---
  const room = useRoomData(() => ({ roomId }));

  $effect(() => {
    const currentRoomId = roomId;
    if (replyStateRoomId === null) {
      replyStateRoomId = currentRoomId;
      return;
    }
    if (replyStateRoomId === currentRoomId) return;
    replyStateRoomId = currentRoomId;
    replyState.cancelReply();
  });

  $effect(() => {
    void stores.mentionRoles.refresh();
  });

  const unread = useRoomUnread(() => ({ roomId }));

  // Room permissions — derived reactively, no $effect needed
  let permissions = $derived(room.roomData ?? DEFAULT_ROOM_PERMISSIONS);
  let composerCanAttach = $derived(room.roomData === undefined ? true : permissions.canAttach);

  createRoomPermissions(() => permissions);

  // roomData === null means the ready projection contains no visible room
  // (deleted, archived, or no access), so reaching this branch is genuine — clear
  // lastRoom so [spaceId]/+page.svelte's onMount doesn't redirect us right
  // back here in an infinite loop.
  $effect.pre(() => {
    if (room.roomData === null) {
      clearLastRoom(getActiveServer());
      goto(resolve('/chat/[serverId]', { serverId: serverSegment }), { replaceState: true });
    }
  });

  const presentation = $derived(
    buildRoomPresentation({
      roomData: room.roomData,
      isDM: room.isDM,
      dmData: room.dmData,
      directMessageLabel: m['room.title.direct_message'](),
      currentUserLabel: m['common.you'](),
      getDisplayName: getLiveDisplayName
    })
  );

  // Remember this room as the last visited (for the chat-root → last-room
  // auto-redirect). Room.svelte is reused across roomId changes, so wait for
  // the loaded room data to catch up to the current route before writing.
  $effect(() => {
    if (room.roomData?.room.id === roomId) {
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

    const threadMessageTarget = navigation.consumeThreadMessageRoute(
      roomId,
      threadId,
      routeMessageId
    );
    if (threadMessageTarget !== undefined) {
      if (threadMessageTarget) applyHighlight(threadMessageTarget);
      return;
    }

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
    const requestId = navigation.beginHighlight(eventId, !!threadId);
    if (requestId === null) return;

    tick().then(async () => {
      const jumped = await jumpState.jumpToMessage(eventId);
      if (!jumped && navigation.failMainHighlight(requestId, eventId)) {
        toast.error(m['room.jump_failed']());
      }
    });
  }

  // Durable message rows arrive only through projection operations. Keep
  // presence/read side effects and the independent paginated files read model
  // aligned with those authoritative row replacements.
  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      if (operation.operation.case !== 'roomTimelineEventUpsert') continue;
      const update = operation.operation.value;
      if (update.roomId !== roomId || update.event?.event.case !== 'messagePosted') continue;
      const message = update.event.event.value.message;
      if (!message?.threadRootEventId) {
        const actorId = event.actorId;
        if (actorId) typingIndicator.removeTypingUser(actorId);
        if (currentUser.user && actorId !== currentUser.user.id && appState.isPresent) {
          // Projection envelopes for row replacements can be driven by an
          // asset/reaction fact whose ID is not itself part of the room
          // timeline. Anchor read state to the row being upserted.
          unread.markRoomAsRead(roomId, update.event.id);
        }
      }
    }
  });

  usePresenceChange((userId, status) => {
    roomMembersStore.updatePresence(userId, status);
  });

  // Header action visibility — flat derivations keep the template clean
  let showVoiceCall = $derived(!!room.roomData && !!serverInfo.livekitUrl);
  // Channel rooms can be left unless membership is granted by Universal policy.
  let showLeaveRoom = $derived(!!room.roomData && !room.isDM && !room.roomData.room.isUniversal);
  const activeRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(room.isDM, appUi.activeDesktopRoomSidebarPanel, showVoiceCall)
  );
  const mobileRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(room.isDM, appUi.mobileRoomSidebarPanel, showVoiceCall)
  );
  const roomFilesPanelActive = $derived(
    visibleRoomSidebarPanel(
      desktopRoomLayout.current,
      activeRoomSidebarPanel,
      mobileRoomSidebarPanel
    ) === 'files'
  );
  const roomSidebarTogglePanels = $derived(roomSidebarPanelsForRoom(room.isDM, showVoiceCall));
  const hasActiveRoomCall = $derived(
    stores.activeCallRooms.has(roomId) || stores.voiceCall.isInCall(roomId)
  );
  const isDesktopCallMaximized = $derived(
    activeRoomSidebarPanel === 'call' &&
      hasActiveRoomCall &&
      appUi.isRoomCallWideFor(getActiveServer(), roomId)
  );
  const sharedRoomSidebarProps = $derived({
    roomId,
    hasActiveCall: hasActiveRoomCall,
    loading: room.isRoomLoading,
    filesStore: roomFilesStore,
    livekitUrl: serverInfo.livekitUrl ?? undefined,
    canBanRoomMembers: canBanMembersFromRoomSidebar(room.isDM, room.roomData?.canBanRoomMembers),
    currentUserId: currentUser.user?.id ?? null,
    membersStore: roomMembersStore
  });

  const syncRoomMembers: Attachment = () => {
    const selectedRoomId = roomId;
    const hasCompleteMembership = stores.hasCompleteProjectedRoomMembership(selectedRoomId);
    const projectedMembers = hasCompleteMembership
      ? stores.projectedMembersForRoom(selectedRoomId)
      : [];
    untrack(() => {
      roomMembersStore.setRoom(selectedRoomId);
      if (hasCompleteMembership) {
        roomMembersStore.replaceProjection(selectedRoomId, projectedMembers);
      } else {
        roomMembersStore.awaitProjection(selectedRoomId);
      }
    });
  };

  const syncRoomFiles: Attachment = () => {
    const store = roomFilesStore;
    const active = roomFilesPanelActive;
    if (active) return untrack(() => store.retain());
  };

  const syncRoomCallWide: Attachment = () => {
    const active = hasActiveRoomCall;
    const serverId = getActiveServer();
    const selectedRoomId = roomId;
    if (!active) {
      untrack(() => appUi.disableRoomCallWideFor(serverId, selectedRoomId));
    }
  };

  let leavingRoom = $state(false);

  function toggleDesktopRoomSidebarPanel(panel: RoomSidebarPanel): void {
    appUi.toggleDesktopRoomSidebarPanel(panel);
  }

  function closeDesktopRoomSidebarPanel(): void {
    appUi.closeDesktopRoomSidebarPanel();
  }

  function toggleDesktopCallWide(): void {
    if (activeRoomSidebarPanel !== 'call' || !hasActiveRoomCall) return;
    appUi.toggleRoomCallWide(getActiveServer(), roomId);
  }

  function openRoomSidebarPanel(panel: RoomSidebarPanel): void {
    if (window.matchMedia('(min-width: 1024px)').matches) {
      appUi.openDesktopRoomSidebarPanel(panel);
    } else {
      appUi.openMobileRoomSidebarPanel(panel);
    }
  }

  function handleRoomSidebarPanelStorage(event: StorageEvent): void {
    const key = serverStorageKey(getActiveServer(), roomSidebarPanelStorageSuffix(roomId));
    if (event.key !== key) return;
    if (event.newValue !== 'call') return;

    consumePendingRoomSidebarPanel(getActiveServer(), roomId);
    openRoomSidebarPanel('call');
  }

  $effect(() => {
    const pendingPanel = consumePendingRoomSidebarPanel(getActiveServer(), roomId);
    if (pendingPanel) openRoomSidebarPanel(pendingPanel);
  });

  function openFileMessage(
    messageEventId: string,
    threadRootEventId: string | null,
    closeMobile = false
  ): void {
    if (threadRootEventId) {
      openThread(threadRootEventId, { highlightEventId: messageEventId });
    } else {
      void jumpState.jumpToMessage(messageEventId);
    }
    if (closeMobile) {
      appUi.closeMobileRoomSidebarPanel();
    }
  }

  // Drop zone state for drag-and-drop image uploads
  let isDraggingFiles = $state(false);
  let composerApi = $state<MessageComposerApi | null>(null);

  // Drop zone attachment - only active when user can post and attach files.
  const roomDropZone = $derived(
    room.roomData?.canPostMessage && room.roomData?.canAttach
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
  onstorage={handleRoomSidebarPanelStorage}
  onkeydown={(e) => {
    if (e.key === 'Escape' && mobileRoomSidebarPanel && !e.defaultPrevented) {
      e.preventDefault();
      appUi.closeMobileRoomSidebarPanel();
      return;
    }

    if (e.key === 'Escape' && threadId && !e.defaultPrevented) {
      e.preventDefault();
      closeThread();
    }
  }}
  onpointerdown={(e) => {
    if (mobileRoomSidebarPanel && e.button === 0) {
      const target = e.target as HTMLElement;
      if (
        target.closest(
          '[data-testid="room-sidebar-mobile-pane"], [data-testid="room-sidebar-toggle"], dialog'
        )
      ) {
        return;
      }
      appUi.closeMobileRoomSidebarPanel();
      return;
    }

    if (!threadId || e.button !== 0) return;
    const target = e.target as HTMLElement;
    if (target.closest('[data-testid="thread-pane"], dialog')) return;
    // A thread is an overlay over the room view, so only the dimmed room
    // surface behind it should behave as a click-outside dismissal target.
    // Controls elsewhere in the app (such as the room extras sidebar) manage
    // their own state and must not close the thread as a side effect.
    if (!target.closest('[data-thread-dismiss-surface]')) return;
    closeThread();
  }}
/>

<!--
  Render the layout shell whether or not roomData has loaded. EventList stays
  mounted across roomId changes, so scroll and virtualization state can settle
  without remounting the whole room body.

  roomData === null triggers a redirect via $effect.pre above, so we skip
  rendering in that case to avoid a flash of the previous room's UI under
  the new (empty) data.
-->
{#if room.roomData !== null}
  {#if presentation.pageTitle}
    <PageTitle title={presentation.pageTitle} />
  {/if}

  <div
    class="flex min-h-0 min-w-0 flex-1"
    {@attach syncRoomMembers}
    {@attach syncRoomFiles}
    {@attach syncRoomCallWide}
  >
    <div
      class={[
        'relative flex min-h-0 min-w-0 flex-1 overflow-hidden',
        isDesktopCallMaximized ? 'lg:hidden' : ''
      ]}
      data-testid="room-view-region"
      data-thread-dismiss-surface
    >
      <div
        class={[
          'relative flex min-h-0 min-w-0 flex-1 flex-col transition-opacity duration-200',
          threadId ? 'opacity-30' : '',
          mobileRoomSidebarPanel ? 'max-lg:opacity-30' : ''
        ]}
        inert={threadId || mobileRoomSidebarPanel ? true : undefined}
        {@attach roomDropZone}
      >
        <DropZoneOverlay visible={isDraggingFiles} />

        <PaneHeader
          title={presentation.title}
          subtitle={presentation.description}
          loading={!room.roomData}
        >
          {#snippet actions()}
            <RoomSidebarToggle
              mode="mobile"
              activePanel={mobileRoomSidebarPanel}
              panels={roomSidebarTogglePanels}
              hasActiveCall={hasActiveRoomCall}
              onToggle={(panel) => appUi.toggleMobileRoomSidebarPanel(panel)}
            />
            <RoomSidebarToggle
              mode="desktop"
              activePanel={activeRoomSidebarPanel}
              panels={roomSidebarTogglePanels}
              hasActiveCall={hasActiveRoomCall}
              onToggle={toggleDesktopRoomSidebarPanel}
            />
            {#if showLeaveRoom}
              <button
                class="group/pane-header-icon-button pane-header-icon-button"
                onclick={() =>
                  pushState('', {
                    modal: {
                      type: 'leaveRoom',
                      roomId,
                      roomName: room.roomData!.room.name
                    }
                  })}
                disabled={leavingRoom}
                title={m['room.leave.title']()}
              >
                <span class="pane-header-icon-glyph uil--sign-out-alt" aria-hidden="true"></span>
              </button>
            {/if}
          {/snippet}
        </PaneHeader>

        <RoomEventsPane
          {roomId}
          messageStore={roomMessageStore}
          unreadMarkerEventId={unread.unreadMarkerEventId}
          unreadMarkerWindow={unread.unreadMarkerWindow}
          onUnreadMarkerResolved={(eventId) => unread.setUnreadMarkerEventId(eventId)}
          onUnreadMarkerCleared={() => unread.clearUnreadMarker()}
          onOpenThread={openThread}
          pendingHighlightId={navigation.pendingMainHighlightId}
          onHighlightComplete={() => navigation.clearMainHighlight()}
          typingUserIds={typingIndicator.userIds}
          typingMembers={getRoomMembers()}
        />

        <MessageComposer
          {roomId}
          canPost={permissions.canPostMessage}
          canAttach={composerCanAttach}
          inReplyTo={replyState.messageEventId ?? undefined}
          replyDisplayName={replyState.actorDisplayName || undefined}
          replyExcerpt={replyState.excerpt || undefined}
          onCancelReply={() => replyState.cancelReply()}
          autoFocus={!threadId && !mobileRoomSidebarPanel}
          onReady={(api) => (composerApi = api)}
          onTyping={() => typingIndicator?.sendTypingIndicator()}
          onMessageSent={(event) => {
            typingIndicator?.resetDebounce();
            if (event) {
              roomMessageStore.ingestEvent(event);
            } else {
              void roomMessageStore.refreshCurrentWindow(null);
            }
          }}
        />
      </div>

      {#if threadId && room.roomData}
        <ThreadPane
          {roomId}
          roomName={room.roomData.room.name}
          threadRootEventId={threadId}
          onClose={closeThread}
          canPostInThread={room.roomData.canPostInThread}
          canAttach={room.roomData.canAttach}
          canEchoMessage={room.roomData.canEchoMessage && room.roomData.canPostMessage}
          highlightEventId={navigation.pendingThreadHighlight}
          pendingQuote={navigation.pendingThreadQuote}
          pendingReply={navigation.pendingThreadReply}
          onHighlightComplete={() => navigation.clearThreadHighlight()}
          onQuoteConsumed={() => navigation.clearThreadQuote()}
          onReplyConsumed={() => navigation.clearThreadReply()}
        />
      {/if}

      <RoomSidebarPane
        presentation="mobile"
        sidebarProps={mobileRoomSidebarPanel
          ? {
              ...sharedRoomSidebarProps,
              activePanel: mobileRoomSidebarPanel,
              onOpenFile: (messageEventId, threadRootEventId) =>
                openFileMessage(messageEventId, threadRootEventId, true),
              onClose: () => appUi.closeMobileRoomSidebarPanel()
            }
          : null}
      />
    </div>

    {#if activeRoomSidebarPanel}
      <RoomSidebarPane
        presentation="desktop"
        sidebarProps={{
          ...sharedRoomSidebarProps,
          activePanel: activeRoomSidebarPanel,
          maximized: isDesktopCallMaximized,
          onOpenFile: openFileMessage,
          onToggleMaximized: toggleDesktopCallWide,
          onClose: closeDesktopRoomSidebarPanel
        }}
      />
    {/if}
  </div>
{/if}
