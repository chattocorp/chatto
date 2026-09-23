import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { SvelteSet } from 'svelte/reactivity';

import { NotificationSignalKind } from '$lib/api-client/notifications';
import { q } from '$lib/test-utils';
import { page } from '$app/state';

const connectionLostServers = new SvelteSet<string>();

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
      startRemoteReauthentication: vi.fn(),
      recoverServer: vi.fn().mockResolvedValue(undefined),
      beginOriginReauthentication: vi.fn(),
      isOriginServer: vi.fn(() => false),
      writeClipboardText: vi.fn(),
      toastError: vi.fn(),
      toastSuccess: vi.fn(),
      appUi: {
        disableRoomCallWideFor: vi.fn()
      },
      showConnectionLostIcon: false,
      server: {
        id: 'remote',
        url: 'https://remote.example.com',
        name: 'Remote Chatto',
        iconUrl: null,
        token: 'token' as string | null,
        userId: 'user-1',
        userLogin: 'alice',
        userDisplayName: 'Alice',
        userAvatarUrl: null,
        reauthRequiredAt: null as number | null,
        addedAt: 0
      },
      store: {
        savedView: null,
        networkStartupDeferred: false,
        restoreSavedView: vi.fn(),
        isAuthenticated: true,
        projection: { viewer: {} as object | null },
        notifications: {
          fetch: vi.fn().mockResolvedValue(undefined),
          setUnreadNotificationCount: vi.fn(),
          unreadNotificationCount: 0,
          importantUnreadNotificationCount: 0,
          get attention() {
            return {
              unreadNotificationCount: this.unreadNotificationCount,
              importantUnreadNotificationCount: this.importantUnreadNotificationCount
            };
          },
          getNonDMNotification: vi.fn().mockReturnValue(null),
          getDMNotification: vi.fn().mockReturnValue(null),
          markRead: vi.fn(),
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
        pendingHighlights: { set: vi.fn() },
        serverInfo: {
          name: 'Chatto',
          iconUrl: null as string | null,
          version: '0.5.0',
          compatibility: {
            status: 'supported',
            reason: 'version-confirmed'
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
    data: { serverInfo: { setupRequired: false } },
    route: { id: '/setup' },
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
        return connectionLostServers.has('remote') || mocks.showConnectionLostIcon;
      },
      connectBaseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    }))
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    needsRecovery: () => Boolean(mocks.server.token && mocks.server.reauthRequiredAt === null && !mocks.store.isAuthenticated),
    recoverServer: mocks.recoverServer,
    isOriginServer: mocks.isOriginServer,
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

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication,
  beginOriginReauthentication: mocks.beginOriginReauthentication
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { error: mocks.toastError, success: mocks.toastSuccess }
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
      presenceStatus: PresenceStatus.ONLINE,
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
    connectionLostServers.clear();
    mocks.getAuthenticatedServerState.mockReset();
    mocks.getViewerStateViaConnect.mockReset();
    mocks.createRoomDirectoryAPI.mockReset();
    mocks.listRooms.mockReset();
    mocks.goto.mockClear();
    mocks.pushState.mockClear();
    mocks.markNavigationServerAsRead.mockClear();
    mocks.markNavigationServerAsRead.mockResolvedValue(true);
    mocks.startRemoteReauthentication.mockReset();
    mocks.startRemoteReauthentication.mockResolvedValue(undefined);
    mocks.beginOriginReauthentication.mockReset();
    mocks.isOriginServer.mockReset();
    mocks.isOriginServer.mockReturnValue(false);
    mocks.server.url = 'https://remote.example.com';
    mocks.server.reauthRequiredAt = null;
    mocks.server.token = 'token';
    mocks.recoverServer.mockReset();
    mocks.recoverServer.mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: mocks.writeClipboardText },
      configurable: true
    });
    mocks.writeClipboardText.mockReset();
    mocks.writeClipboardText.mockResolvedValue(undefined);
    mocks.toastError.mockReset();
    mocks.toastSuccess.mockReset();
    mocks.appUi.disableRoomCallWideFor.mockClear();
    mocks.getAuthenticatedServerState.mockResolvedValue(serverState());
    mocks.getViewerStateViaConnect.mockResolvedValue(viewerState());
    mocks.store.isAuthenticated = true;
    mocks.store.networkStartupDeferred = false;
    mocks.listRooms.mockResolvedValue([]);
    mocks.createRoomDirectoryAPI.mockReturnValue({ listRooms: mocks.listRooms });
    mocks.store.notifications.fetch.mockClear();
    mocks.store.notifications.fetch.mockResolvedValue(undefined);
    mocks.store.notifications.setUnreadNotificationCount.mockClear();
    mocks.store.notifications.unreadNotificationCount = 0;
    mocks.store.notifications.importantUnreadNotificationCount = 0;
    mocks.store.notifications.getNonDMNotification.mockReturnValue(null);
    mocks.store.notifications.getDMNotification.mockReturnValue(null);
    mocks.store.notifications.markRead.mockClear();
    mocks.store.notifications.getCleanPath.mockReturnValue('/chat/remote.example.com/room-1');
    mocks.store.roomUnread.clear.mockClear();
    mocks.store.roomUnread.captureSnapshotRevision.mockClear();
    mocks.store.roomUnread.captureSnapshotRevision.mockReturnValue(0);
    mocks.store.roomUnread.initRooms.mockClear();
    mocks.store.roomUnread.updateRooms.mockClear();
    mocks.store.roomUnread.resolveUnknownUnread.mockClear();
    mocks.store.roomUnread.setServerHasUnread.mockClear();
    mocks.store.roomUnread.setRoomUnread.mockClear();
    mocks.store.setPermissions.mockClear();
    mocks.store.serverIndicator.mockReturnValue(null);
    mocks.store.projection.viewer = {};
    mocks.store.serverInfo.name = 'Loaded Remote';
    mocks.store.serverInfo.iconUrl = null;
    mocks.store.serverInfo.version = '0.5.0';
    mocks.store.serverInfo.compatibility = {
      status: 'supported',
      reason: 'version-confirmed'
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
    expect(q(document.body, '[data-testid="server-name"]')?.textContent?.trim()).toBe(
      'Loaded Remote'
    );
    expect(q(document.body, '[data-testid="server-hostname"]')?.textContent?.trim()).toBe(
      'remote.example.com'
    );
    expect(q(document.body, '[data-testid="server-sign-in-message"]')).toBeNull();
    expect(q(document.body, '[data-testid="server-compatibility-message"]')).toBeNull();
    expect(q(document.body, '[data-testid="server-connection-message"]')).toBeNull();

    const markRead = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Mark as read'
    );
    const remove = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Remove server'
    );
    await expect.element(markRead ?? null).toBeInTheDocument();
    await expect.element(markRead ?? null).toBeEnabled();
    await expect.element(remove ?? null).toBeInTheDocument();
    expect(markRead?.closest('.menu-section')).not.toBe(remove?.closest('.menu-section'));
    expect(q(document.body, '[role="separator"]')).toBeNull();
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

  it('shows Copy Server Hostname last and copies the displayed host', async () => {
    mocks.server.url = 'https://remote.example.com:8443/chat';
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });
    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;

    icon.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() =>
      expect(document.querySelector('[data-testid="copy-server-hostname"]')).not.toBeNull()
    );

    const copyHostname = q(
      document.body,
      '[data-testid="copy-server-hostname"]'
    ) as HTMLButtonElement;
    await expect.element(copyHostname).toHaveTextContent('Copy Server Hostname');
    const menuItems = copyHostname
      .closest('[role="menu"]')
      ?.querySelectorAll('[role="menuitem"]');
    expect(menuItems?.item((menuItems?.length ?? 0) - 1)).toBe(copyHostname);

    const remove = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Remove server'
    );
    expect(remove?.closest('.menu-section')).not.toBe(copyHostname.closest('.menu-section'));

    copyHostname.click();

    await vi.waitFor(() =>
      expect(mocks.writeClipboardText).toHaveBeenCalledWith('remote.example.com:8443')
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Copied to clipboard');
    await vi.waitFor(() =>
      expect(q(document.body, '[data-testid="copy-server-hostname"]')).toBeNull()
    );
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
      reason: 'server-too-old'
    };
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });

    await expect
      .element(q(container, '[data-testid="server-compatibility-warning"]'))
      .toBeInTheDocument();

    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;
    await expect
      .element(icon)
      .toHaveAttribute(
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

    const compatibilitySection = q(document.body, '[data-testid="server-compatibility-section"]');
    expect(compatibilitySection!.classList).toContain('text-sm');
    expect(compatibilitySection!.querySelector('.text-xs')).toBeNull();
    expect(compatibilitySection!.closest('.w-80')).not.toBeNull();
  });

  it('warns when the server version cannot establish compatibility', async () => {
    mocks.store.serverInfo.version = 'custom-build';
    mocks.store.serverInfo.compatibility = {
      status: 'unknown',
      reason: 'server-version-unknown'
    };
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });

    await expect
      .element(q(container, '[data-testid="server-compatibility-warning"]'))
      .toBeInTheDocument();

    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;
    await expect
      .element(icon)
      .toHaveAttribute(
        'title',
        'Loaded Remote — This app cannot determine compatibility from the server version.'
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
        'This app cannot determine compatibility from the server version.'
      )
    );
    expect(document.body.textContent).toContain('Version custom-build');
  });

  it('keeps a dormant server healthy until its discovery starts', async () => {
    mocks.store.networkStartupDeferred = true;
    mocks.store.isAuthenticated = false;
    mocks.store.projection.viewer = null;
    mocks.store.serverInfo.version = '';
    mocks.store.serverInfo.compatibility = {
      status: 'unknown',
      reason: 'server-version-unknown'
    };
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });

    await expect.element(q(container, '[data-testid="server-icon"]')).toBeInTheDocument();
    expect(q(container, '[data-testid="server-compatibility-warning"]')).toBeNull();
    expect(q(container, '[data-testid="server-sign-in-required"]')).toBeNull();
    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;
    expect(icon.classList).not.toContain('opacity-40');
    expect(icon.title).toBe('Loaded Remote');

    icon.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(q(document.body, '[data-testid="server-compatibility-section"]')).not.toBeNull());
    expect(q(document.body, '[data-testid="server-compatibility-message"]')).toBeNull();
    expect(document.body.textContent).not.toContain('Version unknown');
  });

  it('shows an unreachable status instead of an unknown version and hides read actions', async () => {
    mocks.store.projection.viewer = null;
    mocks.store.serverInfo.version = '';
    mocks.store.serverInfo.compatibility = {
      status: 'unreachable',
      reason: 'unreachable'
    };
    mocks.showConnectionLostIcon = true;
    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote', currentUserId: 'user-1' }
    });

    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote — Server unreachable');
    icon.dispatchEvent(
      new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        clientX: 24,
        clientY: 36
      })
    );

    await vi.waitFor(() => expect(document.body.textContent).toContain('Server unreachable'));
    expect(document.body.textContent).not.toContain('Version unknown');
    expect(q(document.body, '[data-testid="server-connection-message"]')).toBeNull();
    expect(document.body.textContent).not.toContain('Mark as read');
    expect(q(document.body, '[role="separator"]')).toBeNull();
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

  it('marks an unauthenticated synchronized server and starts sign-in when clicked', async () => {
    mocks.server.token = null;
    mocks.store.isAuthenticated = false;
    mocks.store.projection.viewer = null;

    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote' }
    });
    const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;

    await expect.element(icon).toHaveClass('opacity-40');
    await expect.element(icon).toHaveAttribute('title', 'Sign in to reconnect to Loaded Remote');
    await expect
      .element(q(container, '[data-testid="server-sign-in-required"]'))
      .toBeInTheDocument();
    icon.click();

    await vi.waitFor(() => {
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(mocks.server);
      expect(mocks.goto).not.toHaveBeenCalled();
    });
  });

  it('marks a server that requires reauthentication and prioritises it over compatibility', async () => {
    mocks.server.reauthRequiredAt = 123;
    mocks.store.isAuthenticated = false;
    mocks.showConnectionLostIcon = true;
    mocks.store.serverInfo.compatibility = {
      status: 'unsupported',
      reason: 'server-too-old'
    };

    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote' }
    });
    const icon = q(container, '[data-testid="server-icon"]');

    await expect.element(icon).toHaveAttribute('title', 'Sign in to reconnect to Loaded Remote');
    await expect.element(icon).toHaveAttribute('aria-label', 'Sign in to reconnect to Loaded Remote');
    const reauthMarker = q(container, '[data-testid="server-sign-in-required"]');
    await expect.element(reauthMarker).toBeInTheDocument();
    expect(
      reauthMarker?.querySelector('[class~="icon-[uil--exclamation-circle]"]')
    ).not.toBeNull();
    await expect
      .element(q(container, '[data-testid="server-compatibility-warning"]'))
      .not.toBeInTheDocument();

    icon?.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() =>
      expect(q(document.body, '[data-testid="server-sign-in-message"]')).not.toBeNull()
    );
    await expect
      .element(q(document.body, '[data-testid="server-sign-in-message"]'))
      .toHaveTextContent('Sign in to reconnect to Loaded Remote');
    await expect
      .element(q(document.body, '[data-testid="server-compatibility-message"]'))
      .toHaveTextContent('This server must be upgraded to Chatto 0.5 or newer');
    await expect
      .element(q(document.body, '[data-testid="server-connection-message"]'))
      .toHaveTextContent('Connection unavailable');
  });

  it('keeps a temporarily unreachable bearer server retryable without a sign-in marker', async () => {
    mocks.store.isAuthenticated = false;
    mocks.server.reauthRequiredAt = null;

    const { container } = render(ServerSidebarEntry, { props: { serverId: 'remote' } });
    const icon = q(container, '[data-testid="server-icon"]');
    await expect.element(icon).not.toHaveAttribute('title', 'Sign in to reconnect to Loaded Remote');
    await expect.element(q(container, '[data-testid="server-sign-in-required"]')).not.toBeInTheDocument();
  });

  it('links the pending origin to setup without a sign-in marker', async () => {
    mocks.server.token = null;
    mocks.isOriginServer.mockReturnValue(true);
    mocks.store.isAuthenticated = false;
    page.data.serverInfo = { setupRequired: true } as never;
    try {
      const { container } = render(ServerSidebarEntry, { props: { serverId: 'remote' } });
      const icon = q(container, 'a');
      await expect.element(icon).toHaveAttribute('href', '/setup');
      await expect.element(q(container, '[data-testid="server-sign-in-required"]')).not.toBeInTheDocument();
      expect(mocks.beginOriginReauthentication).not.toHaveBeenCalled();
    } finally {
      page.data.serverInfo = { setupRequired: false } as never;
    }
  });

  it('retries a saved session on selection instead of opening sign-in', async () => {
    mocks.store.isAuthenticated = false;
    const { container } = render(ServerSidebarEntry, { props: { serverId: 'remote' } });
    q(container, '[data-testid="server-icon"]')?.click();
    await vi.waitFor(() => expect(mocks.recoverServer).toHaveBeenCalledWith('remote'));
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
    expect(mocks.beginOriginReauthentication).not.toHaveBeenCalled();
  });

  it('uses the origin sign-in flow for an unauthenticated origin server', async () => {
    mocks.server.token = null;
    mocks.store.isAuthenticated = false;
    mocks.isOriginServer.mockReturnValue(true);

    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote' }
    });
    q(container, '[data-testid="server-icon"]')?.click();

    await vi.waitFor(() => {
      expect(mocks.beginOriginReauthentication).toHaveBeenCalledOnce();
      expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
    });
  });

  it('does not start a second sign-in while the first attempt is pending', async () => {
    mocks.server.token = null;
    mocks.store.isAuthenticated = false;
    mocks.startRemoteReauthentication.mockReturnValueOnce(new Promise(() => {}));

    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote' }
    });
    const icon = q(container, '[data-testid="server-icon"]');
    icon?.click();
    icon?.click();

    await vi.waitFor(() => {
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledOnce();
    });
  });

  it('shows an error and permits retry after sign-in fails', async () => {
    mocks.server.token = null;
    mocks.store.isAuthenticated = false;
    mocks.startRemoteReauthentication
      .mockRejectedValueOnce(new Error('discovery failed'))
      .mockResolvedValueOnce(undefined);

    const { container } = render(ServerSidebarEntry, {
      props: { serverId: 'remote' }
    });
    const icon = q(container, '[data-testid="server-icon"]');
    icon?.click();
    await vi.waitFor(() => expect(mocks.toastError).toHaveBeenCalledOnce());

    icon?.click();
    await vi.waitFor(() => {
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledTimes(2);
    });
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
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote — Connection unavailable');
    await expect.element(q(container, '[data-testid="server-compatibility-warning"]')).toBeInTheDocument();
    expect(container.textContent).toContain('L');
  });

  it('keeps a server with a retained projection fully visible during a disconnect', async () => {
    mocks.showConnectionLostIcon = true;

    const { container } = render(ServerSidebarEntry, { props: { serverId: 'remote' } });
    const icon = q(container, '[data-testid="server-icon"]');

    await expect.element(icon).not.toHaveClass('opacity-40');
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote — Connection unavailable');
    await expect.element(q(container, '[data-testid="server-compatibility-warning"]')).toBeInTheDocument();

    icon?.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() =>
      expect(q(document.body, '[data-testid="server-connection-message"]')).not.toBeNull()
    );
    await expect
      .element(q(document.body, '[data-testid="server-connection-message"]'))
      .toHaveTextContent('Connection unavailable');
  });

  it('updates an open menu when the connection fails and recovers', async () => {
    const { container } = render(ServerSidebarEntry, { props: { serverId: 'remote' } });
    const icon = q(container, '[data-testid="server-icon"]');

    icon?.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(q(document.body, '[data-testid="server-name"]')).not.toBeNull());
    expect(q(document.body, '[data-testid="server-connection-message"]')).toBeNull();

    connectionLostServers.add('remote');
    await vi.waitFor(() =>
      expect(q(document.body, '[data-testid="server-connection-message"]')).not.toBeNull()
    );
    await expect
      .element(q(document.body, '[data-testid="server-connection-message"]'))
      .toHaveTextContent('Connection unavailable');
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote — Connection unavailable');

    connectionLostServers.delete('remote');
    await vi.waitFor(() =>
      expect(q(document.body, '[data-testid="server-connection-message"]')).toBeNull()
    );
    await expect.element(q(container, '[data-testid="server-compatibility-warning"]')).not.toBeInTheDocument();
    await expect.element(icon).toHaveAttribute('title', 'Loaded Remote');
  });

  it('shows the connection warning in the touch sheet', async () => {
    vi.useFakeTimers();
    try {
      mocks.showConnectionLostIcon = true;
      const { container } = render(ServerSidebarEntry, { props: { serverId: 'remote' } });
      const icon = q(container, '[data-testid="server-icon"]') as HTMLAnchorElement;

      icon.dispatchEvent(
        new PointerEvent('pointerdown', {
          bubbles: true,
          pointerId: 1,
          pointerType: 'touch',
          isPrimary: true,
          clientX: 20,
          clientY: 30
        })
      );
      await vi.advanceTimersByTimeAsync(500);

      await expect.element(q(document.body, 'dialog.bottom-sheet')).toBeInTheDocument();
      await expect
        .element(q(document.body, '[data-testid="server-connection-message"]'))
        .toHaveTextContent('Connection unavailable');
    } finally {
      vi.useRealTimers();
    }
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
      signalKind: NotificationSignalKind.DIRECT_MENTION,
      targetSupported: true,
      room: { id: 'room-1', name: 'general' },
      eventId: 'event-1',
      threadRootId: 'thread-1'
    };
    mocks.store.serverIndicator.mockReturnValue('notification');
    mocks.store.notifications.unreadNotificationCount = 1;
    mocks.store.notifications.importantUnreadNotificationCount = 1;
    mocks.store.notifications.getNonDMNotification.mockReturnValue(notification);
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
    await expect.element(badge).toHaveClass('bg-attention');
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('remote', 'room-1');
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
      );
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'room-1',
        'thread-1',
        'event-1',
        'mention-1'
      );
      expect(mocks.store.notifications.markRead).not.toHaveBeenCalled();
      expect(mocks.goto).toHaveBeenCalledWith('/chat/remote.example.com/room-1/thread-1');
    });
  });

  it('opens notifications instead of navigating an unsupported future target', async () => {
    mocks.store.serverIndicator.mockReturnValue('notification');
    mocks.store.notifications.unreadNotificationCount = 1;
    mocks.store.notifications.importantUnreadNotificationCount = 1;
    mocks.store.notifications.getNonDMNotification.mockReturnValue({
      id: 'future-target',
      signalKind: NotificationSignalKind.UNSUPPORTED,
      targetSupported: false,
      createdAt: new Date().toISOString(),
      actor: null
    });

    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    const badge = q(container, '[data-testid="server-notification-badge"]');
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/notifications');
    });
    expect(mocks.appUi.disableRoomCallWideFor).not.toHaveBeenCalled();
  });

  it('uses a neutral server badge when only ambient notifications are unread', async () => {
    mocks.store.serverIndicator.mockReturnValue('notification');
    mocks.store.notifications.unreadNotificationCount = 2;
    mocks.store.notifications.importantUnreadNotificationCount = 0;

    const { container } = render(ServerSidebarEntry, {
      props: {
        serverId: 'remote',
        currentUserId: 'user-1'
      }
    });

    const badge = q(container, '[data-testid="server-notification-badge"]');
    await expect.element(badge).toHaveClass('bg-text');
    await expect.element(badge).not.toHaveClass('bg-attention');
  });
});
