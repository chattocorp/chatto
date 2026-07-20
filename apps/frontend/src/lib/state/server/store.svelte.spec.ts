import { Timestamp } from '@bufbuild/protobuf';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { flushSync } from 'svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { AuthenticatedServerState } from '$lib/api-client/serverState';
import type { RoomFileItem } from '$lib/api-client/attachments';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { ServerRuntimeConfig } from '@chatto/api-types/api/v1/server_state_pb';
import { ActiveCall, CallParticipant } from '@chatto/api-types/api/v1/voice_calls_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { Message, MessageAttachment } from '@chatto/api-types/api/v1/message_types_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import {
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
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
  RealtimeProjectionRoomViewerStateReplace,
  RealtimeProjectionReactionChange,
  RealtimeProjectionRoomTimelineEventRemove,
  RealtimeProjectionRoomTimelineEventUpsert,
  RealtimeProjectionRoomTimelineReplace,
  RealtimeProjectionServerState,
  RealtimeProjectionReset,
  RealtimeProjectionRoom,
  RealtimeProjectionUserRemove
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
    ),
    listRoomAttachments: vi.fn<
      () => Promise<{ items: RoomFileItem[]; totalCount: number; hasMore: boolean }>
    >(() => Promise.resolve({ items: [], totalCount: 0, hasMore: false })),
    refreshAssetUrls: vi.fn(() => Promise.resolve(new Map()))
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

function projectedMessage(
  id: string,
  createdAt: Date,
  attachmentIds: string[] = []
): RoomTimelineEvent {
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
          createdAt: Timestamp.fromDate(createdAt),
          attachments: attachmentIds.map(
            (attachmentId) =>
              new MessageAttachment({
                id: attachmentId,
                filename: `${attachmentId}.jpg`,
                contentType: 'image/jpeg'
              })
          )
        })
      })
    }
  });
}

function projectedRoomFile(attachmentId = 'A1', messageEventId = 'M1'): RoomFileItem {
  return {
    messageEventId,
    threadRootEventId: 'ROOT-1',
    createdAt: '2026-07-19T12:00:00.000Z',
    attachment: {
      id: attachmentId,
      filename: `${attachmentId}.jpg`,
      contentType: 'image/jpeg',
      width: 0,
      height: 0,
      assetUrl: null,
      thumbnailAssetUrl: null,
      videoProcessing: null
    }
  };
}

beforeEach(() => {
  apiMocks.listRooms.mockResolvedValue([]);
  apiMocks.listRoomGroups.mockResolvedValue([]);
  apiMocks.listRoomMembers.mockResolvedValue({
    members: [],
    totalCount: 0,
    hasMore: false
  });
  apiMocks.listRoomAttachments.mockReset();
  apiMocks.listRoomAttachments.mockResolvedValue({ items: [], totalCount: 0, hasMore: false });
  apiMocks.refreshAssetUrls.mockReset();
  apiMocks.refreshAssetUrls.mockResolvedValue(new Map());
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
  it('refreshes a mounted admin room layout after remote projection changes', async () => {
    vi.useFakeTimers();
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);
    const deactivate = store.activateAdminRoomLayout();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id)!;
    const remoteRoom = new RealtimeProjectionRoom({
      room: new RoomWithViewerState({ room: new Room({ id: 'R-remote', name: 'remote-room' }) })
    });
    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: { case: 'roomUpsert', value: remoteRoom }
            })
          ]
        })
      );
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: { case: 'roomUpsert', value: remoteRoom }
            })
          ]
        })
      );
    }

    await vi.advanceTimersByTimeAsync(49);
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);

    deactivate();
    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: { case: 'roomUpsert', value: remoteRoom }
            })
          ]
        })
      );
    }
    await vi.advanceTimersByTimeAsync(100);
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });

  it('clears every projection-derived mirror immediately on reset', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id)!;

    store.notifications.replaceProjection({
      items: [
        {
          kind: 'mention',
          id: 'N1',
          createdAt: '2026-01-01T00:00:00Z',
          summary: 'Alice mentioned you',
          mentionRoom: { id: 'R1', name: 'general' },
          mentionEventId: 'M1'
        } as never
      ],
      totalCount: 1
    });
    store.activeCallRooms.replaceProjection([
      new ActiveCall({ room: new Room({ id: 'R1' }), callId: 'call-1' })
    ]);
    store.notificationLevels.setServerPreference('MUTED' as never, 'MUTED' as never);
    store.roomUnread.setRoomUnread('R1', true);
    store.setPermissions({ canViewAdmin: true } as never);
    store.serverInfo.applyProjectionState(
      new RealtimeProjectionServerState({
        motd: 'private MOTD',
        runtime: new ServerRuntimeConfig({
          pushNotificationsEnabled: true,
          livekitUrl: 'wss://livekit'
        })
      })
    );
    store.rooms.rooms = [{ id: 'R1' } as never];
    store.rooms.roomGroups = [{ id: 'G1' } as never];
    store.rooms.isInitialLoading = false;
    store.roomDirectory.allRooms = [{ id: 'R1' } as never];
    store.roomDirectory.isLoading = false;
    store.currentUser.loading = false;

    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: { case: 'reset', value: new RealtimeProjectionReset() }
            })
          ]
        })
      );
    }

    expect(store.notifications.notifications).toEqual([]);
    expect(store.notifications.unreadNotificationCount).toBe(0);
    expect(store.notifications.hasLoaded).toBe(true);
    expect(store.activeCallRooms.has('R1')).toBe(false);
    expect(store.notificationLevels.isServerMuted()).toBe(false);
    expect(store.roomUnread.hasAnyUnread).toBe(false);
    expect(store.permissions.loaded).toBe(false);
    expect(store.permissions.canViewAdmin).toBe(false);
    expect(store.serverInfo.motd).toBeNull();
    expect(store.serverInfo.pushNotificationsEnabled).toBe(false);
    expect(store.serverInfo.livekitUrl).toBeNull();
    expect(store.rooms.rooms).toEqual([]);
    expect(store.rooms.roomGroups).toEqual([]);
    expect(store.rooms.isInitialLoading).toBe(true);
    expect(store.roomDirectory.allRooms).toEqual([]);
    expect(store.roomDirectory.isLoading).toBe(true);
    expect(store.currentUser.loading).toBe(true);

    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'serverStateUpsert',
                value: new RealtimeProjectionServerState({
                  motd: 'rehydrated',
                  runtime: new ServerRuntimeConfig({ livekitUrl: 'wss://fresh' })
                })
              }
            }),
            new RealtimeProjectionOperation({
              operation: {
                case: 'activeCallsReplace',
                value: new RealtimeProjectionActiveCallsReplace({
                  calls: [new ActiveCall({ room: new Room({ id: 'R2' }), callId: 'call-2' })]
                })
              }
            })
          ]
        })
      );
    }
    expect(store.serverInfo.motd).toBe('rehydrated');
    expect(store.serverInfo.livekitUrl).toBe('wss://fresh');
    expect(store.activeCallRooms.has('R2')).toBe(true);
  });

  it('purges removed users from navigation and retained render stores', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const messages = store.messagesForRoom('R1');
    const message = projectedMessage('M1', new Date('2026-01-01T00:00:00Z'));
    messages.events = [
      {
        id: message.id,
        createdAt: '2026-01-01T00:00:00Z',
        actorId: 'U2',
        actor: { id: 'U2', displayName: 'Deleted Person' },
        event: {
          kind: 'messagePosted',
          roomId: 'R1',
          body: 'hello',
          attachments: [],
          reactions: [],
          replyCount: 0,
          threadParticipants: []
        }
      } as never
    ];
    store.projection.viewer = {
      user: { id: 'U1' },
      serverNotificationPreference: { level: 'DEFAULT', effectiveLevel: 'NORMAL' },
      roomNotificationPreferences: []
    } as never;
    store.projection.users.set(
      'U2',
      new DirectoryMember({ user: new User({ id: 'U2', displayName: 'Deleted Person' }) })
    );
    store.projection.rooms.set(
      'R1',
      new RealtimeProjectionRoom({
        room: new RoomWithViewerState({ room: new Room({ id: 'R1' }) }),
        memberUserIds: ['U2']
      })
    );
    const replaceNavigation = vi.spyOn(store.rooms, 'replaceProjection');

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id)!;
    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'userRemove',
                value: new RealtimeProjectionUserRemove({ userId: 'U2' })
              }
            })
          ]
        })
      );
    }

    expect(store.projection.users.has('U2')).toBe(false);
    expect(store.projection.rooms.get('R1')?.memberUserIds).toEqual([]);
    expect(replaceNavigation).toHaveBeenCalled();
    const membersByRoom = replaceNavigation.mock.calls.at(-1)?.[3];
    expect(membersByRoom?.get('R1')).toEqual([]);
    expect(messages.events[0]).toMatchObject({ actorId: 'U2', actor: null });
  });

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

  it('scrubs retained plaintext on membership loss and restores the same mounted room store', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const messages = store.messagesForRoom('R1');
    store.realtimeSync.retainRoom('R1');
    store.realtimeSync.confirmRoom('R1');
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id)!;
    const dispatch = (projectionEvent: RealtimeProjectionEvent) => {
      for (const handler of bus.projectionHandlers) handler(projectionEvent);
    };
    const room = (isMember: boolean) =>
      new RealtimeProjectionRoom({
        room: new RoomWithViewerState({
          room: new Room({ id: 'R1' }),
          viewerState: new RoomViewerState({ isMember })
        })
      });

    dispatch(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: { case: 'roomUpsert', value: room(true) }
          }),
          new RealtimeProjectionOperation({
            operation: {
              case: 'roomTimelineReplace',
              value: new RealtimeProjectionRoomTimelineReplace({
                roomId: 'R1',
                page: new RoomTimelinePage({
                  events: [projectedMessage('M-secret', new Date('2026-01-01T00:00:00Z'))]
                })
              })
            }
          })
        ]
      })
    );
    expect(messages.events.map(({ id }) => id)).toEqual(['M-secret']);

    // The room upsert alone is sufficient to revoke plaintext. The server also
    // sends an empty timeline replacement, but the client fails closed if a
    // future or mixed-version sender omits it.
    dispatch(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: { case: 'roomUpsert', value: room(false) }
          })
        ]
      })
    );
    expect(store.projection.timelines.has('R1')).toBe(false);
    expect(messages.events).toEqual([]);
    expect(messages.isInitialLoading).toBe(false);
    expect(store.realtimeSync.desiredRoomIds).toEqual(['R1']);
    expect(store.realtimeSync.retainedRoomIds).toEqual(['R1']);

    store.activeCallRooms.replaceProjection([
      new ActiveCall({ room: new Room({ id: 'R1' }), callId: 'call-secret' })
    ]);
    dispatch(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: { case: 'roomUpsert', value: room(false) }
          }),
          new RealtimeProjectionOperation({
            operation: {
              case: 'roomTimelineReplace',
              value: new RealtimeProjectionRoomTimelineReplace({
                roomId: 'R1',
                page: new RoomTimelinePage()
              })
            }
          })
        ]
      })
    );
    // Even a later stale replacement cannot reopen the canonical or mirrored
    // timeline before an explicit positive membership operation arrives.
    dispatch(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: {
              case: 'roomTimelineReplace',
              value: new RealtimeProjectionRoomTimelineReplace({
                roomId: 'R1',
                page: new RoomTimelinePage({
                  events: [projectedMessage('M-stale', new Date('2026-01-01T00:00:01Z'))]
                })
              })
            }
          })
        ]
      })
    );
    expect(messages.events).toEqual([]);
    expect(store.projection.timelines.has('R1')).toBe(false);
    expect(store.activeCallRooms.has('R1')).toBe(false);

    dispatch(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: { case: 'roomUpsert', value: room(true) }
          }),
          new RealtimeProjectionOperation({
            operation: {
              case: 'roomTimelineReplace',
              value: new RealtimeProjectionRoomTimelineReplace({
                roomId: 'R1',
                page: new RoomTimelinePage({
                  events: [projectedMessage('M-restored', new Date('2026-01-02T00:00:00Z'))]
                })
              })
            }
          })
        ]
      })
    );
    expect(store.messagesForRoom('R1')).toBe(messages);
    expect(messages.events.map(({ id }) => id)).toEqual(['M-restored']);

    dispatch(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: {
              case: 'roomViewerStateReplace',
              value: new RealtimeProjectionRoomViewerStateReplace({
                roomId: 'R1',
                viewerState: new RoomViewerState({ isMember: false })
              })
            }
          })
        ]
      })
    );
    expect(store.projection.timelines.has('R1')).toBe(false);
    expect(messages.events).toEqual([]);
  });

  it('releases decrypted thread stores after their final mounted consumer', () => {
    const store = makeStore(new FakeServerConnection([]));
    const first = store.messagesForThread('R1', 'T1');
    store.retainMessagesForThread('R1', 'T1', first);
    store.retainMessagesForThread('R1', 'T1', first);
    store.releaseMessagesForThread('R1', 'T1', first);
    expect(store.messagesForThread('R1', 'T1')).toBe(first);

    store.releaseMessagesForThread('R1', 'T1', first);
    expect(store.messagesForThread('R1', 'T1')).not.toBe(first);
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
  });

  it('owns one lazy file cache per room', async () => {
    const store = makeStore(new FakeServerConnection([]));
    const files = store.filesForRoom('R1');

    expect(store.filesForRoom('R1')).toBe(files);
    expect(files.items).toEqual([]);
    expect(apiMocks.listRoomAttachments).not.toHaveBeenCalled();

    await files.hydrate();

    expect(apiMocks.listRoomAttachments).toHaveBeenCalledOnce();
  });

  it('reconciles realtime message attachments into a hydrated room file cache', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const files = store.filesForRoom('R1');
    await files.hydrate();
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          id: 'M1',
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'roomTimelineEventUpsert',
                value: new RealtimeProjectionRoomTimelineEventUpsert({
                  roomId: 'R1',
                  event: projectedMessage('M1', new Date('2026-07-19T12:00:00Z'), ['A1'])
                })
              }
            })
          ]
        })
      );
    }

    expect(files.items.map((item) => item.attachment.id)).toEqual(['A1']);
    expect(apiMocks.listRoomAttachments).toHaveBeenCalledOnce();

    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          id: 'EDIT-1',
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'roomTimelineEventUpsert',
                value: new RealtimeProjectionRoomTimelineEventUpsert({
                  roomId: 'R1',
                  event: projectedMessage('M1', new Date('2026-07-19T12:00:00Z'))
                })
              }
            })
          ]
        })
      );
    }

    expect(files.items).toEqual([]);
    expect(apiMocks.listRoomAttachments).toHaveBeenCalledOnce();
  });

  it('ignores reaction upserts and projection-only row removals for room files', async () => {
    apiMocks.listRoomAttachments.mockResolvedValue({
      items: [projectedRoomFile()],
      totalCount: 1,
      hasMore: false
    });
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const files = store.filesForRoom('R1');
    await files.hydrate();
    const applyTimelineEvent = vi.spyOn(files, 'applyTimelineEvent');
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

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
                  event: projectedMessage('M1', new Date('2026-07-19T12:00:00Z'), ['A1']),
                  reactionChange: new RealtimeProjectionReactionChange()
                })
              }
            })
          ]
        })
      );
      handler(
        new RealtimeProjectionEvent({
          id: 'ECHO-REMOVED-1',
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'roomTimelineEventRemove',
                value: new RealtimeProjectionRoomTimelineEventRemove({
                  roomId: 'R1',
                  eventId: 'M1'
                })
              }
            })
          ]
        })
      );
    }

    expect(applyTimelineEvent).not.toHaveBeenCalled();
    expect(files.items.map((item) => item.attachment.id)).toEqual(['A1']);
    expect(apiMocks.listRoomAttachments).toHaveBeenCalledOnce();
  });

  it('restores retained room files only after an explicit positive access grant', async () => {
    apiMocks.listRoomAttachments
      .mockResolvedValueOnce({
        items: [projectedRoomFile()],
        totalCount: 1,
        hasMore: false
      })
      .mockResolvedValueOnce({
        items: [projectedRoomFile('A2', 'M2')],
        totalCount: 1,
        hasMore: false
      });
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const files = store.filesForRoom('R1');
    const release = files.retain();
    await vi.waitFor(() => expect(files.items.map((item) => item.attachment.id)).toEqual(['A1']));
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');
    const dispatch = (operation: RealtimeProjectionOperation) => {
      for (const handler of bus.projectionHandlers) {
        handler(new RealtimeProjectionEvent({ operations: [operation] }));
      }
    };

    dispatch(
      new RealtimeProjectionOperation({
        operation: {
          case: 'roomUpsert',
          value: new RealtimeProjectionRoom({
            room: new RoomWithViewerState({
              room: new Room({ id: 'R1' }),
              viewerState: new RoomViewerState({ isMember: false })
            })
          })
        }
      })
    );
    expect(files.items).toEqual([]);

    dispatch(
      new RealtimeProjectionOperation({
        operation: {
          case: 'roomUpsert',
          value: new RealtimeProjectionRoom({
            room: new RoomWithViewerState({ room: new Room({ id: 'R1' }) })
          })
        }
      })
    );
    dispatch(
      new RealtimeProjectionOperation({
        operation: {
          case: 'roomViewerStateReplace',
          value: new RealtimeProjectionRoomViewerStateReplace({ roomId: 'R1' })
        }
      })
    );
    expect(apiMocks.listRoomAttachments).toHaveBeenCalledOnce();
    expect(files.items).toEqual([]);

    dispatch(
      new RealtimeProjectionOperation({
        operation: {
          case: 'roomViewerStateReplace',
          value: new RealtimeProjectionRoomViewerStateReplace({
            roomId: 'R1',
            viewerState: new RoomViewerState({ isMember: true })
          })
        }
      })
    );
    await vi.waitFor(() => expect(files.items.map((item) => item.attachment.id)).toEqual(['A2']));
    expect(apiMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    release();
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

  it('derives call join and leave effects from active-call projection replacements', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.rooms.currentUserId = 'U1';
    const shouldPlay = vi
      .spyOn(store.voiceCall, 'callTransitionSoundDecision')
      .mockReturnValue('play');
    const handleParticipantLeftEvent = vi
      .spyOn(store.voiceCall, 'handleParticipantLeftEvent')
      .mockImplementation(() => {});
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id)!;
    const participant = new CallParticipant({
      user: new User({ id: 'U2', login: 'bob', displayName: 'Bob' })
    });

    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          id: 'E-call-base',
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
      handler(
        new RealtimeProjectionEvent({
          id: 'E-call-join',
          actorId: 'U2',
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'activeCallsReplace',
                value: new RealtimeProjectionActiveCallsReplace({
                  calls: [
                    new ActiveCall({
                      room: new Room({ id: 'R1' }),
                      callId: 'call-1',
                      participants: [participant]
                    })
                  ]
                })
              }
            })
          ]
        })
      );
      handler(
        new RealtimeProjectionEvent({
          id: 'E-call-leave',
          actorId: 'U2',
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

    expect(shouldPlay).toHaveBeenNthCalledWith(1, 'join', 'R1', 'call-1', false);
    expect(shouldPlay).toHaveBeenNthCalledWith(2, 'leave', 'R1', 'call-1', false);
    expect(soundMocks.playCallSound).toHaveBeenNthCalledWith(1, 'join');
    expect(soundMocks.playCallSound).toHaveBeenNthCalledWith(2, 'leave');
    expect(handleParticipantLeftEvent).toHaveBeenCalledWith('R1', 'call-1', 'U2', 'U1');
  });

  it('disconnects a locally connected call when its projection disappears', () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.voiceCall.roomId = 'R1';
    const handleCallEndedEvent = vi
      .spyOn(store.voiceCall, 'handleCallEndedEvent')
      .mockImplementation(() => {});
    const shouldPlay = vi.spyOn(store.voiceCall, 'callTransitionSoundDecision');
    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id)!;

    for (const handler of bus.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'activeCallsReplace',
                value: new RealtimeProjectionActiveCallsReplace({
                  calls: [
                    new ActiveCall({
                      room: new Room({ id: 'R1' }),
                      callId: 'call-1',
                      participants: [
                        new CallParticipant({
                          user: new User({ id: 'U2', login: 'bob', displayName: 'Bob' })
                        })
                      ]
                    })
                  ]
                })
              }
            })
          ]
        })
      );
      handler(
        new RealtimeProjectionEvent({
          id: 'E-call-end',
          actorId: 'U2',
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'activeCallsReplace',
                value: new RealtimeProjectionActiveCallsReplace()
              }
            })
          ]
        })
      );
    }

    expect(handleCallEndedEvent).toHaveBeenCalledWith('R1', 'call-1');
    expect(shouldPlay).not.toHaveBeenCalled();
  });
});
