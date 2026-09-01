import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { AuthenticatedServerState } from '$lib/api-client/serverState';
import type { RoomFileItem } from '$lib/api-client/attachments';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import {
  ListRoomsResponse,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import { ServerSnapshotChunk } from '@chatto/api-types/api/v1/server_snapshot_pb';
import { ListUsersResponse } from '@chatto/api-types/api/v1/user_service_pb';
import { Event } from '@chatto/api-types/core/evt/v1/event_pb';
import {
  CallParticipantJoinedEvent,
  RoomThreadingModeChangedEvent,
  UserJoinedRoomEvent,
  UserLeftRoomEvent
} from '@chatto/api-types/core/evt/v1/room_events_pb';
import { MessagePostedEvent } from '@chatto/api-types/core/evt/v1/message_events_pb';
import { ThreadFollowChangedEvent } from '@chatto/api-types/core/live/v1/live_events_pb';
import {
  UserAccountDeletedEvent,
  UserDisplayNameChangedEvent
} from '@chatto/api-types/core/evt/v1/user_events_pb';

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
    readRealtimeResource: vi.fn(() => Promise.resolve([] as ServerSnapshotChunk[])),
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

vi.mock('$lib/api-client/realtimeResources', () => ({
  createRealtimeResourceAPI: vi.fn(() => ({ read: apiMocks.readRealtimeResource }))
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

vi.mock('$lib/api-client/viewer', () => ({
  getViewerStateViaConnect: apiMocks.getViewerStateViaConnect,
  getCurrentUserViaConnect: apiMocks.getCurrentUserViaConnect,
  viewerResponseToState: (viewer: unknown) => viewer
}));

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
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function roomSnapshot(rooms: RoomWithViewerState[]): ServerSnapshotChunk {
  return new ServerSnapshotChunk({
    resource: { case: 'rooms', value: new ListRoomsResponse({ rooms }) }
  });
}

function userDeleted(userId: string): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    event: new Event({
      event: { case: 'userAccountDeleted', value: new UserAccountDeletedEvent({ userId }) }
    })
  });
}

function userLeftRoom(roomId: string, actorId: string, eventId = ''): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    event: new Event({
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
  apiMocks.listRoomAttachments.mockReset();
  apiMocks.listRoomAttachments.mockResolvedValue({ items: [], totalCount: 0, hasMore: false });
  apiMocks.refreshAssetUrls.mockReset();
  apiMocks.refreshAssetUrls.mockResolvedValue(new Map());
  apiMocks.joinCall.mockResolvedValue(true);
  apiMocks.getCallToken.mockResolvedValue(null);
  apiMocks.leaveCall.mockResolvedValue(true);
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
  it('applies canonical room snapshots without a realtime-specific room shape', () => {
    const store = makeStore(new FakeServerConnection([]));

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        snapshot: roomSnapshot([
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

  it('applies public server resources through the shared snapshot chunk', () => {
    const store = makeStore(new FakeServerConnection([]));

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        snapshot: new ServerSnapshotChunk({
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
    const pending = deferred<ServerSnapshotChunk[]>();
    apiMocks.readRealtimeResource.mockReturnValueOnce(pending.promise);
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
    const first = deferred<ServerSnapshotChunk[]>();
    apiMocks.readRealtimeResource
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce([]);
    const store = makeStore(new FakeServerConnection([]));

    store.realtimeProjectionHandler(userDeleted('U2'));
    store.realtimeProjectionHandler(userDeleted('U3'));

    expect(apiMocks.readRealtimeResource).toHaveBeenCalledTimes(1);
    first.resolve([]);
    await flushPromises();

    expect(apiMocks.readRealtimeResource).toHaveBeenCalledTimes(2);
    expect(apiMocks.readRealtimeResource).toHaveBeenNthCalledWith(2, 'users');
  });

  it('publishes refreshed canonical resources to every projection consumer', async () => {
    const users = new ServerSnapshotChunk({
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
    apiMocks.readRealtimeResource.mockResolvedValueOnce([users]);
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
        event: new Event({
          event: {
            case: 'userDisplayNameChanged',
            value: new UserDisplayNameChangedEvent({
              userId: 'U2',
              displayNamePlaintext: 'Robert'
            })
          }
        })
      })
    );
    await flushPromises();

    expect(store.projection.users.get('U2')?.user?.displayName).toBe('Robert');
    expect(observer).toHaveBeenCalledWith(
      expect.objectContaining({ snapshot: users })
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
        event: new Event({
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

    expect(refresh).toHaveBeenCalledWith('E-JOIN');
    expect(refresh).toHaveBeenCalledWith('E-LEAVE');
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
        event: new Event({
          id: 'E-THREADING-MODE',
          event: {
            case: 'roomThreadingModeChanged',
            value: new RoomThreadingModeChangedEvent({ roomId: 'R1' })
          }
        })
      })
    );
    await flushPromises();

    expect(refresh).toHaveBeenCalledWith('E-THREADING-MODE');
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
    const setThreadFollow = vi.spyOn(threadMessages, 'setThreadRootFollowState');
    await flushPromises();
    refresh.mockClear();
    refreshThread.mockClear();

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new Event({
          event: {
            case: 'threadFollowChangedSync',
            value: new ThreadFollowChangedEvent({
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
        event: new Event({
          id: 'E-REPLY',
          event: {
            case: 'messagePosted',
            value: new MessagePostedEvent({ roomId: 'R1', inThread: 'E-ROOT' })
          }
        })
      })
    );

    expect(refresh).toHaveBeenCalledWith('E-ROOT');
    expect(refreshThread).toHaveBeenCalledWith('E-REPLY');
    expect(cacheMocks.refreshFollowedThreads).toHaveBeenCalledTimes(2);
  });

  it('drives call sounds from the canonical participant event', () => {
    const store = makeStore(new FakeServerConnection([]));
    store.currentUser.user = { id: 'U1' } as typeof store.currentUser.user;
    vi.spyOn(store.voiceCall, 'callTransitionSoundDecision').mockReturnValue('play');

    store.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        event: new Event({
          id: 'E-CALL-JOIN',
          actorId: 'U2',
          event: {
            case: 'voiceCallParticipantJoined',
            value: new CallParticipantJoinedEvent({ roomId: 'R1', callId: 'CALL-1' })
          }
        })
      })
    );

    expect(soundMocks.playCallSound).toHaveBeenCalledWith('join');
    expect(apiMocks.readRealtimeResource).toHaveBeenCalledWith('activeCalls');
  });
});
