<script lang="ts">
  import { goto, pushState, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import type { EventEnvelope } from '$lib/eventBus.svelte';
  import { createRoleAPI } from '@chatto/api-client/roles';
  import {
    useRoomData,
    useRoomUnread,
    useEvent,
    usePresenceChange,
    createTypingIndicator
  } from '$lib/hooks';
  import { appState } from '$lib/state/globals.svelte';
  import * as m from '$lib/i18n/messages';
  import {
    createComposerContext,
    createMentionRoles,
    getRoomMembers,
    MessagesStore,
    RoomFilesStore,
    RoomMembersStore,
    setRoomMembersStore,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS,
    type QuoteInsertionContent
  } from '$lib/state/room';
  import { onRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
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
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { isMessagePostedEvent, RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
  import { onDestroy, tick } from 'svelte';
  import { fly } from 'svelte/transition';
  import RoomEventsPane from './RoomEventsPane.svelte';
  import RoomSidebar from './RoomSidebar.svelte';
  import RoomSidebarToggle from './RoomSidebarToggle.svelte';
  import {
    canBanMembersFromRoomSidebar,
    roomSidebarPanelForRoom,
    roomSidebarPanelsForRoom
  } from './roomSidebarBehavior';
  import { RoomSidebarPanelsState } from './roomSidebarPanels.svelte';
  import ThreadPane from './ThreadPane.svelte';

  let { roomId, threadId }: { roomId: string; threadId?: string } = $props();

  const connection = useConnection();
  const roomFilesStore = new RoomFilesStore(connection());
  const roomMembersStore = setRoomMembersStore(new RoomMembersStore(connection()));
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const stores = serverRegistry.getStore(getActiveServer());
  const serverInfo = stores.serverInfo;
  const notificationStore = stores.notifications;

  // Thread navigation functions (URL-driven state)
  let pendingThreadHighlight = $state<string | null>(null);
  let pendingThreadQuote = $state<{ id: number; text: QuoteInsertionContent } | null>(null);
  let pendingThreadQuoteId = 0;

  function openThread(
    threadRootEventId: string,
    highlightEventId?: string,
    quoteText?: QuoteInsertionContent
  ) {
    pendingThreadHighlight = highlightEventId ?? null;
    pendingThreadQuote = quoteText ? { id: ++pendingThreadQuoteId, text: quoteText } : null;
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
  const mentionRoles = createMentionRoles();
  const replyState = composerContext.replyState;
  let replyStateRoomId: string | null = null;
  const jumpState = composerContext.jumpState;
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const roomMessageStore = new MessagesStore(connection(), () => currentUser.user?.id ?? null);

  onDestroy(() => roomMessageStore.dispose());

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
    const conn = connection();
    const api = createRoleAPI({
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
    let cancelled = false;

    async function loadMentionRoles() {
      let roles;
      try {
        roles = (await api.listRoles()).roles;
      } catch {
        if (!cancelled) {
          mentionRoles.clear();
        }
        return;
      }
      if (cancelled) return;
      mentionRoles.setRoles(
        roles
          ?.filter((role) => role.name !== 'everyone')
          .map((role) => ({
            name: role.name,
            isSystem: role.isSystem,
            position: role.position,
            pingable: role.pingable
          })) ?? []
      );
    }

    void loadMentionRoles();
    return () => {
      cancelled = true;
    };
  });

  const unread = useRoomUnread(() => ({ roomId }));

  $effect(() => {
    roomFilesStore.setRoom(roomId);
    roomMembersStore.setRoom(roomId);
  });

  $effect(() => {
    if (room.roomData) {
      roomMembersStore.ensureLoaded();
    }
  });

  // Room permissions — derived reactively, no $effect needed
  let permissions = $derived(room.roomData ?? DEFAULT_ROOM_PERMISSIONS);
  let composerCanAttach = $derived(room.roomData === undefined ? true : permissions.canAttach);

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

  let roomDescription = $derived.by(() => {
    if (!room.roomData || room.isDM) return undefined;

    const description = room.roomData.room.description?.trim();
    return description || undefined;
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
    void (async () => {
      const results = room.isDM
        ? [await notificationStore.dismissDMNotifications(currentRoomId)]
        : await Promise.all([
            notificationStore.dismissMentionNotifications(currentRoomId),
            notificationStore.dismissRoomReplyNotifications(currentRoomId),
            notificationStore.dismissRoomMessageNotifications(currentRoomId)
          ]);

      const dismissedForRoom = results.reduce(
        (sum, counts) => sum + (counts.byRoom[currentRoomId] ?? 0),
        0
      );
      if (dismissedForRoom > 0) {
        stores.rooms.decrementUnreadNotification(currentRoomId, dismissedForRoom);
        void stores.rooms.refreshNotificationCounts();
      }
    })();
  });

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

  function scopedRoomId(event: EventEnvelope['event']): string | null {
    if (!event || !('roomId' in event) || typeof event.roomId !== 'string') return null;
    return event.roomId;
  }

  function shouldRevealAwaySeparator(event: EventEnvelope): boolean {
    const eventData = event.event;
    if (!eventData) return false;
    if (event.actorId === currentUser.user?.id) return false;

    switch (roomEventKind(eventData)) {
      case RoomEventKind.MessagePosted:
        if (!isMessagePostedEvent(eventData)) return false;
        return (
          eventData.roomId === roomId && (!!eventData.echoOfEventId || !eventData.threadRootEventId)
        );
      case RoomEventKind.UserJoinedRoom:
      case RoomEventKind.UserLeftRoom:
      case RoomEventKind.RoomUpdated:
      case RoomEventKind.RoomDeleted:
      case RoomEventKind.RoomArchived:
      case RoomEventKind.RoomUnarchived:
        return scopedRoomId(eventData) === roomId;
      default:
        return false;
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
    roomFilesStore.ingestServerEvent(event);
    roomMembersStore.ingestServerEvent(event);
    if (!event.event) return;

    if (!appState.isPresent && shouldRevealAwaySeparator(event)) {
      unread.noteAwayEvent();
    }

    if (isMessagePostedEvent(event.event) && event.event.roomId === roomId) {
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

  usePresenceChange((userId, status) => {
    roomMembersStore.updatePresence(userId, status);
  });

  // Header action visibility — flat derivations keep the template clean
  let showVoiceCall = $derived(!!room.roomData && !!serverInfo.livekitUrl);
  // Channel rooms can be left unless membership is granted by Universal policy.
  let showLeaveRoom = $derived(!!room.roomData && !room.isDM && !room.roomData.room.isUniversal);
  const roomSidebarPanels = new RoomSidebarPanelsState(
    () => getActiveServer(),
    () => roomId
  );
  const activeRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(room.isDM, roomSidebarPanels.activeDesktopPanel, showVoiceCall)
  );
  const mobileRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(room.isDM, roomSidebarPanels.mobilePanel, showVoiceCall)
  );
  const roomSidebarTogglePanels = $derived(roomSidebarPanelsForRoom(room.isDM, showVoiceCall));
  const hasActiveRoomCall = $derived(stores.activeCallRooms.has(roomId));

  let leavingRoom = $state(false);

  function openRoomSidebarPanel(panel: RoomSidebarPanel): void {
    if (window.matchMedia('(min-width: 1024px)').matches) {
      roomSidebarPanels.openDesktopPanel(panel);
    } else {
      roomSidebarPanels.openMobilePanel(panel);
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
      openThread(threadRootEventId, messageEventId);
    } else {
      void jumpState.jumpToMessage(messageEventId);
    }
    if (closeMobile) {
      roomSidebarPanels.closeMobile();
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
      roomSidebarPanels.closeMobile();
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
      roomSidebarPanels.closeMobile();
      return;
    }

    if (!threadId || e.button !== 0) return;
    const target = e.target as HTMLElement;
    if (target.closest('[data-testid="thread-pane"], dialog')) return;
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
  {#if pageTitle}
    <PageTitle title={pageTitle} />
  {/if}

  <div class="flex min-h-0 min-w-0 flex-1">
    <div class="relative flex min-h-0 min-w-0 flex-1 overflow-hidden">
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

        <PaneHeader {title} subtitle={roomDescription} loading={!room.roomData}>
          {#snippet actions()}
            <RoomSidebarToggle
              mode="mobile"
              activePanel={mobileRoomSidebarPanel}
              panels={roomSidebarTogglePanels}
              hasActiveCall={hasActiveRoomCall}
              onToggle={(panel) => roomSidebarPanels.toggleMobilePanel(panel)}
            />
            <RoomSidebarToggle
              mode="desktop"
              activePanel={activeRoomSidebarPanel}
              panels={roomSidebarTogglePanels}
              hasActiveCall={hasActiveRoomCall}
              onToggle={(panel) => roomSidebarPanels.toggleDesktopPanel(panel)}
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
          unreadAfterTime={unread.unreadAfterTime}
          unreadBeforeTime={unread.unreadBeforeTime}
          onOpenThread={openThread}
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
              if (
                isMessagePostedEvent(event.event) &&
                event.event.roomId === roomId &&
                !event.event.threadRootEventId
              ) {
                unread.noteReadCursor(event.createdAt);
              }
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
          highlightEventId={pendingThreadHighlight}
          pendingQuote={pendingThreadQuote}
          onHighlightComplete={() => {
            pendingThreadHighlight = null;
          }}
          onQuoteConsumed={() => {
            pendingThreadQuote = null;
          }}
        />
      {/if}

      {#if mobileRoomSidebarPanel}
        <button
          type="button"
          class="absolute inset-0 z-10 bg-transparent lg:hidden"
          aria-label={m['room.close_extras']()}
          onclick={() => roomSidebarPanels.closeMobile()}
        ></button>
        <div
          class="absolute inset-y-0 right-0 z-20 flex min-h-0 w-full min-w-0 flex-col overflow-hidden border-l border-border bg-background shadow-[-4px_0_12px_rgba(0,0,0,0.15)] sm:w-[90%] lg:hidden"
          data-testid="room-sidebar-mobile-pane"
          transition:fly={{ x: 300, duration: 200 }}
        >
          <RoomSidebar
            {roomId}
            activePanel={mobileRoomSidebarPanel}
            presentation="overlay"
            loading={room.isRoomLoading}
            filesStore={roomFilesStore}
            livekitUrl={serverInfo.livekitUrl ?? undefined}
            canBanRoomMembers={canBanMembersFromRoomSidebar(
              room.isDM,
              room.roomData?.canBanRoomMembers
            )}
            currentUserId={currentUser.user?.id ?? null}
            membersStore={roomMembersStore}
            onOpenFile={(messageEventId, threadRootEventId) =>
              openFileMessage(messageEventId, threadRootEventId, true)}
            onClose={() => roomSidebarPanels.closeMobile()}
          />
        </div>
      {/if}
    </div>

    {#if activeRoomSidebarPanel}
      <div class="hidden lg:flex">
        <RoomSidebar
          {roomId}
          activePanel={activeRoomSidebarPanel}
          loading={room.isRoomLoading}
          filesStore={roomFilesStore}
          livekitUrl={serverInfo.livekitUrl ?? undefined}
          canBanRoomMembers={canBanMembersFromRoomSidebar(
            room.isDM,
            room.roomData?.canBanRoomMembers
          )}
          currentUserId={currentUser.user?.id ?? null}
          membersStore={roomMembersStore}
          onOpenFile={(messageEventId, threadRootEventId) =>
            openFileMessage(messageEventId, threadRootEventId)}
          onClose={() => roomSidebarPanels.closeDesktop()}
        />
      </div>
    {/if}
  </div>
{/if}
