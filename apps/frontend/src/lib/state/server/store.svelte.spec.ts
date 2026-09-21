import { resetUserStoresForTests } from './users.svelte';
import { userProfileFixture } from '$lib/test-utils/userProfile';
import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { AuthenticatedServerState } from '$lib/api-client/serverState';
import type { RoomFileItem } from '$lib/api-client/attachments';
import { createRoomTimelineAPI } from '$lib/api-client/roomTimeline';
import type { MessageResource } from '$lib/api-client/messageResources';
import { Message, MessageAttachment } from '@chatto/api-types/api/v1/message_types_pb';
import { messageToTimelineEvent } from '$lib/api-client/roomTimeline';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { GetMotdResponse } from '@chatto/api-types/api/v1/server_state_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { ListNotificationOccurrencesResponse } from '@chatto/api-types/api/v1/notifications_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { Room, PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import {
  ListRoomsResponse,
  ListRoomGroupsResponse,
  RoomGroup,
  RoomGroupViewerState,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import {
  GetViewerResponse,
  PrivilegedModeState,
  ServerViewerPermissions,
  ViewerCapabilities,
  ViewerUser
} from '@chatto/api-types/api/v1/viewer_pb';
import { CapabilityGrant, PermissionGrant } from '@chatto/api-types/api/v1/permissions_pb';
import {
  RealtimeResourceUpdate,
  type RealtimeResourceFamily
} from '$lib/api-client/realtimeResources';
import { ListUsersResponse } from '@chatto/api-types/api/v1/user_service_pb';
import {
  AssetProcessingStartedEvent,
  AssetProcessingSucceededEvent,
  AssetProcessingFailedEvent,
  AssetDeletedEvent,
  VoiceCallParticipantJoinedEvent,
  RoomThreadingModeChangedEvent,
  UserJoinedRoomEvent,
  UserLeftRoomEvent,
  MessagePostedEvent,
  MessageEditedEvent,
  ReactionAddedEvent,
  UserAccountDeletedEvent,
  UserProfileChangedEvent,
  PresenceChangedEvent,
  NotificationUnreadStateChangedEvent,
  NotificationOccurrencesChangedEvent,
  ThreadViewerStateChangedEvent
} from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';

const { soundMocks, apiMocks, cacheMocks } = vi.hoisted(() => ({
  soundMocks: {
    playCallSound: vi.fn(() => Promise.resolve())
  },
  cacheMocks: {
    reconcileRegisteredAdminRoomGroupQueries: vi.fn(),
    reconcileRegisteredAdminRoomQueries: vi.fn(),
    refreshRegisteredAdminQueries: vi.fn(),
    refreshRegisteredServerQueries: vi.fn(async () => {}),
    removeRegisteredAdminQueries: vi.fn(),
    removeRegisteredAdminUserQueries: vi.fn(),
    removeRegisteredServerQueries: vi.fn(),
    resetFollowedThreads: vi.fn(),
    refreshFollowedThreads: vi.fn(),
    reconcileFollowedThreads: vi.fn(),
    scrubFollowedThreadRoom: vi.fn(),
    scrubFollowedThreadMessage: vi.fn(),
    scrubFollowedThreadUser: vi.fn(),
    updateFollowedThreadSummary: vi.fn(),
    invalidateRoomMemberQueries: vi.fn(),
    purgeRoomMemberQueries: vi.fn(),
    scrubRoomMemberUser: vi.fn()
  },
  apiMocks: {
    listPins: vi.fn(),
    readMessages: vi.fn<(roomId: string, ids: string[], cursor?: string) => Promise<MessageResource[]>>(() => Promise.resolve([])),
    readRealtimeResource: vi.fn<
      (family: RealtimeResourceFamily, cursor?: string) => Promise<RealtimeResourceUpdate[]>
    >(() => Promise.resolve([])),
    readRealtimeUsers: vi.fn<
      (userIds: Iterable<string>, cursor?: string) => Promise<RealtimeResourceUpdate[]>
    >(() => Promise.resolve([])),
    listRooms: vi.fn(() => Promise.resolve([])),
    listRoomGroups: vi.fn(() => Promise.resolve([])),
    listRoomMembers: vi.fn(() =>
      Promise.resolve({
        members: [],
        totalCount: 0,
        hasMore: false
      })
    ),
    joinCall: vi.fn(() => Promise.resolve(true)),
    createCallToken: vi.fn(() => Promise.resolve(null)),
    leaveCall: vi.fn(() => Promise.resolve(true)),
    activatePrivilegedMode: vi.fn(() =>
      Promise.resolve({
        privilegedMode: new PrivilegedModeState({ available: true, active: true }),
        capabilities: new ViewerCapabilities(),
        viewerPermissions: new ServerViewerPermissions()
      })
    ),
    deactivatePrivilegedMode: vi.fn(() =>
      Promise.resolve({
        privilegedMode: new PrivilegedModeState({ available: true, active: false }),
        capabilities: new ViewerCapabilities(),
        viewerPermissions: new ServerViewerPermissions()
      })
    ),
    refreshPrivilegedMode: vi.fn(() => Promise.resolve(new GetViewerResponse())),
    getAuthenticatedServerState: vi.fn<() => Promise<AuthenticatedServerState>>(() =>
      Promise.resolve({
        name: 'Store Event Test',
        version: 'test',
        logoUrl: null,
        bannerUrl: null,
        welcomeMessage: null,
        description: null,
        motd: null,
        pushNotificationsEnabled: false,
        vapidPublicKey: null,
        livekitUrl: null,
        videoProcessingEnabled: false,
        maxUploadSize: 25,
        maxVideoUploadSize: 25,
        messageEditWindowSeconds: 3600,
        viewerPermissions: {},
        viewerCanManageServer: false,
        viewerCanCreateRooms: false,
        viewerCanJoinRooms: false,
        viewerCanListRooms: false,
        viewerCanManageRooms: false,
        viewerCanBanRoomMembers: false,
        viewerCanPostMessages: false,
        viewerCanPostInThreads: false,
        viewerCanAttachFiles: false,
        viewerCanManageMessages: false,
        viewerCanReactToMessages: false,
        viewerCanEchoMessages: false,
        viewerCanManageRoles: false,
        viewerCanAssignRoles: false,
        viewerCanViewAdminUsers: false,
        viewerCanViewAdminSystem: false,
        viewerCanViewAdminAudit: false,
        viewerCanDeleteAnyUser: false,
        viewerCanDeleteSelf: false,
        viewerCanManageUserPermissions: false,
        viewerHasUnreadRooms: false
      })
    ),
    getViewerStateViaConnect: vi.fn(() =>
      Promise.resolve({
        user: {
          id: 'U1',
          login: 'alice',
          displayName: 'Alice',
          avatarUrl: null,
          customStatus: null,
          presenceStatus: 'ONLINE',
          hasVerifiedEmail: true,
          viewerCanDeleteAccount: true,
          lastLoginChange: null,
          settings: null
        },
        canViewAdmin: false,
        canStartDMs: true,
        canAdminViewUsers: false,
        canAdminManageAccounts: false,
        canAssignRoles: false,
        canAdminViewRoles: false,
        canAdminManageRoles: false,
        canAdminViewSystem: false,
        canAdminViewAudit: false,
        canManageUserPermissions: false
      })
    ),
    getCurrentUserViaConnect: vi.fn(() =>
      Promise.resolve({
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        avatarUrl: null,
        customStatus: null,
        presenceStatus: 'ONLINE',
        hasVerifiedEmail: true,
        viewerCanDeleteAccount: true,
        lastLoginChange: null,
        settings: null
      })
    ),
    listRoomAttachments: vi.fn<
      () => Promise<{ items: RoomFileItem[]; totalCount: number; hasMore: boolean }>
    >(() => Promise.resolve({ items: [], totalCount: 0, hasMore: false })),
    refreshAssetUrls: vi.fn(() => Promise.resolve(new Map())),
    listRoles: vi.fn(() =>
      Promise.resolve({
        roles: [],
        viewerCanManageRoles: false,
        viewerCanAssignRoles: false
      })
    )
  }
}));

vi.mock('$lib/audio/callSounds', () => ({
  playCallSound: soundMocks.playCallSound
}));

vi.mock('$lib/api-client/roomDirectory', async (importActual) => {
  const actual = await importActual<typeof import('$lib/api-client/roomDirectory')>();
  return {
    RoomDirectoryScope: {
      ALL: 1
    },
    RoomKind: {
      CHANNEL: 1,
      DM: 2
    },
    mapDirectoryRoom: (room: unknown) => room,
    mapRoomGroup: actual.mapRoomGroup,
    createRoomDirectoryAPI: vi.fn(() => ({
      listRooms: apiMocks.listRooms,
      listRoomGroups: apiMocks.listRoomGroups
    }))
  };
});

vi.mock('$lib/api-client/memberDirectory', async (importOriginal) => ({
  mapDirectoryMember: (await importOriginal<typeof import('$lib/api-client/memberDirectory')>()).mapDirectoryMember,
  createMemberDirectoryAPI: vi.fn(() => ({
    listRoomMembers: apiMocks.listRoomMembers
  }))
}));

vi.mock('$lib/api-client/voiceCalls', () => ({
  createVoiceCallAPI: vi.fn(() => ({
    joinCall: apiMocks.joinCall,
    createCallToken: apiMocks.createCallToken,
    leaveCall: apiMocks.leaveCall
  }))
}));

vi.mock('$lib/api-client/notifications', async (importActual) => {
  const actual = await importActual<typeof import('$lib/api-client/notifications')>();
  return {
    ...actual,
    createNotificationAPI: vi.fn(() => ({
      listNotificationOccurrences: vi.fn(() =>
        Promise.resolve({
          occurrences: [],
          totalCount: 0,
          hasMore: false,
          unreadCount: 0,
          importantUnreadCount: 0,
          roomUnreadCounts: {},
          roomImportantUnreadCounts: {}
        })
      ),
      markNotificationRead: vi.fn(),
      deleteNotificationOccurrence: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(() => Promise.resolve([])),
      updateNotificationPolicy: vi.fn(() => Promise.resolve([]))
    }))
  };
});

vi.mock('$lib/api-client/roles', () => ({
  createRoleAPI: vi.fn(() => ({
    listRoles: apiMocks.listRoles
  }))
}));

vi.mock('$lib/api-client/realtimeResources', async (importActual) => {
  const actual = await importActual<typeof import('$lib/api-client/realtimeResources')>();
  return {
    ...actual,
    createRealtimeResourceAPI: vi.fn(() => ({
      read: apiMocks.readRealtimeResource,
      readUsers: apiMocks.readRealtimeUsers
    }))
  };
});

vi.mock('$lib/api-client/messageResources', () => ({
  createMessageResourcesAPI: () => ({ read: apiMocks.readMessages })
}));

vi.mock('$lib/api-client/pinnedMessages', () => ({
  createPinnedMessagesAPI: () => ({ list: apiMocks.listPins, create: vi.fn(), remove: vi.fn() })
}));

vi.mock('$lib/api-client/roomTimeline', async (importActual) => {
  const actual = await importActual<typeof import('$lib/api-client/roomTimeline')>();
  const emptyPage = {
    events: [],
    includes: { users: {}, rooms: {} },
    startCursor: null,
    endCursor: null,
    hasOlder: false,
    hasNewer: false
  };
  return {
    ...actual,
    createRoomTimelineAPI: vi.fn(() => ({
      getRoomEvents: vi.fn(() => Promise.resolve(emptyPage)),
      getRoomEventsAround: vi.fn(() => Promise.resolve(emptyPage)),
      getMessage: vi.fn(() => Promise.resolve(null)),
      getThreadEvents: vi.fn(() => Promise.resolve(emptyPage)),
      getThreadEventsAround: vi.fn(() => Promise.resolve(emptyPage))
    }))
  };
});

vi.mock('$lib/api-client/serverState', () => ({
  getAuthenticatedServerState: apiMocks.getAuthenticatedServerState
}));

vi.mock('$lib/api-client/viewer', async (importActual) => {
  const actual = await importActual<typeof import('$lib/api-client/viewer')>();
  return {
    ...actual,
    createPrivilegedModeAPI: vi.fn(() => ({
      activate: apiMocks.activatePrivilegedMode,
      deactivate: apiMocks.deactivatePrivilegedMode,
      refresh: apiMocks.refreshPrivilegedMode
    })),
    getViewerStateViaConnect: apiMocks.getViewerStateViaConnect,
    getCurrentUserViaConnect: apiMocks.getCurrentUserViaConnect
  };
});

vi.mock('$lib/api-client/attachments', async (importActual) => {
  const actual = await importActual<typeof import('$lib/api-client/attachments')>();
  return {
    ...actual,
    createAttachmentAPI: vi.fn(() => ({
      listRoomAttachments: apiMocks.listRoomAttachments,
      refreshAssetUrls: apiMocks.refreshAssetUrls
    }))
  };
});

import { ServerStateStore } from './store.svelte';
import { eventBusManager, setRealtimeSocketFactoryForTests } from './eventBus.svelte';
import {
  registerFollowedThreadQueryCache,
  registerRoomMemberQueryCache,
  registerServerQueryCache
} from '$lib/query/cacheRegistry';
import type { ServerConnection } from './serverConnection.svelte';
import type { RegisteredServer } from './registry.svelte';

class FakeServerConnection {
  invalidatePrivateData = vi.fn();
  serverId = 'store-event-test';
  connectBaseUrl = 'https://store-event.test';
  reconnectCount = $state(0);
  realtimeUrl = 'ws://store-event.test/api/realtime';
  bearerToken: string | null = 'remote-token';
  setRealtimeConnectionStatus = vi.fn();
  registerRealtimeReconnect = vi.fn(() => () => {});
  handleAuthenticationRequired = vi.fn();
  forceReconnect = vi.fn();
  query = vi.fn();
  results: unknown[];

  constructor(results: unknown[]) {
    this.results = results;
    this.query.mockImplementation(() => {
      const data = this.results.shift() ?? null;
      return {
        toPromise: vi.fn().mockResolvedValue({ data, error: null })
      };
    });
  }

  getAPI<T>(factory: (config: never) => T): T {
    return factory({} as never);
  }
}

const registered: RegisteredServer = {
  id: 'store-event-test',
  url: 'https://store-event.test',
  name: 'Store Event Test',
  iconUrl: null,
  token: 'remote-token',
  userId: 'U1',
  userLogin: 'alice',
  userDisplayName: 'Alice',
  userAvatarUrl: null,
  reauthRequiredAt: null,
  addedAt: 1
};

const stores: ServerStateStore[] = [];

function connectUnavailable() {
  return vi
    .fn<(baseUrl: string) => Promise<PublicServerInfo>>()
    .mockRejectedValue(new Error('connect unavailable'));
}

function makeStore(
  fake: FakeServerConnection,
  server: RegisteredServer = registered,
  publicServerInfoLoader = connectUnavailable(),
  onAuthenticationRequired?: () => void
): ServerStateStore {
  const store = new ServerStateStore(
    {
      id: server.id,
      url: server.url,
      name: server.name,
      iconUrl: server.iconUrl,
      addedAt: server.addedAt
    },
    () => ({
      token: server.token,
      userId: server.userId,
      userLogin: server.userLogin,
      userDisplayName: server.userDisplayName,
      userAvatarUrl: server.userAvatarUrl,
      reauthRequiredAt: server.reauthRequiredAt
    }),
    false,
    fake as unknown as ServerConnection,
    publicServerInfoLoader,
    onAuthenticationRequired
  );
  stores.push(store);
  return store;
}

async function flushPromises(times = 5): Promise<void> {
  for (let i = 0; i < times; i++) {
    await Promise.resolve();
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function roomResource(rooms: RoomWithViewerState[]): RealtimeResourceUpdate {
  return new RealtimeResourceUpdate({
    resource: { case: 'rooms', value: new ListRoomsResponse({ rooms }) }
  });
}

function userDeleted(userId: string): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    event: new RealtimeEvent({
      event: { case: 'userAccountDeleted', value: new UserAccountDeletedEvent({ userId }) }
    })
  });
}

function userLeftRoom(roomId: string, actorId: string, eventId = ''): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    event: new RealtimeEvent({
      id: eventId,
      actorId,
      event: { case: 'userLeftRoom', value: new UserLeftRoomEvent({ roomId }) }
    })
  });
}

beforeEach(() => {
  apiMocks.listPins.mockReset().mockResolvedValue({ items: [], totalCount: 0, hasMore: false, latestPinMarker: '' });
  apiMocks.readMessages.mockReset().mockResolvedValue([]);
  resetUserStoresForTests();
  registerServerQueryCache({
    server: cacheMocks.removeRegisteredServerQueries,
    refreshServer: cacheMocks.refreshRegisteredServerQueries,
    admin: cacheMocks.removeRegisteredAdminQueries,
    refreshAdmin: cacheMocks.refreshRegisteredAdminQueries,
    adminUser: cacheMocks.removeRegisteredAdminUserQueries,
    adminRoom: cacheMocks.reconcileRegisteredAdminRoomQueries,
    adminRoomGroups: cacheMocks.reconcileRegisteredAdminRoomGroupQueries
  });
  registerFollowedThreadQueryCache({
    reset: cacheMocks.resetFollowedThreads,
    refresh: cacheMocks.refreshFollowedThreads,
    reconcile: cacheMocks.reconcileFollowedThreads,
    scrubRoom: cacheMocks.scrubFollowedThreadRoom,
    scrubMessage: cacheMocks.scrubFollowedThreadMessage,
    scrubUser: cacheMocks.scrubFollowedThreadUser,
    updateSummary: cacheMocks.updateFollowedThreadSummary
  });
  registerRoomMemberQueryCache({
    invalidateRoom: cacheMocks.invalidateRoomMemberQueries,
    purgeRoom: cacheMocks.purgeRoomMemberQueries,
    scrubUser: cacheMocks.scrubRoomMemberUser
  });
  cacheMocks.resetFollowedThreads.mockClear();
  cacheMocks.refreshFollowedThreads.mockClear();
  cacheMocks.reconcileFollowedThreads.mockClear();
  cacheMocks.scrubFollowedThreadRoom.mockClear();
  cacheMocks.scrubFollowedThreadMessage.mockClear();
  cacheMocks.scrubFollowedThreadUser.mockClear();
  cacheMocks.updateFollowedThreadSummary.mockClear();
  cacheMocks.invalidateRoomMemberQueries.mockClear();
  cacheMocks.purgeRoomMemberQueries.mockClear();
  cacheMocks.scrubRoomMemberUser.mockClear();
  cacheMocks.reconcileRegisteredAdminRoomQueries.mockClear();
  cacheMocks.reconcileRegisteredAdminRoomGroupQueries.mockClear();
  cacheMocks.removeRegisteredServerQueries.mockClear();
  cacheMocks.refreshRegisteredAdminQueries.mockClear();
  cacheMocks.refreshRegisteredServerQueries.mockClear();
  cacheMocks.removeRegisteredAdminQueries.mockClear();
  cacheMocks.removeRegisteredAdminUserQueries.mockClear();
  apiMocks.listRooms.mockResolvedValue([]);
  apiMocks.listRoomGroups.mockResolvedValue([]);
  apiMocks.listRoomMembers.mockResolvedValue({
    members: [],
    totalCount: 0,
    hasMore: false
  });
  apiMocks.readRealtimeResource.mockReset();
  apiMocks.readRealtimeResource.mockResolvedValue([]);
  apiMocks.readRealtimeUsers.mockReset();
  apiMocks.readRealtimeUsers.mockResolvedValue([]);
  apiMocks.listRoomAttachments.mockReset();
  apiMocks.listRoomAttachments.mockResolvedValue({ items: [], totalCount: 0, hasMore: false });
  apiMocks.refreshAssetUrls.mockReset();
  apiMocks.refreshAssetUrls.mockResolvedValue(new Map());
  apiMocks.joinCall.mockResolvedValue(true);
  apiMocks.createCallToken.mockResolvedValue(null);
  apiMocks.leaveCall.mockResolvedValue(true);
  apiMocks.activatePrivilegedMode.mockReset();
  apiMocks.activatePrivilegedMode.mockResolvedValue({
    privilegedMode: new PrivilegedModeState({ available: true, active: true }),
    capabilities: new ViewerCapabilities(),
    viewerPermissions: new ServerViewerPermissions()
  });
  apiMocks.deactivatePrivilegedMode.mockReset();
  apiMocks.deactivatePrivilegedMode.mockResolvedValue({
    privilegedMode: new PrivilegedModeState({ available: true, active: false }),
    capabilities: new ViewerCapabilities(),
    viewerPermissions: new ServerViewerPermissions()
  });
  apiMocks.refreshPrivilegedMode.mockReset();
  apiMocks.refreshPrivilegedMode.mockResolvedValue(new GetViewerResponse());
  apiMocks.getAuthenticatedServerState.mockResolvedValue({
    name: 'Store Event Test',
    version: 'test',
    logoUrl: null,
    bannerUrl: null,
    welcomeMessage: null,
    description: null,
    motd: null,
    pushNotificationsEnabled: false,
    vapidPublicKey: null,
    livekitUrl: null,
    videoProcessingEnabled: false,
    maxUploadSize: 25,
    maxVideoUploadSize: 25,
    messageEditWindowSeconds: 3600,
    viewerPermissions: {},
    viewerCanManageServer: false,
    viewerCanCreateRooms: false,
    viewerCanJoinRooms: false,
    viewerCanListRooms: false,
    viewerCanManageRooms: false,
    viewerCanBanRoomMembers: false,
    viewerCanPostMessages: false,
    viewerCanPostInThreads: false,
    viewerCanAttachFiles: false,
    viewerCanManageMessages: false,
    viewerCanReactToMessages: false,
    viewerCanEchoMessages: false,
    viewerCanManageRoles: false,
    viewerCanAssignRoles: false,
    viewerCanViewAdminUsers: false,
    viewerCanViewAdminSystem: false,
    viewerCanViewAdminAudit: false,
    viewerCanDeleteAnyUser: false,
    viewerCanDeleteSelf: false,
    viewerCanManageUserPermissions: false,
    viewerHasUnreadRooms: false
  });
  apiMocks.getViewerStateViaConnect.mockResolvedValue({
    user: {
      id: 'U1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      customStatus: null,
      presenceStatus: 'ONLINE',
      hasVerifiedEmail: true,
      viewerCanDeleteAccount: true,
      lastLoginChange: null,
      settings: null
    },
    canViewAdmin: false,
    canStartDMs: true,
    canAdminViewUsers: false,
    canAdminManageAccounts: false,
    canAssignRoles: false,
    canAdminViewRoles: false,
    canAdminManageRoles: false,
    canAdminViewSystem: false,
    canAdminViewAudit: false,
    canManageUserPermissions: false
  });
  apiMocks.getCurrentUserViaConnect.mockResolvedValue({
    id: 'U1',
    login: 'alice',
    displayName: 'Alice',
    avatarUrl: null,
    customStatus: null,
    presenceStatus: 'ONLINE',
    hasVerifiedEmail: true,
    viewerCanDeleteAccount: true,
    lastLoginChange: null,
    settings: null
  });
  setRealtimeSocketFactoryForTests(() => ({
    binaryType: 'arraybuffer',
    readyState: 0,
    onopen: null,
    onmessage: null,
    onerror: null,
    onclose: null,
    send: vi.fn(),
    close: vi.fn()
  }));
});

afterEach(() => {
  for (const store of stores.splice(0)) {
    store.dispose();
  }
  eventBusManager.stopBus(registered.id);
  setRealtimeSocketFactoryForTests(null);
  soundMocks.playCallSound.mockClear();
  vi.restoreAllMocks();
});

describe('ServerStateStore privileged mode', () => {
  it('reads snapshots with current permissions while reconnect is still pending', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      privilegedMode: new PrivilegedModeState({ available: true, active: false })
    });
    const readPermissions = vi.fn();
    cacheMocks.refreshRegisteredAdminQueries.mockImplementationOnce(() => {
      readPermissions(store.projection.viewer?.privilegedMode?.active);
    });
    const changing = store.setPrivilegedMode(true);
    await vi.waitFor(() => expect(fake.forceReconnect).toHaveBeenCalled());
    expect(store.realtimeSync.authorizationRefreshRequired).toBe(true);
    expect(readPermissions).toHaveBeenCalledWith(true);

    store.realtimeSync.markCaughtUp(
      'cursor-after',
      store.realtimeSync.pendingAuthorizationRefreshGeneration
    );
    await changing;

    expect(store.realtimeSync.authorizationRefreshRequired).toBe(false);
  });

  it('applies the activation result and refreshes realtime projections', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      privilegedMode: new PrivilegedModeState({ available: true, active: false })
    });
    store.setPermissions({ canAdminViewSystem: false } as never);
    store.realtimeSync.markCaughtUp('cursor-before');
    apiMocks.activatePrivilegedMode.mockResolvedValueOnce({
      privilegedMode: new PrivilegedModeState({ available: true, active: true }),
      capabilities: new ViewerCapabilities({
        grants: [new CapabilityGrant({ capability: 'admin.view-system', granted: true })]
      }),
      viewerPermissions: new ServerViewerPermissions()
    });
    fake.forceReconnect.mockImplementationOnce(() =>
      store.realtimeSync.markCaughtUp(
        'cursor-after',
        store.realtimeSync.pendingAuthorizationRefreshGeneration
      )
    );

    await store.setPrivilegedMode(true);

    expect(apiMocks.activatePrivilegedMode).toHaveBeenCalledOnce();
    expect(store.projection.viewer?.privilegedMode?.active).toBe(true);
    expect(store.permissions.canAdminViewSystem).toBe(true);
    expect(store.realtimeSync.resumeCursor).toBe('cursor-after');
    expect(store.realtimeSync.authorizationRefreshRequired).toBe(false);
    expect(fake.forceReconnect).toHaveBeenCalledWith('privileged mode changed');
    expect(cacheMocks.refreshRegisteredAdminQueries).toHaveBeenCalledWith(registered.id);
  });

  it('applies deactivation permissions before completing the projection refresh', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      capabilities: new ViewerCapabilities({
        grants: [new CapabilityGrant({ capability: 'admin.view-system', granted: true })]
      }),
      privilegedMode: new PrivilegedModeState({ available: true, active: true })
    });
    store.setPermissions({ canAdminViewSystem: true } as never);
    store.realtimeSync.markCaughtUp('cursor-before');
    apiMocks.deactivatePrivilegedMode.mockResolvedValueOnce({
      privilegedMode: new PrivilegedModeState({ available: true, active: false }),
      capabilities: new ViewerCapabilities({
        grants: [new CapabilityGrant({ capability: 'admin.view-system', granted: false })]
      }),
      viewerPermissions: new ServerViewerPermissions()
    });
    fake.forceReconnect.mockImplementationOnce(() =>
      store.realtimeSync.markCaughtUp(
        'cursor-after',
        store.realtimeSync.pendingAuthorizationRefreshGeneration
      )
    );

    await store.setPrivilegedMode(false);

    expect(store.projection.viewer?.privilegedMode?.active).toBe(false);
    expect(store.permissions.canAdminViewSystem).toBe(false);
    expect(store.realtimeSync.resumeCursor).toBe('cursor-after');
    expect(store.realtimeSync.authorizationRefreshRequired).toBe(false);
    expect(fake.forceReconnect).toHaveBeenCalledWith('privileged mode changed');
    expect(cacheMocks.removeRegisteredAdminQueries).toHaveBeenCalledWith(registered.id);
  });

  it('refreshes navigation group permissions on activation and deactivation without a layout event', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      privilegedMode: new PrivilegedModeState({ available: true, active: false })
    });
    store.realtimeSync.markCaughtUp('cursor-before');
    const group = (granted: boolean) =>
      new RoomGroup({
        id: 'G1',
        name: 'Lobby',
        viewerState: new RoomGroupViewerState({
          permissions: [
            new PermissionGrant({ permission: 'room.create', granted }),
            new PermissionGrant({ permission: 'room.manage', granted })
          ]
        })
      });
    store.projection.roomGroups = [group(false)];
    apiMocks.readRealtimeResource.mockImplementation(async (family) =>
      family === 'roomGroups'
        ? [
            new RealtimeResourceUpdate({
              resource: {
                case: 'roomGroups',
                value: new ListRoomGroupsResponse({
                  groups: [group(store.projection.viewer?.privilegedMode?.active ?? false)]
                })
              }
            })
          ]
        : []
    );
    fake.forceReconnect.mockImplementation(() => {
      const generation = store.realtimeSync.pendingAuthorizationRefreshGeneration;
      void store
        .completeRealtimeCatchUp('cursor-after')
        .then(() => store.realtimeSync.markCaughtUp('cursor-after', generation));
    });

    for (const active of [true, false]) {
      cacheMocks.refreshRegisteredAdminQueries.mockClear();
      await store.setPrivilegedMode(active);
      expect(cacheMocks.refreshRegisteredAdminQueries).toHaveBeenCalledWith(registered.id);
      expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('roomGroups', 'cursor-after');
      expect(store.navigation.roomGroups).toMatchObject([
        {
          id: 'G1',
          viewerCanCreateRoom: active,
          viewerCanManageGroup: active
        }
      ]);
    }
  });

  it('clears expired activation and refreshes effective permissions', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      privilegedMode: new PrivilegedModeState({ available: true, active: true })
    });
    store.setPermissions({ canAdminViewSystem: true } as never);
    apiMocks.refreshPrivilegedMode.mockResolvedValueOnce({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      privilegedMode: new PrivilegedModeState({ available: true, active: false }),
      capabilities: new ViewerCapabilities({
        grants: [new CapabilityGrant({ capability: 'admin.view-system', granted: false })]
      })
    } as GetViewerResponse);

    await store.expirePrivilegedMode();

    expect(store.projection.viewer?.privilegedMode?.available).toBe(true);
    expect(store.projection.viewer?.privilegedMode?.active).toBe(false);
    expect(store.permissions.canAdminViewSystem).toBe(false);
    expect(apiMocks.refreshPrivilegedMode).toHaveBeenCalledOnce();
    expect(fake.forceReconnect).toHaveBeenCalledWith('privileged mode expired');
    expect(cacheMocks.removeRegisteredAdminQueries).toHaveBeenCalledWith(registered.id);
  });

  it('refreshes expired room-scoped grants without a server capability change', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      privilegedMode: new PrivilegedModeState({ available: true, active: true })
    });
    apiMocks.refreshPrivilegedMode.mockResolvedValueOnce(
      new GetViewerResponse({
        user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
        privilegedMode: new PrivilegedModeState({ available: true, active: false })
      })
    );

    await store.expirePrivilegedMode();

    expect(cacheMocks.refreshRegisteredAdminQueries).toHaveBeenCalledWith(registered.id);
  });

  it('rechecks admin reads and reconnects when the expiry viewer refresh fails', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) }),
      privilegedMode: new PrivilegedModeState({ available: true, active: true })
    });
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
    apiMocks.refreshPrivilegedMode.mockRejectedValueOnce(new Error('viewer unavailable'));

    await store.expirePrivilegedMode();

    expect(store.projection.viewer?.privilegedMode?.active).toBe(false);
    expect(cacheMocks.refreshRegisteredAdminQueries).toHaveBeenCalledWith(registered.id);
    expect(fake.forceReconnect).toHaveBeenCalledWith('privileged mode expired');
    expect(warning).toHaveBeenCalled();
  });
});

describe('ServerStateStore authentication state', () => {
  it('treats reauth-required servers as unauthenticated without clearing user data', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake, {
      ...registered,
      reauthRequiredAt: 123
    });
    store.currentUser.user = {
      id: 'U1',
      login: 'alice',
      displayName: 'Alice'
    } as typeof store.currentUser.user;

    expect(store.isAuthenticated).toBe(false);
    expect(store.currentUser.user).toMatchObject({ id: 'U1' });
  });
});

describe('ServerStateStore room search state', () => {
  it('keeps presence current in inactive and newly opened rooms', () => {
    const store = makeStore(new FakeServerConnection([]));
    const a = store.membersForRoom('a');
    store.membersForRoom('b');
    store.realtimePresenceHandler(
      new RealtimeEvent({
        actorId: 'U2',
        event: { case: 'presenceChanged', value: new PresenceChangedEvent({ status: 2 }) }
      })
    );
    expect(a.livePresence.get('U2')).toBe(2);
    expect(store.membersForRoom('c').livePresence.get('U2')).toBe(2);
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true }));
    expect(a.livePresence.has('U2')).toBe(false);
    expect(store.membersForRoom('d').livePresence.has('U2')).toBe(false);
  });
  it('retains member lists across A to B to A navigation and clears them on reset', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const a = store.membersForRoom('a');
    await a.loadInitial();
    await store.membersForRoom('b').loadInitial();
    const requests = apiMocks.listRoomMembers.mock.calls.length;
    expect(store.membersForRoom('a')).toBe(a);
    store.membersForRoom('a').ensureLoaded();
    await flushPromises();
    expect(apiMocks.listRoomMembers).toHaveBeenCalledTimes(requests);
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true }));
    expect(a.hasFirstPage).toBe(false);
    a.ensureLoaded();
    await flushPromises();
    expect(apiMocks.listRoomMembers).toHaveBeenCalledTimes(requests + 1);
  });

  it('applies a leave to an inactive retained room without relisting', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const a = store.membersForRoom('a');
    a.replaceProjection('a', [{ id: 'U2', login: 'two', displayName: 'Two', presenceStatus: 1 }]);
    await store.membersForRoom('b').loadInitial();
    const requests = apiMocks.listRoomMembers.mock.calls.length;
    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          id: 'leave-a',
          actorId: 'U2',
          event: { case: 'userLeftRoom', value: new UserLeftRoomEvent({ roomId: 'a' }) }
        })
      })
    );
    expect(a.members).toEqual([]);
    expect(a.totalCount).toBe(0);
    expect(apiMocks.listRoomMembers).toHaveBeenCalledTimes(requests);
  });
  it('retains separate transient search state for each room', () => {
    const store = makeStore(new FakeServerConnection([]));
    const firstRoomSearch = store.messageSearchForRoom('R1');
    const secondRoomSearch = store.messageSearchForRoom('R2');

    firstRoomSearch.query = 'first room only';

    expect(store.messageSearchForRoom('R1')).toBe(firstRoomSearch);
    expect(secondRoomSearch).not.toBe(firstRoomSearch);
    expect(secondRoomSearch.query).toBe('');
    expect(store.messageSearch.query).toBe('');
  });

  it('bounds retained room search plaintext', () => {
    const store = makeStore(new FakeServerConnection([]));
    const oldestSearch = store.messageSearchForRoom('R1');
    oldestSearch.query = 'sensitive result scope';
    for (let index = 2; index <= 11; index++) store.messageSearchForRoom(`R${index}`);

    expect(oldestSearch.query).toBe('');
    expect(store.messageSearchForRoom('R1')).not.toBe(oldestSearch);
  });
});

describe('ServerStateStore unified realtime resources', () => {
  it.each([true, false])('retains a call during reset and checks fresh join access: %s', (join) => {
    const store = makeStore(new FakeServerConnection([]));
    store.voiceCall.roomId = 'R1';
    store.voiceCall.connected = true;
    const revoked = vi.spyOn(store.voiceCall, 'handleRoomAccessRevoked');
    const reconcile = vi.spyOn(store.voiceCall, 'reconcilePermissions').mockResolvedValue();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({ reset: true, privacyReset: true })
    );
    expect(store.voiceCall.connected).toBe(true);
    expect(store.voiceCall.roomId).toBe('R1');
    expect(store.voiceCall.canUseVoice).toBe(false);
    expect(revoked).not.toHaveBeenCalled();
    expect(reconcile).not.toHaveBeenCalled();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        resource: roomResource([
          new RoomWithViewerState({
            room: new Room({ id: 'R1' }),
            viewerState: new RoomViewerState({
              isMember: true,
              permissions: [new PermissionGrant({ permission: 'call.join', granted: join })]
            })
          })
        ])
      })
    );
    expect(reconcile).toHaveBeenCalledOnce();
    expect(store.voiceCall.permissionsFor('R1').join).toBe(join);
  });

  it.each(['unchanged owner', 'unchanged member', 'revoked', 'room revoked', 'read narrowed', 'posting granted', 'posting revoked', 'failed', 'privilege expired', 'reset while pending'])(
    'reconciles permission changes without resetting the view: %s', async (change) => {
      const fake = new FakeServerConnection([]);
      const store = makeStore(fake);
      const viewer = new GetViewerResponse({
        user: { profile: { id: 'U1' } },
        privilegedMode: { active: true },
        capabilities: { grants: [{ capability: 'admin.view-system', granted: change === 'unchanged owner' }] },
        viewerPermissions: { permissions: [{ permission: 'role.manage', granted: true }] }
      });
      store.projection.viewer = viewer;
      store.projection.rooms.set('R1', new RoomWithViewerState({
        room: { id: 'R1' }, viewerState: { isMember: true, permissions: [
          { permission: 'message.read', granted: true },
          { permission: 'message.post-in-interactions', granted: change === 'posting revoked' }
        ] }
      }));
      store.realtimeSync.markCaughtUp('retained');
      const resetMessages = vi.spyOn(store.messagesForRoom('R1'), 'resetProjectionState');
      const revokeMessages = vi.spyOn(store.messagesForRoom('R1'), 'clearForAccessRevocation');
      const revokeCall = vi.spyOn(store.voiceCall, 'handleRoomAccessRevoked');
      let release!: () => void;
      const gate = new Promise<void>((resolve) => { release = resolve; });
      apiMocks.readRealtimeResource.mockImplementation(async (family) => {
        await gate;
        if (change === 'failed') throw new Error('offline');
        if (family === 'viewer') return [new RealtimeResourceUpdate({ resource: {
          case: 'viewer', value: change === 'revoked' ? new GetViewerResponse({ user: viewer.user }) :
            change === 'privilege expired' ? new GetViewerResponse({ ...viewer, privilegedMode: { active: false } }) : viewer
        } })];
        if (family === 'rooms') return [roomResource(change === 'room revoked' ? [] : [
          new RoomWithViewerState({ room: { id: 'R1' }, viewerState: { isMember: true,
            permissions: [
              { permission: 'message.read', granted: change !== 'read narrowed' },
              { permission: 'message.post-in-interactions', granted: change === 'posting granted' }
            ]
          } })
        ])];
        if (family === 'roomGroups') return [new RealtimeResourceUpdate({ resource: {
          case: 'roomGroups', value: new ListRoomGroupsResponse()
        } })];
        return [];
      });
      store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
        cursor: 'permission-event',
        event: new RealtimeEvent({ event: {
          case: 'rolePermissionsChanged', value: { roleName: 'everyone' }
        } })
      }));
      expect(store.checkingPermissions).toBe(true);
      expect(store.projection.viewer).toBe(viewer);
      expect(resetMessages).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('viewer', 'permission-event'));
      if (change === 'reset while pending') {
        store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true, privacyReset: true }));
      }
      release();
      if (change === 'reset while pending') await store.waitForRealtimeReconciliation();
      await vi.waitFor(() => expect(store.checkingPermissions).toBe(false));
      expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('viewer', 'permission-event');
      expect(fake.forceReconnect).not.toHaveBeenCalled();
      expect(store.realtimeSync.resumeCursor).toBe('retained');
      expect(store.realtimeSync.hasUsableProjection).toBe(true);
      expect(cacheMocks.refreshRegisteredServerQueries).toHaveBeenCalled();
      if (change !== 'reset while pending') {
        expect(fake.invalidatePrivateData).not.toHaveBeenCalled();
        expect(resetMessages).not.toHaveBeenCalled();
      } else {
        expect(fake.invalidatePrivateData).toHaveBeenCalledOnce();
        expect(resetMessages).toHaveBeenCalledOnce();
        expect(store.projection.viewer).toBeNull();
      }
      if (['room revoked', 'read narrowed', 'posting granted', 'posting revoked', 'failed'].includes(change)) {
        expect(revokeMessages).toHaveBeenCalled();
      } else expect(revokeMessages).not.toHaveBeenCalled();
      if (change === 'failed') {
        expect(store.projection.viewer).toBeNull();
        await expect(store.waitForRealtimeReconciliation()).rejects.toThrow('offline');
      }
      if (change === 'read narrowed') expect(revokeCall).not.toHaveBeenCalled();
    }
  );

  it('drains queued resource reads before replacing their authority', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const releases: Array<() => void> = [];
    apiMocks.readRealtimeResource.mockImplementation(async (family, cursor) => {
      if (family === 'rooms' && cursor !== 'permission') {
        await new Promise<void>((resolve) => { releases.push(resolve); });
      }
      return [];
    });
    for (const cursor of ['first', 'queued']) {
      store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ cursor, event: new RealtimeEvent({
        event: { case: 'roomReadStateChanged', value: { roomId: 'R1' } }
      }) }));
      if (cursor === 'first') await vi.waitFor(() => expect(releases).toHaveLength(1));
    }
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ cursor: 'permission', event: new RealtimeEvent({
      event: { case: 'viewerPermissionsChanged', value: {} }
    }) }));
    releases[0]();
    await vi.waitFor(() => expect(releases).toHaveLength(2));
    expect(apiMocks.readRealtimeResource).not.toHaveBeenCalledWith('viewer', 'permission');
    releases[1]();
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('viewer', 'permission');
  });

  it('restores the same room store after a failed authority read is retried', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const room = new RoomWithViewerState({ room: { id: 'R1' }, viewerState: {
      isMember: true, permissions: [{ permission: 'message.read', granted: true }]
    } });
    store.projection.rooms.set('R1', room);
    store.realtimeSync.markCaughtUp('retained');
    const messages = store.messagesForRoom('R1');
    const hydrate = vi.spyOn(messages, 'hydrateRealtimeProjection').mockResolvedValue(true);
    let fail = true;
    apiMocks.readRealtimeResource.mockImplementation(async (family) => {
      if (family !== 'rooms') return [];
      if (fail) throw new Error('room offline');
      return [roomResource([room])];
    });
    const event = new RealtimeProjectionUpdate({ cursor: 'retry', event: new RealtimeEvent({
      event: { case: 'viewerPermissionsChanged', value: {} }
    }) });
    store.realtimeProjectionHandler(event);
    await expect(store.waitForRealtimeReconciliation()).rejects.toThrow('room offline');
    expect(store.projection.rooms.has('R1')).toBe(false);
    fail = false;
    store.realtimeProjectionHandler(event);
    await store.waitForRealtimeReconciliation();
    expect(store.messagesForRoom('R1')).toBe(messages);
    expect(store.projection.rooms.get('R1')).toBe(room);
    expect(hydrate).toHaveBeenCalledWith('retry', expect.any(Function));
    expect(fake.forceReconnect).not.toHaveBeenCalled();
    expect(store.realtimeSync.hasUsableProjection).toBe(true);
  });

  it.each(['deactivate', 'dispose'])('fences a pending permission check on %s', async (action) => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.projection.viewer = new GetViewerResponse({ user: { profile: { id: 'U1' } },
      privilegedMode: { active: true } });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    apiMocks.readRealtimeResource.mockImplementation(async (family) => {
      await gate;
      return family === 'viewer' ? [new RealtimeResourceUpdate({ resource: { case: 'viewer',
        value: new GetViewerResponse({ user: { profile: { id: 'U1' } }, privilegedMode: { active: true } })
      } })] : [];
    });
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ event: new RealtimeEvent({
      event: { case: 'viewerPermissionsChanged', value: {} }
    }) }));
    await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalled());
    if (action === 'deactivate') {
      fake.forceReconnect.mockImplementationOnce(() => store.realtimeSync.markCaughtUp(
        'fresh', store.realtimeSync.pendingAuthorizationRefreshGeneration
      ));
      await store.setPrivilegedMode(false);
    } else store.dispose();
    const viewer = store.projection.viewer;
    release();
    await store.waitForRealtimeReconciliation();
    expect(store.projection.viewer).toBe(viewer);
    expect(store.checkingPermissions).toBe(false);
  });

  it('ignores stale authority responses after a newer check finishes', async () => {
    const store = makeStore(new FakeServerConnection([]));
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    apiMocks.readRealtimeResource.mockImplementation(async (family, cursor) => {
      if (cursor === 'older') await gate;
      if (family !== 'viewer') return [];
      return [new RealtimeResourceUpdate({ resource: { case: 'viewer', value: new GetViewerResponse({
        user: { profile: { id: 'U1' } },
        viewerPermissions: { permissions: [{ permission: 'role.manage', granted: cursor === 'older' }] }
      }) } })];
    });
    const change = (cursor: string) => store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor, event: new RealtimeEvent({ event: { case: 'viewerPermissionsChanged', value: {} } })
    }));
    change('older');
    await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('viewer', 'older'));
    change('newer');
    await vi.waitFor(() => expect(store.checkingPermissions).toBe(false));
    release();
    await store.waitForRealtimeReconciliation();
    expect(store.projection.viewer?.viewerPermissions?.permissions[0].granted).toBe(false);
  });

  it('retains every role change when a newer permission check supersedes an older check', async () => {
    const store = makeStore(new FakeServerConnection([]));
    store.projection.users.set('U1', new DirectoryMember({ user: { id: 'U1' }, roles: ['first', 'second'] }));
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    apiMocks.readRealtimeResource.mockImplementation(async () => { await gate; return []; });
    for (const roleName of ['first', 'second']) {
      store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
        cursor: roleName,
        event: new RealtimeEvent({ event: { case: 'roleDeleted', value: { roleName } } })
      }));
    }
    release();
    await store.waitForRealtimeReconciliation();
    expect(store.projection.users.get('U1')?.roles).toEqual([]);
    expect(store.checkingPermissions).toBe(false);
  });

  it('reauthorizes independent caches even when one authority read fails', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const search = vi.spyOn(store.messageSearch, 'refreshPermissions');
    const members = vi.spyOn(store.membersForRoom('R1'), 'refresh').mockResolvedValue();
    apiMocks.readRealtimeResource.mockImplementation(async (family) => {
      if (family === 'viewer') throw new Error('viewer offline');
      return [];
    });
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'permission-event',
      event: new RealtimeEvent({ event: {
        case: 'rolePermissionsChanged', value: { roleName: 'everyone' }
      } })
    }));
    await expect(store.waitForRealtimeReconciliation()).rejects.toThrow('viewer offline');
    expect(search).toHaveBeenCalledOnce();
    expect(members).toHaveBeenCalledWith({ reauthorize: true });
    expect(store.checkingPermissions).toBe(false);
  });

  it('keeps its cursor and messages when another user receives a role', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.realtimeSync.markCaughtUp('retained');
    const resetMessages = vi.spyOn(store.messagesForRoom('room'), 'resetProjectionState');
    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          event: { case: 'roleAssigned', value: { userId: 'U2', roleName: 'helper' } }
        })
      })
    );
    expect(fake.invalidatePrivateData).not.toHaveBeenCalled();
    expect(resetMessages).not.toHaveBeenCalled();
    expect(store.realtimeSync.resumeCursor).toBe('retained');
  });
  it('applies canonical room resources without a realtime-specific room shape', () => {
    const store = makeStore(new FakeServerConnection([]));

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        resource: roomResource([
          new RoomWithViewerState({
            room: new Room({ id: 'R1', name: 'General' }),
            viewerState: new RoomViewerState({ isMember: true }),
            memberUserIds: ['U1', 'U2'],
            hasMessageHistory: true
          })
        ])
      })
    );

    expect(store.projection.rooms.get('R1')).toMatchObject({
      memberUserIds: ['U1', 'U2'],
      hasMessageHistory: true
    });
  });

  it('applies the canonical public server resource', () => {
    const store = makeStore(new FakeServerConnection([]));

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        resource: new RealtimeResourceUpdate({
          resource: {
            case: 'server',
            value: new ServerPublicProfile({ name: 'Canonical Server', version: '0.5.0' })
          }
        })
      })
    );

    expect(store.serverInfo.name).toBe('Canonical Server');
    expect(store.serverInfo.version).toBe('0.5.0');
  });

  it('scrubs a removed user before its authoritative resource refresh completes', () => {
    const store = makeStore(new FakeServerConnection([]));
    store.projection.users.set(
      'U2',
      new DirectoryMember({ user: new User({ id: 'U2', login: 'bob' }) })
    );

    store.realtimeProjectionHandler(userDeleted('U2'));

    expect(store.projection.users.has('U2')).toBe(false);
    expect(cacheMocks.scrubRoomMemberUser).toHaveBeenCalledWith(store.serverId, 'U2');
  });

  it('coalesces the resource hints from a post before starting reads', async () => {
    const store = makeStore(new FakeServerConnection([]));
    store.messagesForRoom('R1');
    await flushPromises(20);
    apiMocks.readRealtimeResource.mockClear();
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'z-post', event: new RealtimeEvent({ id: 'POST', event: {
        case: 'messagePosted', value: { roomId: 'R1' }
      } })
    }));
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'n-notification', event: new RealtimeEvent({ event: {
        case: 'notificationUnreadStateChanged', value: { roomId: 'R1' }
      } })
    }));
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'a-read', event: new RealtimeEvent({ event: {
        case: 'roomReadStateChanged', value: { roomId: 'R1' }
      } })
    }));
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource.mock.calls).toEqual([
      ['rooms', 'a-read']
    ]);
    expect(apiMocks.readMessages).toHaveBeenCalledExactlyOnceWith('R1', ['POST'], 'z-post');
    expect(apiMocks.readRealtimeUsers.mock.calls.every(([ids]) => [...ids].length === 0)).toBe(true);
  });

  it.each(['', 'ROOT'])('does not reload attention for a warm self-post in scope "%s"', async (threadRootEventId) => {
    const store = makeStore(new FakeServerConnection([]));
    store.projection.rooms.set('R1', new RoomWithViewerState({
      room: { id: 'R1' }, viewerState: { isMember: true, hasUnread: false }
    }));
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      event: new RealtimeEvent({ id: 'POST', actorId: 'U1', event: {
        case: 'messagePosted', value: { roomId: 'R1', threadRootEventId }
      } })
    }));
    for (const hint of ['notificationUnreadStateChanged', 'roomReadStateChanged'] as const) {
      store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
        event: new RealtimeEvent({ actorId: 'U1', event: { case: hint, value: { roomId: 'R1' } } })
      }));
    }
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource).not.toHaveBeenCalled();
  });

  it.each(['unread', 'unknown', 'slow mode', 'other actor', 'unknown actor'])(
    'retains room reads for an unread hint with %s state', async (state) => {
      const store = makeStore(new FakeServerConnection([]));
      if (state !== 'unknown') store.projection.rooms.set('R1', new RoomWithViewerState({
        room: { id: 'R1', slowModeSeconds: state === 'slow mode' ? 30 : 0 },
        viewerState: { isMember: true, hasUnread: state === 'unread' }
      }));
      store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          actorId: state === 'other actor' ? 'U2' : state === 'unknown actor' ? '' : 'U1',
          event: { case: 'notificationUnreadStateChanged', value: { roomId: 'R1' } }
        })
      }));
      await store.waitForRealtimeReconciliation();
      expect(apiMocks.readRealtimeResource.mock.calls).toEqual([['rooms', undefined]]);
    }
  );

  it('coalesces occurrence changes without reading rooms or filtering by actor', async () => {
    const store = makeStore(new FakeServerConnection([]));
    for (const actorId of ['U1', 'U2', '']) {
      store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
        event: new RealtimeEvent({ actorId, event: {
          case: 'notificationOccurrencesChanged', value: {}
        } })
      }));
    }
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource.mock.calls).toEqual([['notifications', undefined]]);
  });

  it('follows an in-flight room read when a self-read hint arrives', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const row = (hasUnread: boolean) => new RoomWithViewerState({
      room: { id: 'R1' }, viewerState: { isMember: true, hasUnread }
    });
    store.projection.rooms.set('R1', row(false));
    const stale = deferred<RealtimeResourceUpdate[]>();
    apiMocks.readRealtimeResource.mockReturnValueOnce(stale.promise)
      .mockResolvedValueOnce([roomResource([row(false)])]);
    const hint = (actorId: string) => new RealtimeProjectionUpdate({
      event: new RealtimeEvent({ actorId, event: {
        case: 'notificationUnreadStateChanged', value: { roomId: 'R1' }
      } })
    });
    store.realtimeProjectionHandler(hint('U2'));
    await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalledOnce());
    store.realtimeProjectionHandler(hint('U1'));
    stale.resolve([roomResource([row(true)])]);
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource.mock.calls).toEqual([['rooms', undefined], ['rooms', undefined]]);
    expect(store.projection.rooms.get('R1')?.viewerState?.hasUnread).toBe(false);
  });

  it.each([false, true])('thread-read recovery only reloads known attention when unread=%s', async (unread) => {
    const store = makeStore(new FakeServerConnection([]));
    store.projection.rooms.set('R1', new RoomWithViewerState({
      room: { id: 'R1' }, viewerState: { isMember: true, hasUnread: unread }
    }));
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ resource: new RealtimeResourceUpdate({
      resource: { case: 'notifications', value: new ListNotificationOccurrencesResponse({
        unreadCount: unread ? 1 : 0,
        roomUnreadCounts: unread ? [{ roomId: 'R1', unreadCount: 1 }] : []
      }) }
    }) }));
    store.reconcileThreadRead('R1', 'ROOT');
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource.mock.calls).toEqual(unread
      ? [['notifications', undefined], ['rooms', undefined]] : []);
  });

  it('invalidates a cached author as soon as a profile change arrives', async () => {
    const store = makeStore(new FakeServerConnection([]));
    store.projection.users.set('U2', userProfileFixture(
      { id: 'U2', login: 'old', displayName: 'old', deleted: false, avatarUrl: null }
    ));
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      event: new RealtimeEvent({ event: { case: 'userProfileChanged', value: { userId: 'U2' } } })
    }));
    expect(store.projection.users.has('U2')).toBe(false);
    await store.waitForRealtimeReconciliation();
  });

  it('clears cached authors when the server store is disposed', () => {
    const store = makeStore(new FakeServerConnection([]));
    store.projection.users.set('U2', userProfileFixture(
      { id: 'U2', login: 'cached', displayName: 'cached', deleted: false, avatarUrl: null }
    ));
    store.dispose();
    expect(store.projection.users.has('U2')).toBe(false);
  });

  it.each(['reset', 'dispose'])('does not send queued resource reads after %s', async (boundary) => {
    const store = makeStore(new FakeServerConnection([]));
    const changed = (cursor: string) => store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor, event: new RealtimeEvent({ event: { case: 'roomReadStateChanged', value: { roomId: 'R1' } } })
    }));
    changed('old');
    if (boundary === 'reset') {
      store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true, privacyReset: true }));
      changed('new');
    } else store.dispose();
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource.mock.calls).toEqual(boundary === 'reset' ? [['rooms', 'new']] : []);
  });

  it('runs one follow-up read when the same resource changes during an active refresh', async () => {
    const first = deferred<RealtimeResourceUpdate[]>();
    apiMocks.readRealtimeUsers.mockReturnValueOnce(first.promise).mockResolvedValueOnce([]);
    const store = makeStore(new FakeServerConnection([]));

    for (const userId of ['U2', 'U3']) {
      store.realtimeProjectionHandler(
        new RealtimeProjectionUpdate({
          event: new RealtimeEvent({
            event: {
              case: 'userProfileChanged',
              value: new UserProfileChangedEvent({ userId })
            }
          })
        })
      );
    }

    expect(apiMocks.readRealtimeUsers).toHaveBeenCalledTimes(1);
    expect(cacheMocks.refreshRegisteredAdminQueries).toHaveBeenCalledWith(store.serverId);
    first.resolve([]);
    await flushPromises();

    expect(apiMocks.readRealtimeUsers).toHaveBeenCalledTimes(2);
    expect(apiMocks.readRealtimeUsers).toHaveBeenNthCalledWith(2, ['U3'], undefined);
  });

  it.each(['profile', 'dm', 'catch-up'] as const)(
    'does not restore a deleted profile from a late %s read or publish it to consumers',
    async (path) => {
      const response = deferred<RealtimeResourceUpdate[]>();
      apiMocks.readRealtimeUsers.mockReturnValueOnce(response.promise);
      const fake = new FakeServerConnection([]);
      const store = makeStore(fake);
      eventBusManager.ensureBus(
        store.serverId,
        fake as unknown as ServerConnection,
        true,
        store.realtimeSync,
        store.realtimeProjectionHandler
      );
      const observer = vi.fn<(update: RealtimeProjectionUpdate) => void>();
      eventBusManager.getBus(store.serverId)!.projectionHandlers.add(observer);
      const deleted = new DirectoryMember({
        user: new User({ id: 'U2', displayName: 'Old profile' })
      });
      const retained = new DirectoryMember({ user: new User({ id: 'U3' }) });
      let completion: Promise<void> | undefined;
      if (path === 'catch-up') {
        store.projection.users.set('U2', deleted);
        completion = store.completeRealtimeCatchUp('before-deletion');
      } else if (path === 'dm') {
        apiMocks.readRealtimeResource.mockResolvedValueOnce([
          roomResource([
            new RoomWithViewerState({ room: new Room({ id: 'DM1' }), memberUserIds: ['U2', 'U3'] })
          ])
        ]);
        store.realtimeProjectionHandler(userLeftRoom('R1', 'U3'));
      } else {
        store.realtimeProjectionHandler(
          new RealtimeProjectionUpdate({
            event: new RealtimeEvent({
              event: {
                case: 'userProfileChanged',
                value: new UserProfileChangedEvent({ userId: 'U2' })
              }
            })
          })
        );
      }
      await vi.waitFor(() => expect(apiMocks.readRealtimeUsers).toHaveBeenCalledTimes(1));
      store.realtimeProjectionHandler(userDeleted('U2'));
      response.resolve([
        new RealtimeResourceUpdate({
          resource: { case: 'users', value: { users: [deleted, retained] } },
          replace: false
        })
      ]);
      await completion;
      await flushPromises(20);
      expect(store.projection.users.has('U2')).toBe(false);
      expect(store.projection.users.get('U3')).toEqual(retained);
      const publishedUsers = observer.mock.calls.flatMap(([update]) =>
        update.resource?.case === 'users' ? update.resource.value.users : []
      );
      expect(publishedUsers.map((member) => member.user?.id)).toEqual(['U3']);
      expect(deleted.user?.displayName).toBe('Old profile');
    }
  );

  it.each(['room', 'thread'] as const)(
    'batches edits and reactions across the loaded %s window',
    async (scope) => {
      const store = makeStore(new FakeServerConnection([]));
      const messages =
        scope === 'room' ? store.messagesForRoom('R1') : store.messagesForThread('R1', 'ROOT');
      await flushPromises(20);
      const row = (id: number, updated = false): TimelineEventView => ({
        id: `M${id}`,
        createdAt: new Date(Date.UTC(2026, 0, 1, 0, 0, id)).toISOString(),
        event: {
          kind: TimelineEventKind.MessagePosted,
          roomId: 'R1',
          threadRootEventId: scope === 'thread' ? 'ROOT' : null,
          body: updated ? 'edited' : 'original',
          attachments: [],
          replyCount: 0,
          threadParticipants: [],
          reactions: updated ? [{ emoji: 'ok', count: 1, hasReacted: false, users: [] }] : []
        }
      });
      for (let id = 1; id <= 140; id++) messages.ingestEvent(row(id));
      const api: ReturnType<typeof createRoomTimelineAPI> = vi
        .mocked(createRoomTimelineAPI)
        .mock.results.at(-1)!.value;
      const read = vi.mocked(scope === 'room' ? api.getRoomEventsAround : api.getThreadEventsAround);
      apiMocks.readMessages.mockImplementation(async (_roomId, ids) => ids.map((id) => ({
        message: new Message({ id, roomId: 'R1' }),
        timeline: id === 'ROOT' ? null : row(Number(id.slice(1)), true)
      })));
      for (const id of [1, 70, 140]) {
        store.realtimeProjectionHandler(
          new RealtimeProjectionUpdate({
            cursor: `cursor-${id}`,
            event: new RealtimeEvent({
              event:
                id === 140
                  ? {
                      case: 'reactionAdded',
                      value: new ReactionAddedEvent({ roomId: 'R1', messageEventId: `M${id}` })
                    }
                  : {
                      case: 'messageEdited',
                      value: new MessageEditedEvent({ roomId: 'R1', messageEventId: `M${id}` })
                    }
            })
          })
        );
      }
      await store.waitForRealtimeReconciliation();
      expect(read).not.toHaveBeenCalled();
      expect(apiMocks.readMessages).toHaveBeenCalledExactlyOnceWith(
        'R1', scope === 'room' ? ['M1', 'M70', 'M140'] : ['M1', 'ROOT', 'M70', 'M140'], 'cursor-140'
      );
      expect(messages.getEventById('M70')?.event).toMatchObject({ body: 'edited' });
      expect(messages.getEventById('M140')?.event).toMatchObject({
        reactions: [{ emoji: 'ok', count: 1 }]
      });
      expect(messages.events).toHaveLength(140);
    }
  );

  it('converges after join, leave, and join overlap one room resource read', async () => {
    const firstRooms = deferred<RealtimeResourceUpdate[]>();
    const finalRooms = roomResource([
      new RoomWithViewerState({
        room: new Room({ id: 'R1' }),
        memberUserIds: ['U1', 'U2']
      })
    ]);
    let roomReads = 0;
    apiMocks.readRealtimeResource.mockImplementation((family) => {
      if (family !== 'rooms') return Promise.resolve([]);
      roomReads++;
      return roomReads === 1 ? firstRooms.promise : Promise.resolve([finalRooms]);
    });
    const store = makeStore(new FakeServerConnection([]));
    const membership = (joined: boolean, cursor: string) =>
      new RealtimeProjectionUpdate({
        cursor,
        event: joined
          ? new RealtimeEvent({
              actorId: 'U2',
              event: {
                case: 'userJoinedRoom',
                value: new UserJoinedRoomEvent({ roomId: 'R1' })
              }
            })
          : new RealtimeEvent({
              actorId: 'U2',
              event: {
                case: 'userLeftRoom',
                value: new UserLeftRoomEvent({ roomId: 'R1' })
              }
            })
      });

    store.realtimeProjectionHandler(membership(true, 'cursor-join-1'));
    await vi.waitFor(() => expect(roomReads).toBe(1));
    store.realtimeProjectionHandler(membership(false, 'cursor-leave'));
    store.realtimeProjectionHandler(membership(true, 'cursor-join-2'));
    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          event: {
            case: 'notificationUnreadStateChanged',
            value: new NotificationUnreadStateChangedEvent({ roomId: 'R1' })
          }
        })
      })
    );
    firstRooms.resolve([]);
    await store.completeRealtimeCatchUp('cursor-final');

    const roomCalls = apiMocks.readRealtimeResource.mock.calls.filter(
      ([family]) => family === 'rooms'
    );
    expect(roomCalls).toEqual([
      ['rooms', 'cursor-join-1'],
      ['rooms', 'cursor-final'],
      ['rooms', 'cursor-join-2']
    ]);
    expect(store.projection.rooms.get('R1')?.memberUserIds).toEqual(['U1', 'U2']);
  });

  it('discards resource responses from a superseded reset generation', async () => {
    const oldState = deferred<RealtimeResourceUpdate[]>();
    const stateResource = (motd: string) =>
      new RealtimeResourceUpdate({
        resource: { case: 'motd', value: new GetMotdResponse({ motd }) }
      });
    apiMocks.readRealtimeResource.mockImplementation((family, cursor) => {
      if (family !== 'serverState') return Promise.resolve([]);
      return cursor === 'cursor-old'
        ? oldState.promise
        : Promise.resolve([stateResource('Current MOTD')]);
    });
    const store = makeStore(new FakeServerConnection([]));

    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true }));
    const obsoleteCompletion = store.completeRealtimeCatchUp('cursor-old');
    await flushPromises();
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true }));
    await store.completeRealtimeCatchUp('cursor-current');
    expect(store.projection.serverState?.motd).toBe('Current MOTD');

    oldState.resolve([stateResource('Obsolete MOTD')]);
    await expect(obsoleteCompletion).rejects.toThrow('superseded by a newer reset');
    expect(store.projection.serverState?.motd).toBe('Current MOTD');
  });

  it('reconciles latest-value resources and snapshot timelines at catch-up', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const timelineRead = deferred<boolean>();
    const hydrate = vi
      .spyOn(messages, 'hydrateRealtimeProjection')
      .mockReturnValue(timelineRead.promise);
    await flushPromises();
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true }));
    store.projection.users.set(
      'U2',
      new DirectoryMember({ user: new User({ id: 'U2', login: 'bob' }) })
    );

    const bootstrap = store.completeRealtimeCatchUp('opaque-reset-cursor');
    await flushPromises();
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith(
      'serverState',
      'opaque-reset-cursor'
    );
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('viewer', 'opaque-reset-cursor');
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('rooms', 'opaque-reset-cursor');
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith(
      'notifications',
      'opaque-reset-cursor'
    );
    const [catchUpUserIds, catchUpUserCursor] = apiMocks.readRealtimeUsers.mock.calls[0];
    expect([...catchUpUserIds]).toEqual(['U2', 'U1']);
    expect(catchUpUserCursor).toBe('opaque-reset-cursor');
    expect(hydrate).toHaveBeenCalledWith('opaque-reset-cursor', expect.any(Function));

    let completed = false;
    void bootstrap.then(() => {
      completed = true;
    });
    await flushPromises();
    expect(completed).toBe(false);

    timelineRead.resolve(true);
    await bootstrap;
  });

  it('does not replace mounted timelines after an ordinary resume', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const hydrate = vi.spyOn(messages, 'hydrateRealtimeProjection');
    await flushPromises();

    await store.completeRealtimeCatchUp('opaque-resume-cursor');

    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith(
      'notifications',
      'opaque-resume-cursor'
    );
    expect(hydrate).not.toHaveBeenCalled();
  });

  it('lets sound observers wait for queued notification reads without consuming cursor errors', async () => {
    const store = makeStore(new FakeServerConnection([]));
    await flushPromises();
    const first = deferred<RealtimeResourceUpdate[]>();
    const second = deferred<RealtimeResourceUpdate[]>();
    apiMocks.readRealtimeResource
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    const changed = () =>
      store.realtimeProjectionHandler(
        new RealtimeProjectionUpdate({
          event: new RealtimeEvent({
            event: {
              case: 'notificationOccurrencesChanged',
              value: new NotificationOccurrencesChangedEvent({ createdNotificationId: 'N1' })
            }
          })
        })
      );
    changed();
    await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalledTimes(1));
    const observed = store.waitForRealtimeResourceRefresh('notifications');
    let completed = false;
    void observed.then(() => {
      completed = true;
    });
    changed();
    first.resolve([]);
    await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalledTimes(2));
    expect(completed).toBe(false);
    const failure = new Error('notification read failed');
    second.reject(failure);
    await expect(observed).resolves.toBe(false);
    await expect(store.waitForRealtimeReconciliation()).rejects.toBe(failure);
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledTimes(2);
  });

  it('does not complete a durable cursor when message hydration fails', async () => {
    const messageRead = deferred<MessageResource[]>();
    const store = makeStore(new FakeServerConnection([]));
    store.messagesForRoom('R1');
    apiMocks.readMessages.mockReturnValueOnce(messageRead.promise);
    await flushPromises();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        cursor: 'opaque-message-cursor',
        event: new RealtimeEvent({
          id: 'E-POST',
          event: {
            case: 'messagePosted',
            value: new MessagePostedEvent({ roomId: 'R1', bodyPlaintext: 'new body' })
          }
        })
      })
    );
    await vi.waitFor(() => expect(apiMocks.readMessages).toHaveBeenCalledWith('R1', ['E-POST'], 'opaque-message-cursor'));

    const completion = store.waitForRealtimeReconciliation();
    let completed = false;
    void completion.then(
      () => {
        completed = true;
      },
      () => undefined
    );
    await flushPromises();
    expect(completed).toBe(false);

    const failure = new Error('message read failed');
    messageRead.reject(failure);
    await expect(completion).rejects.toBe(failure);
  });

  it('waits for cursorless window reads and shared message reads without auxiliary RPCs', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    await flushPromises(20);
    const first = deferred<Awaited<ReturnType<typeof messages.refreshCurrentWindow>>>();
    const last = deferred<MessageResource[]>();
    const result = { hasOlder: false, hasNewer: false, refreshed: true, changed: true };
    const refresh = vi
      .spyOn(messages, 'refreshCurrentWindow')
      .mockResolvedValue(result)
      .mockReturnValueOnce(first.promise);
    store.realtimeProjectionHandler(userLeftRoom('R1', 'U2', 'FIRST'));
    await Promise.all(['rooms', 'roomGroups'].map((family) =>
      store.waitForRealtimeResourceRefresh(family as 'rooms' | 'roomGroups')
    ));
    apiMocks.readRealtimeResource.mockClear();
    apiMocks.readRealtimeUsers.mockClear();
    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        cursor: 'post-cursor',
        event: new RealtimeEvent({
          id: 'POST',
          event: {
            case: 'messagePosted',
            value: new MessagePostedEvent({ roomId: 'R1' })
          }
        })
      })
    );
    const edit = new RealtimeProjectionUpdate({
      cursor: 'edit-cursor',
      event: new RealtimeEvent({
        event: {
          case: 'messageEdited',
          value: new MessageEditedEvent({ roomId: 'R1', messageEventId: 'POST' })
        }
      })
    });
    store.realtimeProjectionHandler(edit);
    store.realtimeProjectionHandler(edit);
    apiMocks.readMessages.mockReturnValueOnce(last.promise);
    await store.waitForRealtimeResourceRefresh('rooms');
    apiMocks.readRealtimeResource.mockClear();
    apiMocks.readRealtimeUsers.mockClear();
    const complete = vi.fn();
    const completion = store.waitForRealtimeReconciliation().then(complete);
    await flushPromises(20);
    expect(complete).not.toHaveBeenCalled();
    first.resolve(result);
    await vi.waitFor(() => expect(apiMocks.readMessages).toHaveBeenCalled());
    expect(
      refresh.mock.calls.map(([anchor, forward, cursor]) => [anchor, forward, cursor])
    ).toEqual([
      ['FIRST', false, undefined]
    ]);
    expect(complete).not.toHaveBeenCalled();
    last.resolve([]);
    await completion;
    expect(apiMocks.readRealtimeResource).not.toHaveBeenCalled();
    expect(apiMocks.readRealtimeUsers).not.toHaveBeenCalled();
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource).not.toHaveBeenCalled();
    expect(apiMocks.readRealtimeUsers).not.toHaveBeenCalled();
  });

  it('discards queued message reads when a snapshot replaces the projection', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    await flushPromises(20);
    const first = deferred<Awaited<ReturnType<typeof messages.refreshCurrentWindow>>>();
    const refresh = vi.spyOn(messages, 'refreshCurrentWindow').mockReturnValue(first.promise);
    store.realtimeProjectionHandler(userLeftRoom('R1', 'U2', 'OLD-1'));
    store.realtimeProjectionHandler(userLeftRoom('R1', 'U2', 'OLD-2'));
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true }));
    first.resolve({ hasOlder: false, hasNewer: false, refreshed: false, changed: false });
    await flushPromises(20);
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(refresh.mock.calls[0][3]!()).toBe(false);
  });

  it('publishes refreshed canonical resources to every projection consumer', async () => {
    const users = new RealtimeResourceUpdate({
      resource: {
        case: 'users',
        value: new ListUsersResponse({
          users: [
            new DirectoryMember({
              user: new User({ id: 'U2', login: 'bob', displayName: 'Robert' })
            })
          ]
        })
      }
    });
    apiMocks.readRealtimeUsers.mockResolvedValueOnce([users]);
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    eventBusManager.ensureBus(
      store.serverId,
      fake as unknown as ServerConnection,
      true,
      store.realtimeSync,
      store.realtimeProjectionHandler
    );
    const observer = vi.fn();
    eventBusManager.getBus(store.serverId)?.projectionHandlers.add(observer);

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          event: {
            case: 'userProfileChanged',
            value: new UserProfileChangedEvent({ userId: 'U2' })
          }
        })
      })
    );
    await flushPromises();

    expect(store.projection.users.get('U2')?.user?.displayName).toBe('Robert');
    expect(observer).toHaveBeenCalledWith(
      expect.objectContaining({ resource: users.resource, replaceResource: true })
    );
  });

  it('does not revoke viewer room access when another user leaves', async () => {
    const store = makeStore(new FakeServerConnection([]));
    store.currentUser.user = { id: 'U1' } as typeof store.currentUser.user;
    const messages = store.messagesForRoom('R1');
    const clear = vi.spyOn(messages, 'clearForAccessRevocation');
    await flushPromises();
    clear.mockClear();

    store.realtimeProjectionHandler(userLeftRoom('R1', 'U2'));

    expect(clear).not.toHaveBeenCalled();
  });

  it('revokes viewer room access synchronously when the viewer leaves', async () => {
    const store = makeStore(new FakeServerConnection([]));
    store.currentUser.user = { id: 'U1' } as typeof store.currentUser.user;
    const messages = store.messagesForRoom('R1');
    const clear = vi.spyOn(messages, 'clearForAccessRevocation');
    await flushPromises();
    clear.mockClear();

    store.realtimeProjectionHandler(userLeftRoom('R1', 'U1'));

    expect(clear).toHaveBeenCalledOnce();
  });

  it.each([
    {
      case: 'assetProcessingStarted',
      value: new AssetProcessingStartedEvent({ assetId: 'A1', roomId: 'R1', messageEventId: 'M1' })
    },
    {
      case: 'assetProcessingSucceeded',
      value: new AssetProcessingSucceededEvent({
        assetId: 'A1',
        roomId: 'R1',
        messageEventId: 'M1'
      })
    },
    {
      case: 'assetProcessingFailed',
      value: new AssetProcessingFailedEvent({ assetId: 'A1', roomId: 'R1', messageEventId: 'M1' })
    },
    {
      case: 'assetDeleted',
      value: new AssetDeletedEvent({ assetId: 'A1', roomId: 'R1', messageEventId: 'M1' })
    }
  ] as const)('scopes $case reads to the affected room and message', async (event) => {
    const store = makeStore(new FakeServerConnection([]));
    const rooms = ['R1', 'R2'].map((id) => ({
      room: store.messagesForRoom(id),
      thread: store.messagesForThread(id, 'ROOT'),
      files: store.filesForRoom(id),
      pins: store.pinsForRoom(id)
    }));
    await flushPromises(20);
    const refreshes = rooms.map(({ room, thread, files, pins }) => ({
      room: vi.spyOn(room, 'refreshCurrentWindow').mockResolvedValue({
        hasOlder: false,
        hasNewer: false,
        refreshed: true,
        changed: true
      }),
      thread: vi.spyOn(thread, 'refreshCurrentWindow').mockResolvedValue({
        hasOlder: false,
        hasNewer: false,
        refreshed: true,
        changed: true
      }),
      files: vi.spyOn(files, 'refreshRetained').mockImplementation(() => {}),
      pins: vi.spyOn(pins, 'retry').mockImplementation(() => {})
    }));
    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        cursor: 'asset-cursor',
        event: new RealtimeEvent({ id: 'asset-event', event })
      })
    );
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readMessages).toHaveBeenCalledExactlyOnceWith('R1', ['M1'], 'asset-cursor');
    for (const room of refreshes) for (const read of Object.values(room)) expect(read).not.toHaveBeenCalled();
  });

  it('refreshes a mounted room timeline for canonical membership rows', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const refresh = vi.spyOn(messages, 'refreshCurrentWindow').mockResolvedValue({
      hasOlder: false,
      hasNewer: false,
      refreshed: true,
      changed: true
    });
    await flushPromises();
    refresh.mockClear();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          id: 'E-JOIN',
          actorId: 'U2',
          event: {
            case: 'userJoinedRoom',
            value: new UserJoinedRoomEvent({ roomId: 'R1' })
          }
        })
      })
    );
    store.realtimeProjectionHandler(userLeftRoom('R1', 'U2', 'E-LEAVE'));
    await flushPromises();

    expect(refresh).toHaveBeenCalledWith('E-JOIN', false, undefined, expect.any(Function));
    expect(refresh).toHaveBeenCalledWith('E-LEAVE', false, undefined, expect.any(Function));
  });

  it('refreshes a mounted room timeline for a threading-mode row', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const refresh = vi.spyOn(messages, 'refreshCurrentWindow').mockResolvedValue({
      hasOlder: false,
      hasNewer: false,
      refreshed: true,
      changed: true
    });
    await flushPromises();
    refresh.mockClear();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          id: 'E-THREADING-MODE',
          event: {
            case: 'roomThreadingModeChanged',
            value: new RoomThreadingModeChangedEvent({ roomId: 'R1' })
          }
        })
      })
    );
    await flushPromises();

    expect(refresh).toHaveBeenCalledWith(
      'E-THREADING-MODE',
      false,
      undefined,
      expect.any(Function)
    );
  });

  it('batches thread roots and replies without refreshing timeline windows', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const threadMessages = store.messagesForThread('R1', 'E-ROOT');
    const refresh = vi.spyOn(messages, 'refreshCurrentWindow').mockResolvedValue({
      hasOlder: false,
      hasNewer: false,
      refreshed: true,
      changed: true
    });
    const refreshThread = vi.spyOn(threadMessages, 'refreshCurrentWindow').mockResolvedValue({
      hasOlder: false,
      hasNewer: false,
      refreshed: true,
      changed: true
    });
    const setThreadFollow = vi.spyOn(threadMessages, 'setThreadRootFollowState');
    await flushPromises();
    refresh.mockClear();
    refreshThread.mockClear();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          event: {
            case: 'threadViewerStateChanged',
            value: new ThreadViewerStateChangedEvent({
              roomId: 'R1',
              threadRootEventId: 'E-ROOT',
              isFollowing: true
            })
          }
        })
      })
    );

    expect(setThreadFollow).toHaveBeenCalledWith('E-ROOT', true);
    expect(refreshThread).not.toHaveBeenCalled();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          id: 'E-REPLY',
          event: {
            case: 'messagePosted',
            value: new MessagePostedEvent({ roomId: 'R1', threadRootEventId: 'E-ROOT' })
          }
        })
      })
    );
    await store.waitForRealtimeReconciliation();

    expect(apiMocks.readMessages).toHaveBeenCalledExactlyOnceWith('R1', ['E-ROOT', 'E-REPLY'], undefined);
    expect(refresh).not.toHaveBeenCalled();
    expect(refreshThread).not.toHaveBeenCalled();
    expect(cacheMocks.refreshFollowedThreads).toHaveBeenCalledTimes(2);
  });

  it('keeps a long thread window when reconciling a successful read', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const thread = store.messagesForThread('R1', 'ROOT');
    await flushPromises();
    const row = (index: number): TimelineEventView => ({
      id: index === 0 ? 'ROOT' : `REPLY-${index}`,
      createdAt: new Date(Date.UTC(2026, 0, 1, 0, 0, index)).toISOString(),
      event: {
        kind: TimelineEventKind.MessagePosted,
        roomId: 'R1',
        threadRootEventId: index === 0 ? null : 'ROOT',
        body: `Message ${index}`,
        attachments: [],
        replyCount: 0,
        threadParticipants: [],
        reactions: []
      }
    });
    const loaded = Array.from({ length: 121 }, (_, index) => row(index));
    for (const event of loaded) thread.ingestEvent(event);
    const beforeRead = thread.threadEvents.map((event) => event.id);
    expect(beforeRead).toHaveLength(120);
    const api: ReturnType<typeof createRoomTimelineAPI> = vi
      .mocked(createRoomTimelineAPI).mock.results.at(-1)!.value;
    vi.mocked(api.getThreadEventsAround).mockResolvedValue({
      events: loaded.slice(0, 50),
      startCursor: 'first-reply',
      endCursor: 'reply-49',
      hasOlder: false,
      hasNewer: true
    });

    store.reconcileThreadRead('R1', 'ROOT');
    await flushPromises();

    expect(thread.threadEvents.map((event) => event.id)).toEqual(beforeRead);
  });

  it('reconciles every successful thread read without a realtime hint', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const room = store.messagesForRoom('R1');
    const thread = store.messagesForThread('R1', 'E-ROOT');
    const result = { hasOlder: false, hasNewer: false, refreshed: true, changed: true };
    const refreshRoom = vi.spyOn(room, 'refreshCurrentWindow').mockResolvedValue(result);
    const refreshThread = vi.spyOn(thread, 'refreshCurrentWindow').mockResolvedValue(result);
    await flushPromises();
    refreshRoom.mockClear();
    refreshThread.mockClear();
    cacheMocks.refreshFollowedThreads.mockClear();

    store.reconcileThreadRead('R1', 'E-ROOT');
    await store.waitForRealtimeReconciliation();
    store.reconcileThreadRead('R1', 'E-ROOT');
    await store.waitForRealtimeReconciliation();

    expect(apiMocks.readMessages).toHaveBeenCalledTimes(2);
    expect(apiMocks.readMessages).toHaveBeenLastCalledWith('R1', ['E-ROOT'], undefined);
    expect(refreshRoom).not.toHaveBeenCalled();
    expect(refreshThread).not.toHaveBeenCalled();
    expect(cacheMocks.refreshFollowedThreads).toHaveBeenCalledTimes(2);
  });

  it.each([false, true])('uses cached realtime authors (bot: %s)', async (isBot) => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    vi.spyOn(messages, 'refreshPostedMessage').mockResolvedValue(false);
    await flushPromises();
    const ingest = vi.spyOn(messages, 'ingestEvent');
    store.projection.users.set('cached-author', userProfileFixture(
      {
        id: 'cached-author',
        login: 'cached',
        displayName: 'Cached author',
        avatarUrl: null,
        deleted: false,
        isBot
      }
    ));
    const post = () =>
      store.realtimeProjectionHandler(
        new RealtimeProjectionUpdate({
          event: new RealtimeEvent({
            id: 'cached-post',
            actorId: 'cached-author',
            event: {
              case: 'messagePosted',
              value: new MessagePostedEvent({ roomId: 'R1', bodyPlaintext: 'hello' })
            }
          })
        })
      );
    expect(store.projection.users.has('cached-author')).toBe(true);
    post();
    expect(ingest).toHaveBeenLastCalledWith(
      expect.objectContaining({
        actor: expect.objectContaining({
          id: 'cached-author',
          displayName: 'Cached author',
          isBot
        }),
        actorResolution: undefined
      })
    );
    store.realtimeProjectionHandler(userDeleted('cached-author'));
    // Even a late cache response must not restore a deleted account.
    store.projection.users.seed(userProfileFixture(
      {
        id: 'cached-author',
        login: 'stale',
        displayName: 'Stale author',
        avatarUrl: null,
        deleted: false,
        isBot
      }
    ));
    post();
    expect(ingest).toHaveBeenLastCalledWith(
      expect.objectContaining({
        actor: null,
        actorResolution: 'deleted'
      })
    );
  });

  it.each([false, true])('shares one reply read across Files, pins and timelines (thread open: %s)', async (openThread) => {
    const store = makeStore(new FakeServerConnection([]));
    const room = store.messagesForRoom('R1');
    const thread = openThread ? store.messagesForThread('R1', 'ROOT') : null;
    const unrelated = store.messagesForThread('R1', 'OTHER');
    const files = store.filesForRoom('R1');
    const pins = store.pinsForRoom('R1');
    const message = new Message({
      id: 'REPLY', roomId: 'R1', threadRootEventId: 'ROOT', body: 'file',
      attachments: [new MessageAttachment({ id: 'ASSET', filename: 'file.txt', contentType: 'text/plain' })]
    });
    apiMocks.listPins.mockResolvedValue({
      items: [new PinnedMessage({ message })], totalCount: 1, hasMore: false, latestPinMarker: 'PIN'
    });
    await Promise.all([files.hydrate(), pins.hydrate()]);
    await flushPromises(20);
    const roomRefresh = vi.spyOn(room, 'refreshCurrentWindow');
    const unrelatedRefresh = vi.spyOn(unrelated, 'refreshCurrentWindow');
    const resource = (value: Message): MessageResource => ({ message: value, timeline: messageToTimelineEvent(value, {}) });
    apiMocks.readMessages.mockResolvedValue([resource(message)]);
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'post', event: new RealtimeEvent({ id: 'REPLY', event: {
        case: 'messagePosted', value: new MessagePostedEvent({ roomId: 'R1', threadRootEventId: 'ROOT', bodyPlaintext: 'file' })
      } })
    }));
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readMessages).toHaveBeenCalledExactlyOnceWith('R1', ['REPLY', 'ROOT'], 'post');
    expect(files.items.map((item) => item.attachment.id)).toEqual(['ASSET']);
    if (thread) expect(thread.getEventById('REPLY')?.event).toMatchObject({ attachments: [{ id: 'ASSET' }] });
    expect(unrelated.threadEvents).toHaveLength(0);
    expect(roomRefresh).not.toHaveBeenCalled();
    expect(unrelatedRefresh).not.toHaveBeenCalled();
    const existingRows = files.items;

    apiMocks.readMessages.mockResolvedValue([resource(new Message({ id: 'TEXT', roomId: 'R1', body: 'text only' }))]);
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'text', event: new RealtimeEvent({ id: 'TEXT', event: {
        case: 'messagePosted', value: new MessagePostedEvent({ roomId: 'R1', bodyPlaintext: 'text only' })
      } })
    }));
    await store.waitForRealtimeReconciliation();
    expect(files.items).toBe(existingRows);
    expect(files.isInitialLoading).toBe(false);

    const edited = message.clone();
    edited.body = 'edited';
    edited.attachments = [];
    apiMocks.readMessages.mockResolvedValue([resource(edited)]);
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'edit', event: new RealtimeEvent({ event: {
        case: 'messageEdited', value: new MessageEditedEvent({ roomId: 'R1', messageEventId: 'REPLY' })
      } })
    }));
    await store.waitForRealtimeReconciliation();
    expect(files.items).toEqual([]);
    expect(pins.items[0].message?.body).toBe('edited');
    expect(apiMocks.listRoomAttachments).toHaveBeenCalledOnce();
    expect(apiMocks.listPins).toHaveBeenCalledOnce();
  });

  it('does not restore files from a message read after room access is revoked', async () => {
    const store = makeStore(new FakeServerConnection([]));
    store.currentUser.user = { id: 'U1' } as typeof store.currentUser.user;
    store.messagesForRoom('R1');
    const files = store.filesForRoom('R1');
    await files.hydrate();
    const pending = deferred<MessageResource[]>();
    apiMocks.readMessages.mockReturnValue(pending.promise);
    store.realtimeProjectionHandler(new RealtimeProjectionUpdate({
      cursor: 'post', event: new RealtimeEvent({ id: 'POST', event: {
        case: 'messagePosted', value: new MessagePostedEvent({ roomId: 'R1' })
      } })
    }));
    await vi.waitFor(() => expect(apiMocks.readMessages).toHaveBeenCalledOnce());
    store.realtimeProjectionHandler(userLeftRoom('R1', 'U1'));
    const message = new Message({ id: 'POST', roomId: 'R1', attachments: [new MessageAttachment({ id: 'SECRET' })] });
    pending.resolve([{ message, timeline: messageToTimelineEvent(message, {}) }]);
    await store.waitForRealtimeReconciliation();
    expect(files.items).toEqual([]);
  });

  it('hydrates a new room post through the shared batch without a window read', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const ingest = vi.spyOn(messages, 'ingestEvent');
    const refresh = vi.spyOn(messages, 'refreshCurrentWindow').mockResolvedValue({
      hasOlder: false,
      hasNewer: false,
      refreshed: true,
      changed: true
    });
    await flushPromises();
    ingest.mockClear();
    refresh.mockClear();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          id: 'E-POST',
          event: {
            case: 'messagePosted',
            value: new MessagePostedEvent({ roomId: 'R1', bodyPlaintext: 'new body' })
          }
        })
      })
    );

    expect(ingest).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'E-POST',
        actor: null,
        actorResolution: 'loading',
        event: expect.objectContaining({
          body: 'new body',
          roomId: 'R1',
          attachments: [],
          linkPreview: null,
          reactions: [],
          pinned: false,
          threadExists: false,
          replyCount: 0,
          threadParticipants: []
        })
      })
    );
    expect(refresh).not.toHaveBeenCalled();
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readMessages).toHaveBeenCalledExactlyOnceWith('R1', ['E-POST'], undefined);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('drives call sounds from the canonical participant event', async () => {
    const store = makeStore(new FakeServerConnection([]));
    store.currentUser.user = { id: 'U1' } as typeof store.currentUser.user;
    vi.spyOn(store.voiceCall, 'callTransitionSoundDecision').mockReturnValue('play');

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          id: 'E-CALL-JOIN',
          actorId: 'U2',
          event: {
            case: 'voiceCallParticipantJoined',
            value: new VoiceCallParticipantJoinedEvent({ roomId: 'R1', callId: 'CALL-1' })
          }
        })
      })
    );

    expect(soundMocks.playCallSound).toHaveBeenCalledWith('join');
    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('activeCalls', undefined);
  });

  it('prepares a missing room destination and waits for its participant hydration', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const rooms = deferred<RealtimeResourceUpdate[]>();
    const users = deferred<RealtimeResourceUpdate[]>();
    apiMocks.readRealtimeResource.mockReturnValueOnce(rooms.promise);
    apiMocks.readRealtimeUsers.mockReturnValueOnce(users.promise);
    const ready = vi.fn();
    const pending = store.ensureRoomAvailable('DM1').then(ready);
    await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('rooms', undefined));
    rooms.resolve([roomResource([new RoomWithViewerState({
      room: { id: 'DM1' }, memberUserIds: ['U2'], viewerState: { isMember: true }
    })])]);
    await flushPromises();
    expect(apiMocks.readRealtimeUsers).toHaveBeenCalledWith(['U2'], undefined);
    expect(ready).not.toHaveBeenCalled();
    users.resolve([]);
    await pending;
    expect(store.projection.rooms.has('DM1')).toBe(true);
    expect(ready).toHaveBeenCalledOnce();
    apiMocks.readRealtimeResource.mockClear();
    await store.ensureRoomAvailable('DM1');
    expect(apiMocks.readRealtimeResource).not.toHaveBeenCalled();
  });

  it.each(['missing', 'failed', 'reset', 'disposed'])(
    'rejects an unavailable room destination: %s', async (failure) => {
      const store = makeStore(new FakeServerConnection([]));
      const rooms = deferred<RealtimeResourceUpdate[]>();
      apiMocks.readRealtimeResource.mockReturnValueOnce(rooms.promise);
      const rejected = expect(store.ensureRoomAvailable('DM1')).rejects.toThrow();
      await vi.waitFor(() => expect(apiMocks.readRealtimeResource).toHaveBeenCalled());
      if (failure === 'reset') store.realtimeProjectionHandler(new RealtimeProjectionUpdate({ reset: true, privacyReset: true }));
      if (failure === 'disposed') store.dispose();
      if (failure === 'failed') rooms.reject(new Error('offline'));
      else rooms.resolve(failure === 'missing' ? [roomResource([])] : [roomResource([
        new RoomWithViewerState({ room: { id: 'DM1' }, viewerState: { isMember: true } })
      ])]);
      await rejected;
      expect(store.projection.rooms.has('DM1')).toBe(false);
    }
  );

  it('refreshes canonical rooms after a neutral unread invalidation', async () => {
    const store = makeStore(new FakeServerConnection([]));
    apiMocks.readRealtimeResource.mockClear();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          event: {
            case: 'notificationUnreadStateChanged',
            value: new NotificationUnreadStateChangedEvent({ roomId: 'R1' })
          }
        })
      })
    );

    await store.waitForRealtimeReconciliation();
    expect(apiMocks.readRealtimeResource).not.toHaveBeenCalledWith('notifications', undefined);
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('rooms', undefined);
  });
});
