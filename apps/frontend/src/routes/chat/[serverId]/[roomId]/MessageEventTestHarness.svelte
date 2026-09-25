<script lang="ts">
  import { untrack } from 'svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';
  import { UserStore } from '$lib/state/server/users.svelte';
  import type { TimelineEventView } from '$lib/render/timelineEvents';
  import {
    createComposerContext,
    createMentionRoles,
    createRoomMembers,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import MessageEvent from './MessageEvent.svelte';
  import MessageUserOverlays from './MessageUserOverlays.svelte';
  import { MessageUserInteractionState } from './messageUserInteractions.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    event,
    userStore,
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
    showMessage = true,
    threadingMode = RoomThreadingMode.ENABLED,
    onOpenThread
  }: {
    event: TimelineEventView;
    userStore?: UserStore;
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
    showMessage?: boolean;
    threadingMode?: RoomThreadingMode;
    onOpenThread?: OpenThreadHandler;
  } = $props();

  const connection = {} as ServerConnection;
  const users = untrack(() => userStore ?? new UserStore());
  const store = {
    projection: { users },
    notifications: { hasThreadNotification: () => false },
    readViews: { covers: () => false },
    serverInfo: { messageEditWindowSeconds: 31_536_000, supportsFeature: () => true },
    activeCallRooms: { getParticipantCallPresence: () => null },
    viewerUser: { id: 'viewer', login: 'viewer', settings: undefined },
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
  const roomMembers = createRoomMembers();
  roomMembers.members = [
    {
      id: 'target-user',
      login: 'target',
      displayName: 'Target User',
      presenceStatus: PresenceStatus.OFFLINE
    }
  ];
  const userInteractions = new MessageUserInteractionState(() => roomMembers.members);
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
  provideUserProfiles(() => users);

  const messageStore = {
    ensureEvent: () => undefined,
    getEventById: () => undefined,
    beginOptimisticThreadFollow: () => undefined,
    setThreadRootFollowState: () => undefined
  };
</script>

{#if showMessage}
  <MessageEvent
    {event}
    {roomId}
    {permalinkThreadRootEventId}
    messageStore={messageStore as never}
    {threadingMode}
    {onOpenThread}
    onOpenUser={(user, anchorRect) => userInteractions.showUser(user, anchorRect)}
  />
{/if}

<MessageUserOverlays
  interactions={userInteractions}
  {serverId}
  {roomId}
  currentUserId="viewer"
  canStartDMs={false}
  canBanRoomMembers={false}
  isUniversal={false}
/>

<output data-testid="active-reply-target">
  {composerContext.replyState.messageEventId ?? ''}
</output>
