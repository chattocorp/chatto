import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { NotificationLevel, PresenceStatus } from '$lib/render/types';
import { NotificationItemKind } from '$lib/api-client/notifications';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => {
  return {
    mocks: {
      getAuthenticatedServerState: vi.fn(),
      getViewerStateViaConnect: vi.fn(),
      createRoomDirectoryAPI: vi.fn(),
      listRooms: vi.fn(),
      goto: vi.fn(),
      pushState: vi.fn(),
      markNavigationServerAsRead: vi.fn().mockResolvedValue(true),
      appUi: {
        disableRoomCallWideFor: vi.fn()
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
        projection: { viewer: {} as object | null },
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
          hasAnyUnread: true,
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
          iconUrl: null as string | null,
          version: '0.5.0',
          compatibility: {
            status: 'supported',
            reason: 'capabilities-confirmed',
            missingCapabilities: [] as string[]
          }
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
  goto: mocks.goto,
  pushState: mocks.pushState
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

vi.mock('$lib/navigation/readActions', () => ({
  markNavigationServerAsRead: mocks.markNavigationServerAsRead
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
    mocks.pushState.mockClear();
    mocks.markNavigationServerAsRead.mockClear();
    mocks.markNavigationServerAsRead.mockResolvedValue(true);
    mocks.appUi.disableRoomCallWideFor.mockClear();
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
    mocks.store.projection.viewer = {};
    mocks.store.serverInfo.name = 'Loaded Remote';
    mocks.store.serverInfo.iconUrl = null;
    mocks.store.serverInfo.version = '0.5.0';
    mocks.store.serverInfo.compatibility = {
      status: 'supported',
      reason: 'capabilities-confirmed',
      missingCapabilities: []
    };
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    consoleWarnSpy.mockRestore();
  });

  it('opens server actions on right-click and marks the server as read', async () => {
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });
    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;

    icon.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 24, clientY: 36 })
    );
    await vi.waitFor(() => expect(document.body.textContent).toContain('Mark as read'));

    const markRead = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Mark as read'
    );
    await expect.element(markRead ?? null).toBeInTheDocument();
    await expect.element(markRead ?? null).toBeEnabled();
    await expect.element(q(document.body, '[role="separator"]')).toBeInTheDocument();
    markRead!.click();

    expect(mocks.markNavigationServerAsRead).toHaveBeenCalledWith('remote');
  });

  it('opens server actions from the overlaid unread badge', async () => {
    mocks.store.serverIndicator.mockReturnValue('unread');
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });
    const badge = q(container, '[data-testid="server-unread-dot"]')?.closest(
      'button'
    ) as HTMLButtonElement;

    badge.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));

    await vi.waitFor(() => expect(document.body.textContent).toContain('Mark as read'));
    await expect
      .element(q(document.body, '[role="menu"]'))
      .toHaveAttribute('aria-label', 'Actions for Loaded Remote');
  });

  it('opens the remove-server confirmation for the selected server', async () => {
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });
    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;
    icon.dispatchEvent(
      new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        clientX: 24,
        clientY: 36
      })
    );
    await vi.waitFor(() => expect(document.body.textContent).toContain('Remove server'));

    const leave = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Remove server'
    );
    await expect.element(leave ?? null).toBeInTheDocument();
    leave!.click();

    expect(mocks.pushState).toHaveBeenCalledWith('', {
      modal: { type: 'removeServer', serverId: 'remote', spaceName: 'Loaded Remote' }
    });
  });

  it('shows the server version and warns when the server is too old', async () => {
    mocks.store.serverInfo.version = '0.4.12';
    mocks.store.serverInfo.compatibility = {
      status: 'unsupported',
      reason: 'server-too-old',
      missingCapabilities: []
    };
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });

    await expect
      .element(q(container, '[data-testid="server-compatibility-warning"]'))
      .toBeInTheDocument();

    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;
    await expect.element(icon).toHaveAttribute(
      'title',
      'Loaded Remote — This server must be upgraded to Chatto 0.5 or newer before this app can connect.'
    );
    icon.dispatchEvent(
      new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        clientX: 24,
        clientY: 36
      })
    );

    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        'This server must be upgraded to Chatto 0.5 or newer before this app can connect.'
      )
    );
    expect(document.body.textContent).toContain('Version 0.4.12');

    const compatibilitySection = q(
      document.body,
      '[data-testid="server-compatibility-section"]'
    );
    expect(compatibilitySection!.classList).toContain('text-sm');
    expect(compatibilitySection!.querySelector('.text-xs')).toBeNull();
    expect(compatibilitySection!.closest('.w-80')).not.toBeNull();
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
    mocks.store.projection.viewer = null;
    mocks.showConnectionLostIcon = true;

    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    const icon = q(container, '[data-testid="server-icon"]');
    await expect.element(icon).toBeInTheDocument();
    await expect.element(icon).toHaveClass('opacity-40');
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote (connection unavailable)');
    expect(container.textContent).toContain('L');
  });

  it('renders projected private server branding without sidebar bootstrap reads', async () => {
    mocks.store.serverInfo.iconUrl = 'https://remote.example.com/assets/server/public-logo.webp';

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
      .toHaveAttribute('src', 'https://remote.example.com/assets/server/public-logo.webp');
    expect(mocks.store.notifications.fetch).not.toHaveBeenCalled();
    expect(mocks.getAuthenticatedServerState).not.toHaveBeenCalled();
    expect(mocks.getViewerStateViaConnect).not.toHaveBeenCalled();
    expect(mocks.listRooms).not.toHaveBeenCalled();
  });

  it('uses an already-hydrated projection without a loading state', async () => {
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
    expect(mocks.store.notifications.fetch).not.toHaveBeenCalled();
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

});
