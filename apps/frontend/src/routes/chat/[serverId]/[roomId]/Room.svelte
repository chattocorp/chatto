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
  import { m } from '$lib/i18n/messages';
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
  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import { threadPaneWidth } from '$lib/state/threadPaneWidth.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { clearLastRoom, setLastRoom } from '$lib/storage/lastRoom';
  import type { RoomSidebarPanel } from '$lib/storage/roomSidebarPanel';
  import { toast } from '$lib/ui/toast';
  import { EmptyState } from '$lib/ui';
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
  import type { ThreadOpenOptions } from './threadOpenOptions';
  import { RoomThreadingMode } from '$lib/roomThreading';
  import { recentThreadRootCandidate } from './recentThreadRoot';

  let threadPaneModule: Promise<typeof import('./ThreadPane.svelte')> | null = null;
  let threadPaneLoadAttempt = $state(0);

  function loadThreadPane(_attempt: number) {
    threadPaneModule ??= import('./ThreadPane.svelte').catch((error: unknown) => {
      threadPaneModule = null;
      throw error;
    });
    return threadPaneModule;
  }

  let {
    roomId,
    threadId,
    routeMessageId
  }: { roomId: string; threadId?: string; routeMessageId?: string } = $props();

  const serverScope = useServerScope();
  const connection = () => serverScope.connection;
  const roomMembersStore = setRoomMembersStore(new RoomMembersStore(connection()));
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const stores = $derived(serverScope.store);
  const roomFilesStore = $derived(stores.filesForRoom(roomId));
  const roomMessageSearchStore = $derived(stores.messageSearchForRoom(roomId));
  const serverInfo = $derived(stores.serverInfo);
  const appUi = getAppUiState();
  const desktopRoomLayout = new MediaQuery('(min-width: 1024px)', false);
  const THREAD_PANE_SPLIT_MIN_WIDTH = 768;
  let splitThreadLayout = $state(false);

  const observeThreadLayout: Attachment<HTMLElement> = (element) => {
    const update = (width: number) => {
      splitThreadLayout = width >= THREAD_PANE_SPLIT_MIN_WIDTH;
    };
    update(element.getBoundingClientRect().width);

    let frame = 0;
    const observer = new ResizeObserver(([entry]) => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => update(entry.contentRect.width));
    });
    observer.observe(element);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  };

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
  const currentUser = $derived(stores.currentUser);
  const roomMessageStore = $derived(stores.messagesForRoom(roomId));
  const room = useRoomData(() => ({ roomId }));
  const canReadMessages = $derived(room.roomData?.canReadMessages !== false);
  const shouldHydrateRoom = $derived(Boolean(room.roomData) && canReadMessages);

  $effect(() => {
    const mountedStores = stores;
    const selectedRoomId = roomId;
    const hydrateRoom = shouldHydrateRoom;
    if (hydrateRoom) untrack(() => mountedStores.restoreProjectedRoomWindow(selectedRoomId));
    return () => {
      // Invalidate any historical-window request before this room becomes
      // inactive. Its late response must not replace the retained latest
      // projection while another room is being rendered.
      navigation.clearMainHighlight();
      if (hydrateRoom) untrack(() => mountedStores.restoreProjectedRoomWindow(selectedRoomId));
    };
  });

  $effect(() =>
    onRoomMessageMutated((detail) => {
      if (detail.serverId !== activeServerId || detail.roomId !== roomId) return;
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
  const supportsPinnedMessages = $derived(serverInfo.supportsFeature('pinnedMessages'));
  const roomPinsStore = $derived(
    room.roomData && canReadMessages && !room.isDM && supportsPinnedMessages
      ? stores.pinsForRoom(roomId)
      : null
  );

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

  const unread = useRoomUnread(() => ({
    roomId,
    events: roomMessageStore.rootEvents,
    canReadMessages
  }));

  // Room permissions — derived reactively, no $effect needed
  let permissions = $derived({
    ...(room.roomData ?? DEFAULT_ROOM_PERMISSIONS),
    canViewPinnedMessages: Boolean(roomPinsStore),
    canPinMessages:
      Boolean(room.roomData) &&
      !room.isDM &&
      !room.roomData?.room.archived &&
      Boolean(roomPinsStore) &&
      Boolean(room.roomData?.canManageRoom)
  });
  let composerCanAttach = $derived(room.roomData === undefined ? true : permissions.canAttach);
  let threadingMode = $derived(room.roomData?.room.threadingMode ?? RoomThreadingMode.ENABLED);
  let composerCanCreateThread = $derived(
    !room.isDM &&
      permissions.canPostMessage &&
      (threadingMode === RoomThreadingMode.REQUIRED ||
        (permissions.canPostInThread &&
          (threadingMode === RoomThreadingMode.ENABLED ||
            threadingMode === RoomThreadingMode.ENCOURAGED)))
  );
  let composerRequiresThread = $derived(
    !room.isDM && permissions.canPostMessage && threadingMode === RoomThreadingMode.REQUIRED
  );

  function getRecentThreadRootCandidate() {
    const currentUserId = currentUser.user?.id;
    if (
      !currentUserId ||
      room.isDM ||
      !permissions.canPostInThread ||
      threadingMode === RoomThreadingMode.DISABLED
    ) {
      return null;
    }
    return recentThreadRootCandidate(roomMessageStore.rootEvents, roomId, currentUserId, Date.now());
  }

  createRoomPermissions(() => permissions);

  // roomData === null means the ready projection contains no visible room
  // (deleted, archived, or no access), so reaching this branch is genuine — clear
  // lastRoom so [spaceId]/+page.svelte's onMount doesn't redirect us right
  // back here in an infinite loop.
  $effect.pre(() => {
    if (room.roomData === null) {
      clearLastRoom(activeServerId);
      goto(resolve('/chat/[serverId]', { serverId: serverSegment }), { replaceState: true });
    }
  });

  const presentation = $derived(
    buildRoomPresentation({
      roomData: room.roomData,
      isDM: room.isDM,
      dmData: room.dmData,
      directMessageLabel: m('room.title.direct_message'),
      currentUserLabel: m('common.you'),
      getDisplayName: getLiveDisplayName
    })
  );

  // Remember this room as the last visited (for the chat-root → last-room
  // auto-redirect). Room.svelte is reused across roomId changes, so wait for
  // the loaded room data to catch up to the current route before writing.
  $effect(() => {
    if (room.roomData?.room.id === roomId) {
      setLastRoom(activeServerId, roomId);
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
      applyHighlight(pending.eventId, pending.notificationId);
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

  function applyHighlight(eventId: string, notificationId: string | null = null): void {
    const requestId = navigation.beginHighlight(eventId, !!threadId);
    if (requestId === null) return;
    const targetRoomId = roomId;

    tick().then(async () => {
      const jumped = await jumpState.jumpToMessage(eventId);
      if (!serverScope.isCurrent() || targetRoomId !== roomId) return;
      if (jumped && notificationId) {
        void stores.notifications.markOccurrenceRead(notificationId).catch((error) => {
          console.error('Failed to mark displayed notification read:', error);
        });
      }
      if (!jumped && navigation.failMainHighlight(requestId, eventId)) {
        toast.error(m('room.jump_failed'));
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
  const supportsMessageSearch = $derived(serverInfo.supportsFeature('messageSearch'));
  const messageSearchAvailable = $derived(
    supportsMessageSearch &&
      !stores.messageSearch.statusLoading &&
      (stores.messageSearch.statusError ||
        (stores.messageSearch.statusLoaded &&
          stores.messageSearch.status.state !== MessageSearchState.DISABLED))
  );
  $effect(() => {
    if (supportsMessageSearch) void stores.messageSearch.ensureStatus();
  });
  // Channel rooms can be left unless membership is granted by Universal policy.
  let showLeaveRoom = $derived(!!room.roomData && !room.isDM && !room.roomData.room.isUniversal);
  const activeRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(
      room.isDM,
      appUi.activeDesktopRoomSidebarPanel,
      showVoiceCall,
      messageSearchAvailable,
      supportsPinnedMessages
    )
  );
  const mobileRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(
      room.isDM,
      appUi.mobileRoomSidebarPanel,
      showVoiceCall,
      messageSearchAvailable,
      supportsPinnedMessages
    )
  );
  const roomFilesPanelActive = $derived(
    visibleRoomSidebarPanel(
      desktopRoomLayout.current,
      activeRoomSidebarPanel,
      mobileRoomSidebarPanel
    ) === 'files'
  );
  const roomPinsPanelActive = $derived(
    visibleRoomSidebarPanel(
      desktopRoomLayout.current,
      activeRoomSidebarPanel,
      mobileRoomSidebarPanel
    ) === 'pins'
  );
  const roomSidebarTogglePanels = $derived(
    roomSidebarPanelsForRoom(
      room.isDM,
      showVoiceCall,
      messageSearchAvailable,
      supportsPinnedMessages
    )
  );
  const hasActiveRoomCall = $derived(
    stores.activeCallRooms.has(roomId) || stores.voiceCall.isInCall(roomId)
  );
  const isDesktopCallMaximized = $derived(
    activeRoomSidebarPanel === 'call' &&
      hasActiveRoomCall &&
      appUi.isRoomCallWideFor(activeServerId, roomId)
  );
  const sharedRoomSidebarProps = $derived({
    roomId,
    hasActiveCall: hasActiveRoomCall,
    loading: room.isRoomLoading,
    searchStore: roomMessageSearchStore,
    filesStore: roomFilesStore,
    pinsStore: roomPinsStore ?? undefined,
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

  const syncRoomPins: Attachment = () => {
    const store = roomPinsStore;
    if (!store) return;
    return untrack(() => store.retain());
  };

  $effect(() => {
    if (roomPinsPanelActive) roomPinsStore?.markSeen();
  });

  const syncRoomCallWide: Attachment = () => {
    const active = hasActiveRoomCall;
    const serverId = activeServerId;
    const selectedRoomId = roomId;
    if (!active) {
      untrack(() => appUi.disableRoomCallWideFor(serverId, selectedRoomId));
    }
  };

  let leavingRoom = $state(false);

  function toggleDesktopRoomSidebarPanel(panel: RoomSidebarPanel): void {
    appUi.toggleDesktopRoomSidebarPanel(panel);
  }

  function openRoomCall(): void {
    appUi.requestRoomSidebarPanel(activeServerId, roomId, 'call', getRoomSidebarPresentation());
  }

  function closeDesktopRoomSidebarPanel(): void {
    appUi.closeDesktopRoomSidebarPanel();
  }

  function handleWindowKeydown(event: KeyboardEvent): void {
    if (
      event.defaultPrevented ||
      event.altKey ||
      event.shiftKey ||
      (!event.metaKey && !event.ctrlKey) ||
      event.key !== '/' ||
      !messageSearchAvailable
    ) {
      return;
    }

    event.preventDefault();
    if (desktopRoomLayout.current) {
      appUi.openDesktopRoomSidebarPanel('search');
    } else {
      appUi.openMobileRoomSidebarPanel('search');
    }
  }

  function toggleDesktopCallWide(): void {
    if (activeRoomSidebarPanel !== 'call' || !hasActiveRoomCall) return;
    appUi.toggleRoomCallWide(activeServerId, roomId);
  }

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

  function openSearchResult(
    messageEventId: string,
    threadRootEventId: string | null,
    closeMobile = false
  ): void {
    if (threadRootEventId) {
      openThread(threadRootEventId, { highlightEventId: messageEventId });
    } else {
      void jumpState.jumpToMessage(messageEventId);
    }
    if (closeMobile) appUi.closeMobileRoomSidebarPanel();
  }

  function openPinnedMessage(
    messageEventId: string,
    threadRootEventId: string | null,
    closeMobile = false
  ): void {
    openFileMessage(messageEventId, threadRootEventId, closeMobile);
  }

  // Drop zone state for drag-and-drop uploads
  let isDraggingFiles = $state(false);
  let composerApi = $state<MessageComposerApi | null>(null);

  // Drop zone attachment - only active when user can post and attach files.
  const roomDropZone = $derived(
    room.roomData?.canPostMessage && room.roomData?.canAttach
      ? dropZone({
          onDrop: (files) => composerApi?.addFiles(files),
          onDragStateChange: (dragging) => (isDraggingFiles = dragging)
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
    handleWindowKeydown(e);
    if (e.defaultPrevented) return;

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

    if (!threadId || splitThreadLayout || e.button !== 0) return;
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
    {@attach syncRoomPins}
    {@attach syncRoomCallWide}
  >
    <div
      class={[
        '@container relative flex min-h-0 min-w-0 flex-1 overflow-hidden',
        isDesktopCallMaximized ? 'lg:hidden' : ''
      ]}
      data-testid="room-view-region"
      data-thread-presentation={splitThreadLayout ? 'split' : 'overlay'}
      data-thread-dismiss-surface
      {@attach observeThreadLayout}
    >
      <div
        class={[
          'relative flex min-h-0 min-w-0 flex-1 flex-col transition-opacity duration-200',
          threadId && canReadMessages ? 'opacity-30 @min-[768px]:opacity-100' : '',
          mobileRoomSidebarPanel ? 'max-lg:opacity-30' : ''
        ]}
        data-testid="room-main-pane"
        inert={(threadId && canReadMessages && !splitThreadLayout) || mobileRoomSidebarPanel
          ? true
          : undefined}
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
              hasUnseenPins={roomPinsStore?.hasUnseen ?? false}
              onToggle={(panel) => appUi.toggleMobileRoomSidebarPanel(panel)}
            />
            <RoomSidebarToggle
              mode="desktop"
              activePanel={activeRoomSidebarPanel}
              panels={roomSidebarTogglePanels}
              hasActiveCall={hasActiveRoomCall}
              hasUnseenPins={roomPinsStore?.hasUnseen ?? false}
              onToggle={toggleDesktopRoomSidebarPanel}
            />
            {#if showLeaveRoom}
              <button
                class="group/pane-header-icon-button pane-header-icon-button"
                onclick={() =>
                  pushState('', {
                    modal: {
                      type: 'leaveRoom',
                      serverId: activeServerId,
                      roomId,
                      roomName: room.roomData!.room.name
                    }
                  })}
                disabled={leavingRoom}
                title={m('room.leave.title')}
              >
                <span class="icon-[uil--sign-out-alt] pane-header-icon-glyph" aria-hidden="true"
                ></span>
              </button>
            {/if}
          {/snippet}
        </PaneHeader>

        {#if canReadMessages}
          <RoomEventsPane
            {roomId}
            messageStore={roomMessageStore}
            unreadMarkerEventId={unread.unreadMarkerEventId}
            onUnreadMarkerCleared={() => unread.clearUnreadMarker()}
            onOpenThread={openThread}
            onOpenCall={openRoomCall}
            pendingHighlightId={navigation.pendingMainHighlightId}
            onHighlightComplete={() => navigation.clearMainHighlight()}
            typingUserIds={typingIndicator.userIds}
            typingMembers={getRoomMembers()}
            {threadingMode}
          />
        {:else}
          <EmptyState icon="icon-[uil--eye-slash]">
            {m('room.timeline.read_denied')}
          </EmptyState>
        {/if}

        <MessageComposer
          {roomId}
          canPost={permissions.canPostMessage}
          canAttach={composerCanAttach}
          slowModeSeconds={room.roomData?.room.slowModeSeconds ?? 0}
          slowModeNextPostAt={room.roomData?.slowModeNextPostAt ?? null}
          slowModeBypassed={permissions.canManageRoom || permissions.canManageOthersMessage}
          showCreateThread={composerCanCreateThread}
          createThreadRequired={composerRequiresThread}
          createThreadDefault={threadingMode === RoomThreadingMode.ENCOURAGED}
          threadsEncouraged={threadingMode === RoomThreadingMode.ENCOURAGED}
          {getRecentThreadRootCandidate}
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
          onThreadCreated={(threadRootEventId) => openThread(threadRootEventId)}
          onThreadMessageSent={(threadRootEventId, event) => {
            typingIndicator?.resetDebounce();
            openThread(threadRootEventId, { highlightEventId: event?.id });
          }}
        />
      </div>

      {#if threadId && room.roomData && canReadMessages}
        {#await loadThreadPane(threadPaneLoadAttempt)}
          <div
            class="absolute inset-y-0 end-0 z-10 flex min-h-0 w-full min-w-0 flex-col items-center justify-center overflow-hidden border-s border-border bg-background p-4 text-sm text-muted inline-end-overlay-shadow sm:w-[90%] @min-[768px]:relative @min-[768px]:inset-auto @min-[768px]:z-auto @min-[768px]:w-[var(--thread-pane-width)] @min-[768px]:shrink-0 @min-[768px]:shadow-none"
            data-testid="thread-pane"
            aria-busy="true"
            style:--thread-pane-width={`${threadPaneWidth.value}px`}
          >
            {m('common.loading')}
          </div>
        {:then { default: ThreadPane }}
          <ThreadPane
            {roomId}
            roomName={room.roomData.room.name}
            threadRootEventId={threadId}
            onClose={closeThread}
            canPostInThread={room.roomData.canPostInThread &&
              threadingMode !== RoomThreadingMode.DISABLED}
            canAttach={room.roomData.canAttach && threadingMode !== RoomThreadingMode.DISABLED}
            canEchoMessage={room.roomData.canEchoMessage &&
              room.roomData.canPostMessage &&
              threadingMode !== RoomThreadingMode.DISABLED}
            slowModeSeconds={room.roomData.room.slowModeSeconds}
            slowModeNextPostAt={room.roomData.slowModeNextPostAt}
            slowModeBypassed={room.roomData.canManageRoom || room.roomData.canManageOthersMessage}
            highlightEventId={navigation.pendingThreadHighlight}
            pendingQuote={navigation.pendingThreadQuote}
            pendingReply={navigation.pendingThreadReply}
            {threadingMode}
            onHighlightComplete={() => navigation.clearThreadHighlight()}
            onQuoteConsumed={() => navigation.clearThreadQuote()}
            onReplyConsumed={() => navigation.clearThreadReply()}
          />
        {:catch}
          <div
            class="absolute inset-y-0 end-0 z-10 flex min-h-0 w-full min-w-0 flex-col items-center justify-center gap-3 overflow-hidden border-s border-border bg-background p-4 text-center inline-end-overlay-shadow sm:w-[90%] @min-[768px]:relative @min-[768px]:inset-auto @min-[768px]:z-auto @min-[768px]:w-[var(--thread-pane-width)] @min-[768px]:shrink-0 @min-[768px]:shadow-none"
            data-testid="thread-pane"
            style:--thread-pane-width={`${threadPaneWidth.value}px`}
          >
            <p class="text-sm text-muted">{m('common.error.network')}</p>
            <div class="flex gap-2">
              <button
                type="button"
                class="btn-secondary"
                onclick={() => (threadPaneLoadAttempt += 1)}
              >
                {m('common.retry')}
              </button>
              <button type="button" class="btn-secondary" onclick={closeThread}>
                {m('room.thread.close')}
              </button>
            </div>
          </div>
        {/await}
      {/if}

      <RoomSidebarPane
        presentation="mobile"
        sidebarProps={mobileRoomSidebarPanel
          ? {
              ...sharedRoomSidebarProps,
              activePanel: mobileRoomSidebarPanel,
              onOpenFile: (messageEventId, threadRootEventId) =>
                openFileMessage(messageEventId, threadRootEventId, true),
              onOpenSearchResult: (messageEventId, threadRootEventId) =>
                openSearchResult(messageEventId, threadRootEventId, true),
              onOpenPin: (messageEventId, threadRootEventId) =>
                openPinnedMessage(messageEventId, threadRootEventId, true),
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
          onOpenSearchResult: openSearchResult,
          onOpenPin: openPinnedMessage,
          onToggleMaximized: toggleDesktopCallWide,
          onClose: closeDesktopRoomSidebarPanel
        }}
      />
    {/if}
  </div>
{/if}
