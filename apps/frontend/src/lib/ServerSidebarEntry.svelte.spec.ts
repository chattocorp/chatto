import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { NotificationLevel, PresenceStatus } from '$lib/render/types';
import { NotificationItemKind } from '$lib/api-client/notifications';
import { q } from '$lib/test-utils';
import type { EventEnvelope, EventHandler } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';

const { mocks } = vi.hoisted(() => {
  const eventHandlers: EventHandler[] = [];
  return {
    mocks: {
      getAuthenticatedServerState: vi.fn(),
      getViewerStateViaConnect: vi.fn(),
      createRoomDirectoryAPI: vi.fn(),
      listRooms: vi.fn(),
      goto: vi.fn(),
      appUi: {
        disableRoomCallWideFor: vi.fn()
      },
      eventHandlers,
      registrar: {
        onEvent: vi.fn((handler: EventHandler) => {
          eventHandlers.push(handler);
          return () => {
            const index = eventHandlers.indexOf(handler);
            if (index >= 0) eventHandlers.splice(index, 1);
          };
        }),
        onRoomMarkedAsRead: vi.fn(() => vi.fn()),
        onNotificationLevelChanged: vi.fn(() => vi.fn())
      },
      showConnectionLostIcon: false,
      server: {
        id: 'remote',
        url: 'https://remote.example.com',
        name: 'Remote Chatto',
        iconUrl: null,
        token: 'token',
        userId: 'user-1',
        userLogin: 'alice',
        userDisplayName: 'Alice',
        userAvatarUrl: null,
        reauthRequiredAt: null,
        addedAt: 0
      },
      store: {
        isAuthenticated: true,
        notifications: {
          fetch: vi.fn().mockResolvedValue(undefined),
          setUnreadNotificationCount: vi.fn(),
          unreadNotificationCount: 0,
          getSpaceNotification: vi.fn().mockReturnValue(null),
          getDMNotification: vi.fn().mockReturnValue(null),
          dismiss: vi.fn(),
          getCleanPath: vi.fn().mockReturnValue('/chat/remote.example.com/room-1')
        },
        roomUnread: {
          captureSnapshotRevision: vi.fn().mockReturnValue(0),
          clear: vi.fn(),
          initRooms: vi.fn(),
          updateRooms: vi.fn(),
          resolveUnknownUnread: vi.fn(),
          setServerHasUnread: vi.fn(),
          setRoomUnread: vi.fn(),
          getFirstUnreadRoomId: vi.fn().mockReturnValue(null)
        },
        notificationLevels: {
          setServerPreference: vi.fn(),
          setRoomPreference: vi.fn(),
          isRoomMuted: vi.fn().mockReturnValue(false),
          isServerMuted: vi.fn().mockReturnValue(false)
        },
        pendingHighlights: { set: vi.fn() },
        serverInfo: {
          name: 'Chatto',
          iconUrl: null as string | null
        },
        setPermissions: vi.fn(),
        serverIndicator: vi.fn().mockReturnValue(null)
      }
    }
  };
});

vi.mock('$app/state', () => ({
  page: {
    params: {
      serverId: 'other-server',
      roomId: undefined
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '').replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('$lib/hooks', () => ({
  useTabResumeCallback: (callback: () => void) => {
    void callback();
  }
}));

vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => mocks.appUi
}));

vi.mock('$lib/eventBus.svelte', () => ({
  createEventBusHandlerRegistrar: vi.fn(() => mocks.registrar)
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: vi.fn(() => ({
      get showConnectionLostIcon() {
        return mocks.showConnectionLostIcon;
      },
      connectBaseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    }))
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: vi.fn(() => false),
    getServer: vi.fn(() => mocks.server),
    getStore: vi.fn(() => mocks.store)
  }
}));

vi.mock('$lib/api-client/serverState', () => ({
  getAuthenticatedServerState: mocks.getAuthenticatedServerState
}));

vi.mock('$lib/api-client/viewer', () => ({
  getViewerStateViaConnect: mocks.getViewerStateViaConnect
}));

vi.mock('$lib/api-client/roomDirectory', () => ({
  RoomDirectoryScope: {
    ALL: 1,
    CHANNELS: 2,
    DMS: 3
  },
  RoomKind: {
    CHANNEL: 1,
    DM: 2
  },
  createRoomDirectoryAPI: mocks.createRoomDirectoryAPI
}));

import ServerSidebarEntry from './ServerSidebarEntry.svelte';

function serverState(overrides: Record<string, unknown> = {}) {
  return {
    name: 'Loaded Remote',
    logoUrl: null,
    viewerHasUnreadRooms: false,
    ...overrides
  };
}

function viewerState(overrides: Record<string, unknown> = {}) {
  return {
    user: {
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      presenceStatus: PresenceStatus.Online,
      hasVerifiedEmail: true
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
    serverNotificationPreference: {
      level: NotificationLevel.Default,
      effectiveLevel: NotificationLevel.Normal
    },
    roomNotificationPreferences: [],
    ...overrides
  };
}

function dispatchServerEvent(event: Record<string, unknown>, actorId = 'other-user') {
  const handler = mocks.eventHandlers[0];
  if (!handler) throw new Error('ServerSidebarEntry event handler was not registered');
  handler({
    id: 'event-1',
    createdAt: new Date().toISOString(),
    actorId,
    actor: null,
    event
  } as EventEnvelope);
}

describe('ServerSidebarEntry', () => {
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;
  let consoleWarnSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleErrorSpy?.mockRestore();
    consoleWarnSpy?.mockRestore();
    consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
    mocks.showConnectionLostIcon = false;
    mocks.getAuthenticatedServerState.mockReset();
    mocks.getViewerStateViaConnect.mockReset();
    mocks.createRoomDirectoryAPI.mockReset();
    mocks.listRooms.mockReset();
    mocks.goto.mockClear();
    mocks.appUi.disableRoomCallWideFor.mockClear();
    mocks.eventHandlers.length = 0;
    mocks.registrar.onEvent.mockClear();
    mocks.registrar.onRoomMarkedAsRead.mockClear();
    mocks.registrar.onNotificationLevelChanged.mockClear();
    mocks.getAuthenticatedServerState.mockResolvedValue(serverState());
    mocks.getViewerStateViaConnect.mockResolvedValue(viewerState());
    mocks.store.isAuthenticated = true;
    mocks.listRooms.mockResolvedValue([]);
    mocks.createRoomDirectoryAPI.mockReturnValue({ listRooms: mocks.listRooms });
    mocks.store.notifications.fetch.mockClear();
    mocks.store.notifications.fetch.mockResolvedValue(undefined);
    mocks.store.notifications.setUnreadNotificationCount.mockClear();
    mocks.store.notifications.unreadNotificationCount = 0;
    mocks.store.notifications.getSpaceNotification.mockReturnValue(null);
    mocks.store.notifications.getDMNotification.mockReturnValue(null);
    mocks.store.notifications.dismiss.mockClear();
    mocks.store.notifications.getCleanPath.mockReturnValue('/chat/remote.example.com/room-1');
    mocks.store.roomUnread.clear.mockClear();
    mocks.store.roomUnread.captureSnapshotRevision.mockClear();
    mocks.store.roomUnread.captureSnapshotRevision.mockReturnValue(0);
    mocks.store.roomUnread.initRooms.mockClear();
    mocks.store.roomUnread.updateRooms.mockClear();
    mocks.store.roomUnread.resolveUnknownUnread.mockClear();
    mocks.store.roomUnread.setServerHasUnread.mockClear();
    mocks.store.roomUnread.setRoomUnread.mockClear();
    mocks.store.notificationLevels.setServerPreference.mockClear();
    mocks.store.notificationLevels.setRoomPreference.mockClear();
    mocks.store.setPermissions.mockClear();
    mocks.store.serverIndicator.mockReturnValue(null);
    mocks.store.serverInfo.name = 'Chatto';
    mocks.store.serverInfo.iconUrl = null;
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    consoleWarnSpy.mockRestore();
  });

  it('renders an unauthenticated server without loading private sidebar state', async () => {
    mocks.store.isAuthenticated = false;
    mocks.store.serverInfo.iconUrl = 'https://remote.example.com/assets/server/logo.webp';

    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote'
      }
    });

    const icon = q(container, '[data-testid="server-icon"]');
    const image = q(container, '[data-testid="server-icon"] img');
    await expect.element(icon).toBeInTheDocument();
    await expect.element(icon).toHaveAttribute('href', '/chat/remote.example.com');
    await expect
      .element(image)
      .toHaveAttribute('src', 'https://remote.example.com/assets/server/logo.webp');
    expect(mocks.getAuthenticatedServerState).not.toHaveBeenCalled();
    expect(mocks.getViewerStateViaConnect).not.toHaveBeenCalled();
    expect(mocks.store.notifications.fetch).not.toHaveBeenCalled();
  });

  it('keeps a failed server in the gutter as a dimmed icon', async () => {
    mocks.getAuthenticatedServerState.mockRejectedValue(new Error('connection refused'));

    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    await vi.waitFor(() => {
      expect(mocks.getAuthenticatedServerState).toHaveBeenCalled();
    });

    const icon = q(container, '[data-testid="server-icon"]');
    await expect.element(icon).toBeInTheDocument();
    await expect.element(icon).toHaveClass('opacity-40');
    await expect.element(icon).toHaveAttribute('title', 'Remote Chatto (connection unavailable)');
    expect(container.textContent).toContain('R');
  });

  it('removes the dimmed state after sidebar init succeeds', async () => {
    mocks.store.serverInfo.iconUrl = 'https://remote.example.com/assets/server/public-logo.webp';
    mocks.store.notifications.fetch.mockImplementationOnce(async () => {
      mocks.store.notifications.setUnreadNotificationCount(3);
    });
    mocks.getAuthenticatedServerState.mockResolvedValue(
      serverState({
        logoUrl: 'https://remote.example.com/assets/server/private-logo.webp',
        viewerHasUnreadRooms: true
      })
    );
    mocks.getViewerStateViaConnect.mockResolvedValue(
      viewerState({
        canViewAdmin: true,
        serverNotificationPreference: {
          level: NotificationLevel.AllMessages,
          effectiveLevel: NotificationLevel.AllMessages
        },
        roomNotificationPreferences: [
          {
            roomId: 'dm-1',
            level: NotificationLevel.Muted,
            effectiveLevel: NotificationLevel.Muted
          }
        ]
      })
    );
    mocks.listRooms.mockResolvedValue([{ id: 'dm-1', kind: 2, hasUnread: true }]);

    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    const icon = q(container, '[data-testid="server-icon"]');
    const image = q(container, '[data-testid="server-icon"] img');
    await expect.element(icon).toBeInTheDocument();
    await expect.element(icon).not.toHaveClass('opacity-40');
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote');
    await expect
      .element(image)
      .toHaveAttribute('src', 'https://remote.example.com/assets/server/private-logo.webp');
    await vi.waitFor(() => {
      expect(mocks.store.notifications.setUnreadNotificationCount).toHaveBeenLastCalledWith(3);
    });
    expect(mocks.store.notifications.fetch).toHaveBeenCalled();
    expect(mocks.getAuthenticatedServerState).toHaveBeenCalledWith({
      serverId: 'remote',
      baseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    });
    expect(mocks.getViewerStateViaConnect).toHaveBeenCalledWith({
      serverId: 'remote',
      baseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    });
    expect(mocks.store.setPermissions).toHaveBeenCalledWith(
      expect.objectContaining({ canViewAdmin: true, canStartDMs: true })
    );
    expect(mocks.store.notificationLevels.setServerPreference).toHaveBeenCalledWith(
      NotificationLevel.AllMessages,
      NotificationLevel.AllMessages
    );
    expect(mocks.store.notificationLevels.setRoomPreference).toHaveBeenCalledWith(
      'dm-1',
      NotificationLevel.Muted,
      NotificationLevel.Muted
    );
    expect(mocks.store.roomUnread.initRooms).toHaveBeenCalledWith(
      [{ id: 'dm-1', kind: 2, hasUnread: true }],
      true,
      0
    );
  });

  it('keeps sidebar init usable when notification fetch returns no count changes', async () => {
    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    const icon = q(container, '[data-testid="server-icon"]');
    await expect.element(icon).toBeInTheDocument();
    await expect.element(icon).not.toHaveClass('opacity-40');
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote');
    await vi.waitFor(() => {
      expect(mocks.store.notifications.fetch).toHaveBeenCalled();
    });
    expect(consoleWarnSpy).not.toHaveBeenCalled();
  });

  it('reveals the target room before navigating from a server notification indicator', async () => {
    const notification = {
      id: 'mention-1',
      kind: NotificationItemKind.Mention,
      mentionRoom: { id: 'room-1', name: 'general' },
      mentionEventId: 'event-1',
      mentionInThread: 'thread-1'
    };
    mocks.store.serverIndicator.mockReturnValue('notification');
    mocks.store.notifications.unreadNotificationCount = 1;
    mocks.store.notifications.getSpaceNotification.mockReturnValue(notification);
    mocks.store.notifications.getCleanPath.mockReturnValue(
      '/chat/remote.example.com/room-1/thread-1'
    );

    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    const badge = q(container, '[data-testid="server-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('remote', 'room-1');
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
      );
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'room-1',
        'thread-1',
        'event-1'
      );
      expect(mocks.store.notifications.dismiss).toHaveBeenCalledWith('mention-1');
      expect(mocks.goto).toHaveBeenCalledWith('/chat/remote.example.com/room-1/thread-1');
    });
  });

  it('marks remote rooms unread from local message event kind', async () => {
    render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    await vi.waitFor(() => {
      expect(mocks.eventHandlers).toHaveLength(1);
    });

    dispatchServerEvent({
      kind: RoomEventKind.MessagePosted,
      roomId: 'room-1',
      threadRootEventId: null
    });

    expect(mocks.store.roomUnread.setRoomUnread).toHaveBeenCalledWith('room-1', true);
  });
});
