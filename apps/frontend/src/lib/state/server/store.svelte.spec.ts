import { Timestamp } from '@bufbuild/protobuf';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { flushSync } from 'svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { AuthenticatedServerState } from '$lib/api-client/serverState';
import { RoomEventKind } from '$lib/render/eventKinds';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { ServerRuntimeConfig } from '@chatto/api-types/api/v1/server_state_pb';
import { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import {
  RoomMessagePosted,
  RoomTimelineEvent,
  RoomTimelinePage
} from '@chatto/api-types/api/v1/room_timeline_pb';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionActiveCallsReplace,
  RealtimeProjectionOperation,
  RealtimeProjectionRoomActivity,
  RealtimeProjectionRoomTimelineEventUpsert,
  RealtimeProjectionRoomTimelineReplace,
  RealtimeProjectionServerState
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { MAX_RETAINED_ROOM_TIMELINES } from './realtimeSync.svelte';

const { soundMocks, apiMocks } = vi.hoisted(() => ({
  soundMocks: {
    playCallSound: vi.fn(() => Promise.resolve())
  },
  apiMocks: {
    listRooms: vi.fn(() => Promise.resolve([])),
    listRoomGroups: vi.fn(() => Promise.resolve([])),
    listRoomMembers: vi.fn(() =>
      Promise.resolve({
        members: [],
        totalCount: 0,
        hasMore: false
      })
    ),
    listActiveCalls: vi.fn(() => Promise.resolve([])),
    getActiveCall: vi.fn(() => Promise.resolve(null)),
    batchGetActiveCalls: vi.fn(() => Promise.resolve([])),
    listCallParticipants: vi.fn(() => Promise.resolve([])),
    joinCall: vi.fn(() => Promise.resolve(true)),
    getCallToken: vi.fn(() => Promise.resolve(null)),
    leaveCall: vi.fn(() => Promise.resolve(true)),
    listNotificationCounts: vi.fn(() => Promise.resolve({})),
    listNotifications: vi.fn(() =>
      Promise.resolve({
        items: [],
        unreadCount: 0
      })
    ),
    listAdminEventLogEvents: vi.fn(() =>
      Promise.resolve({
        entries: [],
        hasOlder: false,
        endCursor: null,
        totalCount: '0',
        scannedCount: 0,
        scanLimit: 50,
        scanLimited: false
      })
    ),
    listAdminEventLogEventTypes: vi.fn(() => Promise.resolve([])),
    getAdminEventLogEvent: vi.fn(() => Promise.resolve(null)),
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
        canManageUserPermissions: false,
        serverNotificationPreference: {
          level: 'DEFAULT',
          effectiveLevel: 'NORMAL'
        },
        roomNotificationPreferences: []
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
    )
  }
}));

function roomEvent(kind: RoomEventKind, fields: Record<string, unknown> = {}) {
  return { kind, ...fields } as never;
}

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
    listActiveCalls: apiMocks.listActiveCalls,
    getActiveCall: apiMocks.getActiveCall,
    batchGetActiveCalls: apiMocks.batchGetActiveCalls,
    listCallParticipants: apiMocks.listCallParticipants,
    joinCall: apiMocks.joinCall,
    getCallToken: apiMocks.getCallToken,
    leaveCall: apiMocks.leaveCall
  }))
}));

vi.mock('$lib/api-client/notifications', () => ({
  NotificationItemKind: {
    DirectMessage: 'directMessage',
    Mention: 'mention',
    Reply: 'reply',
    RoomMessage: 'roomMessage'
  },
  mapNotificationPage: vi.fn((response) => ({
    items: [],
    totalCount: Number(response.page?.totalCount ?? 0),
    hasMore: response.page?.hasMore ?? false
  })),
  createNotificationAPI: vi.fn(() => ({
    listNotifications: apiMocks.listNotifications,
    listRoomNotifications: vi.fn(),
    hasNotifications: vi.fn(),
    listNotificationCounts: apiMocks.listNotificationCounts,
    dismissNotification: vi.fn(),
    dismissAllNotifications: vi.fn()
  }))
}));

vi.mock('$lib/api-client/adminEventLog', () => ({
  EMPTY_ADMIN_EVENT_LOG_FILTER: {
    eventType: '',
    actorId: '',
    createdAtFrom: '',
    createdAtTo: ''
  },
  createAdminEventLogAPI: vi.fn(() => ({
    listEvents: apiMocks.listAdminEventLogEvents,
    listEventTypes: apiMocks.listAdminEventLogEventTypes,
    getEvent: apiMocks.getAdminEventLogEvent
  }))
}));

vi.mock('$lib/api-client/serverState', () => ({
  getAuthenticatedServerState: apiMocks.getAuthenticatedServerState
}));

vi.mock('$lib/api-client/viewer', () => ({
  getViewerStateViaConnect: apiMocks.getViewerStateViaConnect,
  getCurrentUserViaConnect: apiMocks.getCurrentUserViaConnect,
  viewerResponseToState: (viewer: unknown) => viewer
}));

import { ServerStateStore } from './store.svelte';
import { eventBusManager, setRealtimeSocketFactoryForTests } from './eventBus.svelte';
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
    server,
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

function roomDirectoryResult(rooms: unknown[] = []) {
  return { server: { rooms } };
}

function adminRoomLayoutResult(rooms: unknown[] = [], roomGroups: unknown[] = []) {
  return { server: { rooms, roomGroups } };
}

function projectedMessage(id: string, createdAt: Date): RoomTimelineEvent {
  return new RoomTimelineEvent({
    id,
    actorId: 'U1',
    createdAt: Timestamp.fromDate(createdAt),
    event: {
      case: 'messagePosted',
      value: new RoomMessagePosted({
        message: new Message({
          id,
          roomId: 'R1',
          actorId: 'U1',
          body: id,
          createdAt: Timestamp.fromDate(createdAt)
        })
      })
    }
  });
}

beforeEach(() => {
  apiMocks.listRooms.mockResolvedValue([]);
  apiMocks.listRoomGroups.mockResolvedValue([]);
  apiMocks.listRoomMembers.mockResolvedValue({
    members: [],
    totalCount: 0,
    hasMore: false
  });
  apiMocks.listActiveCalls.mockResolvedValue([]);
  apiMocks.getActiveCall.mockResolvedValue(null);
  apiMocks.batchGetActiveCalls.mockResolvedValue([]);
  apiMocks.listCallParticipants.mockResolvedValue([]);
  apiMocks.joinCall.mockResolvedValue(true);
  apiMocks.getCallToken.mockResolvedValue(null);
  apiMocks.leaveCall.mockResolvedValue(true);
  apiMocks.listNotificationCounts.mockResolvedValue({});
  apiMocks.listNotifications.mockResolvedValue({
    items: [],
    unreadCount: 0
  });
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
    canManageUserPermissions: false,
    serverNotificationPreference: {
      level: 'DEFAULT',
      effectiveLevel: 'NORMAL'
    },
    roomNotificationPreferences: []
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

describe('ServerStateStore live server updates', () => {
  it('keeps a first-view room timeline loading while requesting it from realtime', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const hydrateRoom = vi.spyOn(eventBusManager, 'hydrateRoom');

    const messages = store.messagesForRoom('R-cold');
    store.restoreProjectedRoomWindow('R-cold');

    expect(messages.isInitialLoading).toBe(true);
    expect(store.projection.timelines.has('R-cold')).toBe(false);
    expect(store.realtimeSync.desiredRoomIds).toEqual(['R-cold']);
    expect(store.realtimeSync.retainedRoomIds).toEqual([]);
    expect(hydrateRoom).toHaveBeenCalledWith(registered.id, 'R-cold');
  });

  it('evicts an inactive timeline before hydrating a room beyond the retention limit', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const hydrateRoom = vi.spyOn(eventBusManager, 'hydrateRoom');
    for (let index = 0; index < MAX_RETAINED_ROOM_TIMELINES; index++) {
      const roomId = `R${index}`;
      store.realtimeSync.retainRoom(roomId);
      store.realtimeSync.confirmRoom(roomId);
    }
    store.projection.timelines.set('R0', new RoomTimelinePage());

    const messages = store.messagesForRoom('R-overflow');
    store.restoreProjectedRoomWindow('R-overflow');

    expect(store.projection.timelines.has('R0')).toBe(false);
    expect(store.realtimeSync.desiredRoomIds).not.toContain('R0');
    expect(store.realtimeSync.desiredRoomIds).toContain('R-overflow');
    expect(messages.isInitialLoading).toBe(true);
    expect(hydrateRoom).toHaveBeenCalledWith(registered.id, 'R-overflow');
  });

  it('applies public and authenticated server state from projection operations', async () => {
    const fake = new FakeServerConnection([roomDirectoryResult(), adminRoomLayoutResult()]);
    const publicServerInfoLoader = vi.fn<(baseUrl: string) => Promise<PublicServerInfo>>();
    publicServerInfoLoader.mockResolvedValue({
      name: 'Fresh Name',
      version: 'test',
      authorizeUrl: '/oauth/authorize',
      welcomeMessage: 'Fresh welcome',
      description: 'Fresh description',
      iconUrl: 'https://cdn/icon.webp',
      bannerUrl: 'https://cdn/banner.webp',
      directRegistrationEnabled: false,
      authProviders: [],
      compatibility: {
        protocolCapabilities: [
          'chatto.api.v1',
          'chatto.realtime.v1',
          'chatto.realtime.projection.v1'
        ],
        minimumWebClientVersion: null
      }
    });
    const store = makeStore(fake, registered, publicServerInfoLoader);
    await flushPromises();
    apiMocks.getAuthenticatedServerState.mockClear();

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    const projectionEvent = new RealtimeProjectionEvent({
      operations: [
        new RealtimeProjectionOperation({
          operation: {
            case: 'serverUpsert',
            value: new ServerPublicProfile({
              name: 'Fresh Name',
              welcomeMessage: 'Fresh welcome',
              description: 'Fresh description',
              logoUrl: 'https://cdn/icon.webp',
              bannerUrl: 'https://cdn/banner.webp'
            })
          }
        }),
        new RealtimeProjectionOperation({
          operation: {
            case: 'serverStateUpsert',
            value: new RealtimeProjectionServerState({
              motd: 'Fresh MOTD',
              runtime: new ServerRuntimeConfig({
                pushNotificationsEnabled: true,
                vapidPublicKey: 'vapid',
                livekitUrl: 'wss://livekit',
                videoProcessingEnabled: true,
                maxUploadSize: 100n,
                maxVideoUploadSize: 200n,
                messageEditWindowSeconds: 120
              })
            })
          }
        })
      ]
    });
    for (const handler of bus.projectionHandlers) {
      handler(projectionEvent);
    }

    expect(apiMocks.getAuthenticatedServerState).not.toHaveBeenCalled();
    expect(store.serverInfo.name).toBe('Fresh Name');
    expect(store.serverInfo.welcomeMessage).toBe('Fresh welcome');
    expect(store.serverInfo.description).toBe('Fresh description');
    expect(store.serverInfo.iconUrl).toBe('https://cdn/icon.webp');
    expect(store.serverInfo.bannerUrl).toBe('https://cdn/banner.webp');
    expect(store.serverInfo.motd).toBe('Fresh MOTD');
    expect(store.serverInfo.pushNotificationsEnabled).toBe(true);
    expect(store.serverInfo.livekitUrl).toBe('wss://livekit');
  });

  it('uses the projection as the authoritative active-call snapshot', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'activeCallsReplace',
                value: new RealtimeProjectionActiveCallsReplace({
                  calls: [new ActiveCall({ room: new Room({ id: 'R1' }), callId: 'call-1' })]
                })
              }
            })
          ]
        })
      );
    }

    expect(store.activeCallRooms.has('R1')).toBe(true);
    expect(apiMocks.listActiveCalls).not.toHaveBeenCalled();
  });

  it('does not inject an old mutation outside the retained room window or bump the room', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const messages = store.messagesForRoom('R1');
    const bumpRoom = vi.spyOn(store.rooms, 'bumpRoom');
    const retained = Array.from({ length: 50 }, (_, index) =>
      projectedMessage(`M${index}`, new Date(Date.UTC(2026, 0, 1, 0, 0, index)))
    );

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');
    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          id: 'SNAPSHOT',
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'roomTimelineReplace',
                value: new RealtimeProjectionRoomTimelineReplace({
                  roomId: 'R1',
                  page: new RoomTimelinePage({ events: retained }),
                  eventCursors: Object.fromEntries(
                    retained.map((event, index) => [event.id, `cursor-${index}`])
                  )
                })
              }
            })
          ]
        })
      );
    }

    const oldRoot = projectedMessage('OLD-ROOT', new Date(Date.UTC(2025, 0, 1)));
    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          id: 'REACTION-1',
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'roomTimelineEventUpsert',
                value: new RealtimeProjectionRoomTimelineEventUpsert({
                  roomId: 'R1',
                  event: oldRoot,
                  eventCursor: 'cursor-old'
                })
              }
            })
          ]
        })
      );
    }

    expect(store.projection.timelines.get('R1')?.events).toHaveLength(50);
    expect(store.projection.timelines.get('R1')?.events.some(({ id }) => id === 'OLD-ROOT')).toBe(
      false
    );
    expect(messages.events).toHaveLength(50);
    expect(messages.events.some(({ id }) => id === 'OLD-ROOT')).toBe(false);
    expect(bumpRoom).not.toHaveBeenCalled();
  });

  it('bumps an unretained room when lightweight activity arrives', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const bumpRoom = vi.spyOn(store.rooms, 'bumpRoom');

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');
    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'roomActivity',
                value: new RealtimeProjectionRoomActivity({ roomId: 'R2' })
              }
            })
          ]
        })
      );
    }

    expect(bumpRoom).toHaveBeenCalledWith('R2');
    expect(store.projection.timelines.has('R2')).toBe(false);
  });

  it('forwards RoomGroupsUpdatedEvent to public room-state stores by default', async () => {
    const fake = new FakeServerConnection([
      roomDirectoryResult([{ id: 'r1', name: 'general', description: null, archived: false }]),
      adminRoomLayoutResult(
        [{ id: 'r1', name: 'general', description: null, archived: false }],
        [{ id: 'g1', name: 'Lobby', rooms: [{ id: 'r1' }], items: [] }]
      )
    ]);
    const store = makeStore(fake);
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await Promise.resolve();
    await Promise.resolve();
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E2',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: roomEvent(RoomEventKind.RoomGroupsUpdated, { changed: true })
      });
    }
    await Promise.resolve();
    await Promise.resolve();

    expect(store.rooms.refresh).toHaveBeenCalledOnce();
    expect(store.roomDirectory.refresh).toHaveBeenCalledOnce();
    expect(store.adminRoomLayout.refresh).not.toHaveBeenCalled();
  });

  it('forwards RoomGroupsUpdatedEvent to admin room layout while active', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);
    const deactivate = store.activateAdminRoomLayout();
    await Promise.resolve();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E2-admin',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: roomEvent(RoomEventKind.RoomGroupsUpdated, { changed: true })
      });
    }
    await Promise.resolve();
    await Promise.resolve();

    expect(store.rooms.refresh).toHaveBeenCalledOnce();
    expect(store.roomDirectory.refresh).toHaveBeenCalledOnce();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);

    deactivate();
    for (const handler of bus.handlers) {
      handler({
        id: 'E2-admin-inactive',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: roomEvent(RoomEventKind.RoomGroupsUpdated, { changed: true })
      });
    }
    await Promise.resolve();

    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);
  });

  it('plays call join and leave sounds for participant events in the current active call', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.rooms.currentUserId = 'U1';
    const handleJoin = vi.spyOn(store.activeCallRooms, 'handleJoin').mockResolvedValue(undefined);
    const handleLeave = vi.spyOn(store.activeCallRooms, 'handleLeave').mockImplementation(() => {});
    const shouldPlay = vi
      .spyOn(store.voiceCall, 'callTransitionSoundDecision')
      .mockReturnValue('play');

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E-call-join',
        createdAt: new Date().toISOString(),
        actorId: 'U2',
        actor: null,
        event: roomEvent(RoomEventKind.CallParticipantJoined, { roomId: 'R1', callId: 'call-1' })
      });
      handler({
        id: 'E-call-leave',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: roomEvent(RoomEventKind.CallParticipantLeft, { roomId: 'R1', callId: 'call-1' })
      });
    }

    expect(handleJoin).toHaveBeenCalledWith('R1', 'call-1', null);
    expect(handleLeave).toHaveBeenCalledWith('R1', 'call-1', 'U1');
    expect(shouldPlay).toHaveBeenNthCalledWith(1, 'join', 'R1', 'call-1', false);
    expect(shouldPlay).toHaveBeenNthCalledWith(2, 'leave', 'R1', 'call-1', true);
    expect(soundMocks.playCallSound).toHaveBeenCalledTimes(2);
    expect(soundMocks.playCallSound).toHaveBeenNthCalledWith(1, 'join');
    expect(soundMocks.playCallSound).toHaveBeenNthCalledWith(2, 'leave');
  });

  it('clears active call snapshots when a call end event arrives through the server bus', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const handleEnd = vi.spyOn(store.activeCallRooms, 'handleEnd').mockImplementation(() => {});
    const handleCallEndedEvent = vi
      .spyOn(store.voiceCall, 'handleCallEndedEvent')
      .mockImplementation(() => {});

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E-call-ended',
        createdAt: new Date().toISOString(),
        actorId: 'U2',
        actor: null,
        event: roomEvent(RoomEventKind.CallEnded, { roomId: 'R1', callId: 'call-1' })
      });
    }

    expect(handleEnd).toHaveBeenCalledWith('R1', 'call-1');
    expect(handleCallEndedEvent).toHaveBeenCalledWith('R1', 'call-1');
  });

  it('dedupes call sound events by event ID', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.rooms.currentUserId = 'U1';
    vi.spyOn(store.voiceCall, 'callTransitionSoundDecision').mockReturnValue('play');

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      const event = {
        id: 'E-duplicate-call-join',
        createdAt: new Date().toISOString(),
        actorId: 'U2',
        actor: null,
        event: roomEvent(RoomEventKind.CallParticipantJoined, { roomId: 'R1', callId: 'call-1' })
      } as const;
      handler(event);
      handler(event);
    }

    expect(soundMocks.playCallSound).toHaveBeenCalledOnce();
    expect(soundMocks.playCallSound).toHaveBeenCalledWith('join');
  });

  it('dedupes deferred call sound events by event ID', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.rooms.currentUserId = 'U1';
    const decision = vi
      .spyOn(store.voiceCall, 'callTransitionSoundDecision')
      .mockReturnValueOnce('defer')
      .mockReturnValueOnce('play');

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      const event = {
        id: 'E-deferred-call-join',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: roomEvent(RoomEventKind.CallParticipantJoined, { roomId: 'R1', callId: 'call-1' })
      } as const;
      handler(event);
      handler(event);
    }

    expect(decision).toHaveBeenCalledOnce();
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

  it('does not play call sounds for missing-actor or inactive events', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.rooms.currentUserId = 'U1';
    const shouldPlay = vi.spyOn(store.voiceCall, 'callTransitionSoundDecision');

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    shouldPlay.mockReturnValue('play');
    for (const handler of bus.handlers) {
      handler({
        id: 'E-missing-actor',
        createdAt: new Date().toISOString(),
        actorId: null,
        actor: null,
        event: roomEvent(RoomEventKind.CallParticipantJoined, { roomId: 'R1', callId: 'call-1' })
      });
    }

    shouldPlay.mockReturnValue('skip');
    for (const handler of bus.handlers) {
      handler({
        id: 'E-stale',
        createdAt: new Date().toISOString(),
        actorId: 'U2',
        actor: null,
        event: roomEvent(RoomEventKind.CallParticipantJoined, { roomId: 'R2', callId: 'old-call' })
      });
      handler({
        id: 'E-inactive',
        createdAt: new Date().toISOString(),
        actorId: 'U2',
        actor: null,
        event: roomEvent(RoomEventKind.CallParticipantLeft, { roomId: 'R1', callId: 'call-1' })
      });
    }

    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

});
