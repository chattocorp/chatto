import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { flushSync } from 'svelte';
import type { PublicServerInfo } from '@chatto/api-client/server';
import type { AuthenticatedServerState } from '@chatto/api-client/serverState';
import { RoomEventKind } from '$lib/render/eventKinds';

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
        canAdminManageUsers: false,
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

vi.mock('@chatto/api-client/roomDirectory', () => ({
  RoomDirectoryScope: {
    ALL: 1
  },
  RoomKind: {
    CHANNEL: 1,
    DM: 2
  },
  createRoomDirectoryAPI: vi.fn(() => ({
    listRooms: apiMocks.listRooms,
    listRoomGroups: apiMocks.listRoomGroups
  }))
}));

vi.mock('@chatto/api-client/memberDirectory', () => ({
  mapDirectoryMember: (member: unknown) => member,
  createMemberDirectoryAPI: vi.fn(() => ({
    listRoomMembers: apiMocks.listRoomMembers
  }))
}));

vi.mock('@chatto/api-client/notifications', () => ({
  NotificationItemKind: {
    DirectMessage: 'directMessage',
    Mention: 'mention',
    Reply: 'reply',
    RoomMessage: 'roomMessage'
  },
  createNotificationAPI: vi.fn(() => ({
    listNotifications: apiMocks.listNotifications,
    listRoomNotifications: vi.fn(),
    hasNotifications: vi.fn(),
    listNotificationCounts: apiMocks.listNotificationCounts,
    dismissNotification: vi.fn(),
    dismissAllNotifications: vi.fn()
  }))
}));

vi.mock('@chatto/api-client/adminEventLog', () => ({
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

vi.mock('@chatto/api-client/serverState', () => ({
  getAuthenticatedServerState: apiMocks.getAuthenticatedServerState
}));

vi.mock('@chatto/api-client/viewer', () => ({
  getViewerStateViaConnect: apiMocks.getViewerStateViaConnect,
  getCurrentUserViaConnect: apiMocks.getCurrentUserViaConnect
}));

import { ServerStateStore } from './store.svelte';
import { eventBusManager, setRealtimeSocketFactoryForTests } from './eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { RegisteredServer } from './registry.svelte';

class FakeServerConnection {
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
  addedAt: 1
};

const cookieRegistered: RegisteredServer = {
  ...registered,
  token: null
};

function deferred<T = void>(): {
  promise: Promise<T>;
  resolve: (value: T | PromiseLike<T>) => void;
  reject: (reason?: unknown) => void;
} {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const stores: ServerStateStore[] = [];

function connectUnavailable() {
  return vi
    .fn<(baseUrl: string) => Promise<PublicServerInfo>>()
    .mockRejectedValue(new Error('connect unavailable'));
}

function makeStore(
  fake: FakeServerConnection,
  server: RegisteredServer = registered,
  publicServerInfoLoader = connectUnavailable()
): ServerStateStore {
  const store = new ServerStateStore(
    server,
    fake as unknown as ServerConnection,
    publicServerInfoLoader
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

beforeEach(() => {
  apiMocks.listRooms.mockResolvedValue([]);
  apiMocks.listRoomGroups.mockResolvedValue([]);
  apiMocks.listRoomMembers.mockResolvedValue({
    members: [],
    totalCount: 0,
    hasMore: false
  });
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
    canAdminManageUsers: false,
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

describe('ServerStateStore live server updates', () => {
  it('refreshes public profile and authenticated settings on ServerUpdatedEvent', async () => {
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
      authProviders: []
    });
    const store = makeStore(fake, registered, publicServerInfoLoader);
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await flushPromises();
    await Promise.resolve();
    apiMocks.getAuthenticatedServerState.mockClear();
    apiMocks.getAuthenticatedServerState.mockResolvedValueOnce({
      name: 'Store Event Test',
      version: 'test',
      logoUrl: null,
      bannerUrl: null,
      welcomeMessage: null,
      description: null,
      pushNotificationsEnabled: true,
      vapidPublicKey: 'vapid',
      livekitUrl: 'wss://livekit',
      videoProcessingEnabled: true,
      maxUploadSize: 100,
      maxVideoUploadSize: 200,
      messageEditWindowSeconds: 120,
      motd: 'Fresh MOTD',
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
    } as AuthenticatedServerState);
    fake.query.mockClear();

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E1',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: roomEvent(RoomEventKind.ServerUpdated, { name: 'stale' })
      });
    }
    await vi.waitFor(() => expect(apiMocks.getAuthenticatedServerState).toHaveBeenCalledTimes(1));

    expect(publicServerInfoLoader).toHaveBeenCalledWith(registered.url);
    expect(store.serverInfo.name).toBe('Fresh Name');
    expect(store.serverInfo.welcomeMessage).toBe('Fresh welcome');
    expect(store.serverInfo.description).toBe('Fresh description');
    expect(store.serverInfo.iconUrl).toBe('https://cdn/icon.webp');
    expect(store.serverInfo.bannerUrl).toBe('https://cdn/banner.webp');
    expect(store.serverInfo.motd).toBe('Fresh MOTD');
    expect(store.serverInfo.pushNotificationsEnabled).toBe(true);
    expect(store.serverInfo.livekitUrl).toBe('wss://livekit');
  });

  it('forwards RoomGroupsUpdatedEvent once to every room-state store', async () => {
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
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();
  });

  it('plays call join and leave sounds for participant events in the current active call', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.rooms.currentUserId = 'U1';
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

    expect(shouldPlay).toHaveBeenNthCalledWith(1, 'join', 'R1', 'call-1', false);
    expect(shouldPlay).toHaveBeenNthCalledWith(2, 'leave', 'R1', 'call-1', true);
    expect(soundMocks.playCallSound).toHaveBeenCalledTimes(2);
    expect(soundMocks.playCallSound).toHaveBeenNthCalledWith(1, 'join');
    expect(soundMocks.playCallSound).toHaveBeenNthCalledWith(2, 'leave');
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

  it('refreshes projected server state for bearer-auth sessions', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.serverInfo.livekitUrl = 'wss://livekit';
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);
    store.activeCallRooms.load = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledOnce();
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledOnce();
    expect(store.notifications.fetch).toHaveBeenCalledOnce();
    expect(store.rooms.refresh).toHaveBeenCalledOnce();
    expect(store.roomDirectory.refresh).toHaveBeenCalledOnce();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();
    expect(store.activeCallRooms.load).toHaveBeenCalledOnce();
  });

  it('refreshes projected server state for cookie-auth sessions', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake, cookieRegistered);
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await flushPromises();
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledOnce();
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledOnce();
    expect(store.notifications.fetch).toHaveBeenCalledOnce();
    expect(store.rooms.refresh).toHaveBeenCalledOnce();
    expect(store.roomDirectory.refresh).toHaveBeenCalledOnce();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();
  });

  it('runs one queued projected-state refresh after an in-flight catch-up succeeds', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const rooms = deferred();
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockReturnValueOnce(rooms.promise).mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('subscription-ended');
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.rooms.refresh).toHaveBeenCalledOnce();

    rooms.resolve();
    await vi.waitFor(() => expect(store.rooms.refresh).toHaveBeenCalledTimes(2));

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledTimes(2);
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledTimes(2);
    expect(store.notifications.fetch).toHaveBeenCalledTimes(2);
    expect(store.roomDirectory.refresh).toHaveBeenCalledTimes(2);
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);
  });

  it('runs a queued projected-state refresh after the in-flight catch-up fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const rooms = deferred();
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockReturnValueOnce(rooms.promise).mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('subscription-ended');
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.rooms.refresh).toHaveBeenCalledOnce();

    rooms.reject(new Error('network waking'));
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledTimes(2);
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledTimes(2);
    expect(store.notifications.fetch).toHaveBeenCalledTimes(2);
    expect(store.rooms.refresh).toHaveBeenCalledTimes(2);
    expect(store.roomDirectory.refresh).toHaveBeenCalledTimes(2);
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);
    expect(consoleError).toHaveBeenCalledOnce();
  });

  it('does not dedupe the next projected-state catch-up after a failed refresh', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('heartbeat-stalled');
    }
    await flushPromises();

    for (const handler of bus.catchUpHandlers) {
      handler('ws-reconnected');
    }
    await flushPromises();

    expect(store.notifications.fetch).toHaveBeenCalledTimes(2);
    expect(store.rooms.refresh).toHaveBeenCalledTimes(2);
    expect(consoleError).toHaveBeenCalledOnce();
  });
});
