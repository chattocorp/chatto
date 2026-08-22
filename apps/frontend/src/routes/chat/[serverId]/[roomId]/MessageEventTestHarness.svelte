<script lang="ts">
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';
  import type { TimelineEventView } from '$lib/render/timelineEvents';
  import {
    createComposerContext,
    createMentionRoles,
    createRoomMembers,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import MessageEvent from './MessageEvent.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    event,
    roomId = 'room-1',
    serverId = 'remote-server',
    permalinkThreadRootEventId = null,
    canReact = true,
    canPostMessage = true,
    canPostInThread = true,
    canManageOthersMessage = false,
    canViewPinnedMessages = false,
    canPinMessages = false,
    pinStatus = null,
    threadingMode = RoomThreadingMode.ENABLED,
    onOpenThread
  }: {
    event: TimelineEventView;
    roomId?: string;
    serverId?: string;
    permalinkThreadRootEventId?: string | null;
    canReact?: boolean;
    canPostMessage?: boolean;
    canPostInThread?: boolean;
    canManageOthersMessage?: boolean;
    canViewPinnedMessages?: boolean;
    canPinMessages?: boolean;
    pinStatus?: boolean | null;
    threadingMode?: RoomThreadingMode;
    onOpenThread?: OpenThreadHandler;
  } = $props();

  const connection = {} as ServerConnection;
  const store = {
    notifications: { hasThreadNotification: () => false },
    serverInfo: { messageEditWindowSeconds: 31_536_000 },
    activeCallRooms: { getParticipantCallPresence: () => null },
    currentUser: {
      user: { id: 'viewer', login: 'viewer', settings: undefined }
    },
    permissions: { canStartDMs: false },
    pinsForRoom: () => ({
      isPinned: (_messageEventId: string, hydratedStatus = false) => pinStatus ?? hydratedStatus,
      create: async () => undefined,
      remove: async () => undefined
    })
  } as unknown as ServerStateStore;

  provideServerScope({
    get serverId() {
      return serverId;
    },
    connection,
    store,
    isCurrent: () => true
  });
  const composerContext = createComposerContext({ scroll: true });
  createMentionRoles();
  createRoomMembers();
  createRoomPermissions(() => ({
    ...DEFAULT_ROOM_PERMISSIONS,
    canPostMessage,
    canPostInThread,
    canReact,
    canManageOthersMessage,
    canEchoMessage: true,
    canViewPinnedMessages,
    canPinMessages
  }));
  createPresenceCache();
  createUserProfileCache();

  const messageStore = {
    ensureEvent: () => undefined,
    getEventById: () => undefined,
    beginOptimisticThreadFollow: () => undefined,
    setThreadRootFollowState: () => undefined
  };
</script>

<MessageEvent
  {event}
  {roomId}
  {permalinkThreadRootEventId}
  messageStore={messageStore as never}
  {threadingMode}
  {onOpenThread}
/>

<output data-testid="active-reply-target">
  {composerContext.replyState.messageEventId ?? ''}
</output>
