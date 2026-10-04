<script lang="ts">
  import { setServerUiForTests } from '$lib/state/server/serverUi';
  import { untrack } from 'svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { ServerConnection } from '@chatto/client/server/serverConnection';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import { UserStore } from '@chatto/client/server/users';
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import {
    createComposerContext,
    createMentionRoles,
    createRoomMembers,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import MessageEvent from './MessageEvent.svelte';
  import MessageUserOverlays from './MessageUserOverlays.svelte';
  import { MessageUserInteractionState } from './messageUserInteractions.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { setTimelineRowPins, type TimelineRowPins } from './timelineRowPins';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

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
    onOpenThread,
    rowPins
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
    /** Pins of the enclosing timeline, as `EventList` provides them. */
    rowPins?: TimelineRowPins;
  } = $props();

  const pins = untrack(() => rowPins);
  if (pins) setTimelineRowPins(pins);

  const connection = {} as ServerConnection;
  const users = untrack(() => userStore ?? new UserStore());
  const store = {
    projection: { users },
    notifications: { hasThreadNotification: () => false },
    readViews: { covers: () => false },
    serverInfo: { messageEditWindowSeconds: 31_536_000 },
    viewerUser: { id: 'viewer', login: 'viewer', settings: undefined },
    viewerId: 'viewer',
    permissions: { canStartDMs: false },
    rooms: {
      pins: () => ({
        isPinned: (_messageEventId: string, hydratedStatus = false) => pinStatus ?? hydratedStatus,
        create: async () => undefined,
        remove: async () => undefined
      })
    }
  } as unknown as ServerStateStore;
  setServerUiForTests(store, { activeCallRooms: { getParticipantCallPresence: () => null } });

  provideServerScope({
    get serverId() {
      return serverId;
    },
    connection,
    store,
    isCurrent: () => true
  });
  const composerContext = createComposerContext();
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
<output data-testid="active-reply-excerpt">{composerContext.replyState.excerpt}</output>
