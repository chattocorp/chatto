import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { AuthenticatedServerState } from '$lib/api-client/serverState';
import type { RoomFileItem } from '$lib/api-client/attachments';
import { createRoomTimelineAPI, type EventConnectionPage } from '$lib/api-client/roomTimeline';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { GetMotdResponse } from '@chatto/api-types/api/v1/server_state_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import {
  ListRoomsResponse,
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
import { CapabilityGrant } from '@chatto/api-types/api/v1/permissions_pb';
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
    getCallToken: vi.fn(() => Promise.resolve(null)),
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

vi.mock('$lib/api-client/roomDirectory', () => ({
  RoomDirectoryScope: {
    ALL: 1
  },
  RoomKind: {
    CHANNEL: 1,
    DM: 2
  },
  mapDirectoryRoom: (room: unknown) => room,
  mapRoomGroup: (group: unknown) => group,
  createRoomDirectoryAPI: vi.fn(() => ({
    listRooms: apiMocks.listRooms,
    listRoomGroups: apiMocks.listRoomGroups
  }))
}));

vi.mock('$lib/api-client/memberDirectory', () => ({
  mapDirectoryMember: (member: unknown) => member,
  createMemberDirectoryAPI: vi.fn(() => ({
    listRoomMembers: apiMocks.listRoomMembers
  }))
}));

vi.mock('$lib/api-client/voiceCalls', () => ({
  createVoiceCallAPI: vi.fn(() => ({
    joinCall: apiMocks.joinCall,
    getCallToken: apiMocks.getCallToken,
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
  registerServerQueryCache({
    server: cacheMocks.removeRegisteredServerQueries,
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
  apiMocks.getCallToken.mockResolvedValue(null);
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
    'keeps edits and reactions at distinct queued %s anchors',
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
      const page = (id: number): EventConnectionPage => ({
        events: [row(id, true)],
        startCursor: null,
        endCursor: null,
        hasOlder: true,
        hasNewer: true
      });
      const first = deferred<EventConnectionPage>();
      const api: ReturnType<typeof createRoomTimelineAPI> = vi
        .mocked(createRoomTimelineAPI)
        .mock.results.at(-1)!.value;
      const read = vi
        .mocked(scope === 'room' ? api.getRoomEventsAround : api.getThreadEventsAround)
        .mockImplementation(({ eventId }) => Promise.resolve(page(Number(eventId.slice(1)))))
        .mockReturnValueOnce(first.promise);
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
      expect(read).toHaveBeenCalledTimes(1);
      first.resolve(page(1));
      await vi.waitFor(() => expect(read).toHaveBeenCalledTimes(3));
      await flushPromises(20);
      expect(read.mock.calls.map(([input]) => input.eventId)).toEqual(['M1', 'M70', 'M140']);
      expect(messages.getEventById('M70')?.event).toMatchObject({ body: 'edited' });
      expect(messages.getEventById('M140')?.event).toMatchObject({
        reactions: [{ emoji: 'ok', count: 1 }]
      });
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
    expect([...catchUpUserIds]).toEqual(['U2']);
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
    const observed = store.waitForRealtimeResourceRefresh('notifications');
    let completed = false;
    void observed.then(() => {
      completed = true;
    });
    changed();
    first.resolve([]);
    await flushPromises();
    expect(completed).toBe(false);
    const failure = new Error('notification read failed');
    second.reject(failure);
    await expect(observed).resolves.toBe(false);
    await expect(store.waitForRealtimeReconciliation()).rejects.toBe(failure);
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledTimes(2);
  });

  it('does not complete a durable cursor when message hydration fails', async () => {
    const messageRead = deferred<boolean>();
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const hydrate = vi.spyOn(messages, 'refreshPostedMessage').mockReturnValue(messageRead.promise);
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
    expect(hydrate).toHaveBeenCalledWith('E-POST', 'opaque-message-cursor', expect.any(Function));

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

  it('waits for cursorless window reads and every queued direction without auxiliary RPCs', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    await flushPromises(20);
    const first = deferred<Awaited<ReturnType<typeof messages.refreshCurrentWindow>>>();
    const last = deferred<Awaited<ReturnType<typeof messages.refreshCurrentWindow>>>();
    const result = { hasOlder: false, hasNewer: false, refreshed: true, changed: true };
    const refresh = vi
      .spyOn(messages, 'refreshCurrentWindow')
      .mockResolvedValue(result)
      .mockReturnValueOnce(first.promise);
    vi.spyOn(messages, 'refreshPostedMessage').mockResolvedValue(true);
    store.realtimeProjectionHandler(userLeftRoom('R1', 'U2', 'FIRST'));
    await flushPromises(20);
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
    await flushPromises(20);
    apiMocks.readRealtimeResource.mockClear();
    apiMocks.readRealtimeUsers.mockClear();
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
    refresh.mockResolvedValueOnce(result).mockReturnValueOnce(last.promise);
    const complete = vi.fn();
    const completion = store.waitForRealtimeReconciliation().then(complete);
    await flushPromises(20);
    expect(complete).not.toHaveBeenCalled();
    first.resolve(result);
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(3));
    expect(
      refresh.mock.calls.map(([anchor, forward, cursor]) => [anchor, forward, cursor])
    ).toEqual([
      ['FIRST', false, undefined],
      ['POST', true, 'post-cursor'],
      ['POST', false, 'edit-cursor']
    ]);
    expect(complete).not.toHaveBeenCalled();
    last.resolve(result);
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
    for (const read of [refreshes[0].room, refreshes[0].thread]) {
      expect(read).toHaveBeenCalledExactlyOnceWith(
        'M1',
        false,
        'asset-cursor',
        expect.any(Function)
      );
    }
    expect(refreshes[0].files).toHaveBeenCalledOnce();
    expect(refreshes[0].pins).toHaveBeenCalledOnce();
    for (const read of Object.values(refreshes[1])) expect(read).not.toHaveBeenCalled();
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

  it('refreshes thread reads for follow changes and replies', async () => {
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
    const refreshPostedThread = vi
      .spyOn(threadMessages, 'refreshPostedMessage')
      .mockResolvedValue(true);
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
    await flushPromises();

    expect(refresh).toHaveBeenCalledWith('E-ROOT', false, undefined, expect.any(Function));
    expect(refreshPostedThread).toHaveBeenCalledWith('E-REPLY', undefined, expect.any(Function));
    expect(refreshThread).toHaveBeenCalledWith('E-REPLY', true, undefined, expect.any(Function));
    expect(cacheMocks.refreshFollowedThreads).toHaveBeenCalledTimes(2);
  });

  it('hydrates a new room post before advancing the retained window', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const messages = store.messagesForRoom('R1');
    const hydrate = vi.spyOn(messages, 'refreshPostedMessage').mockResolvedValue(true);
    const ingest = vi.spyOn(messages, 'ingestEvent');
    const refresh = vi.spyOn(messages, 'refreshCurrentWindow').mockResolvedValue({
      hasOlder: false,
      hasNewer: false,
      refreshed: true,
      changed: true
    });
    await flushPromises();
    hydrate.mockClear();
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
    expect(hydrate).toHaveBeenCalledWith('E-POST', undefined, expect.any(Function));
    expect(refresh).not.toHaveBeenCalled();
    await flushPromises();
    expect(refresh).toHaveBeenCalledWith('E-POST', true, undefined, expect.any(Function));
  });

  it('drives call sounds from the canonical participant event', () => {
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
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('activeCalls', undefined);
  });

  it('refreshes canonical rooms after a neutral unread invalidation', () => {
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

    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('notifications', undefined);
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('rooms', undefined);
  });
});
