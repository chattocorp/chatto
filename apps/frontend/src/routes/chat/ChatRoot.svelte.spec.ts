import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createRawSnippet } from 'svelte';
import type { CurrentUser } from '@chatto/client/api/viewer';
import { CurrentUserState } from '@chatto/client/auth/currentUser';

const mocks = vi.hoisted(() => {
  const originCurrentUser = {
    user: undefined as CurrentUser | undefined,
    loading: true,
    verifiedUserId: null as string | null
  };
  const remoteCurrentUser = {
    user: { id: 'remote-user' } as CurrentUser,
    loading: false
  };
  const originStore = {
    currentUser: originCurrentUser,
    get isAuthenticated() {
      return this.currentUser.user !== undefined;
    },
    get accountId(): string | null {
      return this.currentUser.user?.id ?? null;
    },
    voiceCall: { isInAnyCall: false },
    serverInfo: { isSupportedVersion: true },
    realtimeSync: { serverId: 'origin-sync' }
  };
  const remoteStore = {
    currentUser: remoteCurrentUser,
    get accountId(): string | null {
      return this.currentUser.user.id;
    },
    isAuthenticated: true,
    voiceCall: { isInAnyCall: false },
    serverInfo: { isSupportedVersion: true },
    realtimeSync: { serverId: 'remote-sync' }
  };

  return {
    originCurrentUser,
    originStore,
    remoteStore,
    remoteCurrentUser,
    servers: [{ id: 'origin' }, { id: 'remote' }],
    lifecycle: [] as string[],
    synchronizeAuthenticatedServers: vi.fn(),
    resumeReturnNavigation: vi.fn(async () => false),
    initPresenceTracking: vi.fn(),
    stopPresenceTracking: vi.fn(),
    refreshPresencePreference: vi.fn(),
    watchStores: vi.fn(),
    stopWatchingStores: vi.fn(),
    initSessionChannel: vi.fn(),
    stopSessionChannel: vi.fn(),
    onSessionTerminated: vi.fn(),
    stopSessionTermination: vi.fn(),
    firstAuthenticatedServerId: vi.fn(() => 'remote'),
    clearServerAuthentication: vi.fn(),
    hardRedirectAfterSignOut: vi.fn(),
    originSignInRequired: false,
    beginOriginReauthentication: vi.fn(),
    deviceTimezone: vi.fn<() => string | null>(() => null),
    updateSettings: vi.fn(async () => ({
      timezone: 'Europe/Berlin',
      timeFormat: undefined,
      shareTimezone: false
    }))
  };
});

// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    originServer: { id: 'origin' },
    get servers() {
      return mocks.servers;
    },
    getStore: (serverId: string) => (serverId === 'origin' ? mocks.originStore : mocks.remoteStore),
    tryGetStore: (serverId: string) => {
      if (serverId === 'origin') return mocks.originStore;
      if (serverId === 'remote') return mocks.remoteStore;
      return undefined;
    },
    firstAuthenticatedServerId: mocks.firstAuthenticatedServerId,
    clearServerAuthentication: mocks.clearServerAuthentication,
    watchStores: (setup: (store: unknown) => void) => {
      mocks.watchStores(setup);
      return mocks.stopWatchingStores;
    },
    get originSignInRequired() {
      return mocks.originSignInRequired;
    }
  },
  serverConnectionManager: {
    getClient: (serverId: string) => ({
      serverId,
      getAPI: () =>
        ({
          serverId,
          updateSettings: mocks.updateSettings
        }) as unknown
    })
  },
  eventBusManager: {
    synchronizeAuthenticatedServers: (registrations: unknown[], activeServerId: string | null) => {
      mocks.lifecycle.push('synchronize');
      mocks.synchronizeAuthenticatedServers(registrations, activeServerId);
    },
    getBus: (serverId: string) => ({
      onSessionTerminated: (handler: (reason: string) => void) => {
        mocks.lifecycle.push('session');
        mocks.onSessionTerminated(serverId, handler);
        return mocks.stopSessionTermination;
      }
    })
  }
}));

vi.mock('$lib/auth/reauth', () => ({
  beginOriginReauthentication: mocks.beginOriginReauthentication
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'remote'
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => `${serverId}.example.test`
}));

vi.mock('$lib/state/server/presenceTracking', () => ({
  initPresenceTracking: (...args: unknown[]) => {
    mocks.initPresenceTracking(...args);
    return { sync: () => (args[0] as () => unknown[])(), stop: mocks.stopPresenceTracking };
  },
  refreshPresencePreference: mocks.refreshPresencePreference
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_userId: string, fallback: string | null) => fallback,
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  scheduleCustomStatusExpiry: vi.fn()
}));

vi.mock('$lib/auth/returnNavigation', () => ({
  resumeReturnNavigation: mocks.resumeReturnNavigation
}));

vi.mock('@chatto/client/auth/signOut', () => ({
  isExplicitSignOutRedirectInProgress: () => false
}));

vi.mock('$lib/auth/signOutRedirect', () => ({
  hardRedirectAfterSignOut: mocks.hardRedirectAfterSignOut
}));

vi.mock('$lib/auth/sessionChannel', () => ({
  initSessionChannel: (...args: unknown[]) => {
    mocks.initSessionChannel(...args);
    return mocks.stopSessionChannel;
  }
}));

vi.mock('@chatto/client/api/memberDirectory', () => ({
  mapDirectoryMember: vi.fn()
}));

vi.mock('$lib/utils/deviceTimezone', () => ({
  deviceTimezone: () => mocks.deviceTimezone(),
  createDeviceTimezoneReportTracker: () => {
    const reported = new Set<string>();
    return {
      begin: (key: string) => {
        if (reported.has(key)) return false;
        reported.add(key);
        return true;
      },
      allowRetry: (key: string) => reported.delete(key)
    };
  }
}));

vi.mock('@chatto/client/api/presence', () => ({
  createPresenceAPI: vi.fn()
}));

vi.mock('@chatto/client/api/viewer', () => ({
  viewerResponseToState: vi.fn(),
  getCurrentUserViaConnect: vi.fn()
}));

vi.mock('$lib/components/AuthStatusNotice.svelte', async () => ({
  default: (await import('./ChatRootTestStub.svelte')).default
}));

vi.mock('$lib/components/PushNotificationSetup.svelte', async () => ({
  default: (await import('./ChatRootTestStub.svelte')).default
}));

vi.mock('$lib/components/WelcomeBanner.svelte', async () => ({
  default: (await import('./ChatRootTestStub.svelte')).default
}));

import ChatRoot from './ChatRoot.svelte';

// The plain account mock uses the real same-account update rule.
Object.assign(mocks.remoteCurrentUser, { update: CurrentUserState.prototype.update });

const originUser: CurrentUser = {
  id: 'origin-user',
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
  customStatus: null,
  presenceStatus: PresenceStatus.AWAY,
  hasVerifiedEmail: true,
  hasPassword: true,
  viewerCanDeleteAccount: true,
  lastLoginChange: null,
  settings: null
};

const children = createRawSnippet(() => ({
  render: () => '<div data-testid="chat-root-child">Chat child</div>'
}));

describe('ChatRoot', () => {
  beforeEach(() => {
    mocks.originCurrentUser = new CurrentUserState(true);
    mocks.originStore.currentUser = mocks.originCurrentUser;
    mocks.remoteCurrentUser.user = {
      ...originUser,
      id: 'remote-user',
      settings: null
    };
    mocks.updateSettings.mockResolvedValue({
      timezone: 'Europe/Berlin',
      timeFormat: undefined,
      shareTimezone: false
    });
    mocks.lifecycle.length = 0;
    mocks.originSignInRequired = false;
    vi.clearAllMocks();
  });

  it('opens sign-in when the origin rejected its viewer and nothing remains to read', () => {
    mocks.originSignInRequired = true;
    const { unmount } = render(ChatRoot, {
      props: { children }
    });

    expect(mocks.beginOriginReauthentication).toHaveBeenCalledOnce();
    unmount();
  });

  it('keeps the current page while the origin session is usable or still readable', () => {
    const { unmount } = render(ChatRoot, {
      props: { children }
    });

    expect(mocks.beginOriginReauthentication).not.toHaveBeenCalled();
    unmount();
  });

  it('uses the origin viewer and bus installed by the application-root coordinator', () => {
    mocks.originCurrentUser.user = originUser;
    mocks.originCurrentUser.loading = false;
    mocks.originCurrentUser.verifiedUserId = originUser.id;
    const { container, unmount } = render(ChatRoot, {
      props: { children }
    });

    expect(mocks.originCurrentUser.user).toEqual(originUser);
    expect(mocks.originCurrentUser.loading).toBe(false);
    expect(mocks.lifecycle[0]).toBe('session');
    expect(mocks.synchronizeAuthenticatedServers).not.toHaveBeenCalled();
    const [[getPresenceAPIs]] = mocks.initPresenceTracking.mock.calls as [[() => unknown[]]];
    expect(
      getPresenceAPIs().map((api) => ({ serverId: (api as { serverId: string }).serverId }))
    ).toEqual([{ serverId: 'origin' }, { serverId: 'remote' }]);
    expect(container.querySelector('[data-testid="chat-root-child"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="chat-root-component-stub"]')).toHaveLength(3);

    const [[handleCrossTabLogout]] = mocks.initSessionChannel.mock.calls as [[() => void]];
    handleCrossTabLogout();
    expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin');
    expect(mocks.firstAuthenticatedServerId).toHaveBeenCalledWith('origin');
    expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/chat/remote.example.test');

    unmount();

    expect(mocks.originCurrentUser.user).toEqual(originUser);
    expect(mocks.stopPresenceTracking).toHaveBeenCalledOnce();
    expect(mocks.stopSessionChannel).toHaveBeenCalledOnce();
    expect(mocks.stopSessionTermination).toHaveBeenCalledOnce();
  });

  it('installs origin session handling when the viewer becomes verified', async () => {
    const { container, unmount } = render(ChatRoot, { props: { children } });
    const child = container.querySelector('[data-testid="chat-root-child"]');

    expect(mocks.onSessionTerminated).not.toHaveBeenCalled();
    expect(mocks.initSessionChannel).not.toHaveBeenCalled();

    mocks.originCurrentUser.user = originUser;
    mocks.originCurrentUser.verifiedUserId = originUser.id;
    mocks.originCurrentUser.loading = false;

    await vi.waitFor(() => expect(mocks.onSessionTerminated).toHaveBeenCalledOnce());
    expect(mocks.onSessionTerminated).toHaveBeenCalledWith('origin', expect.any(Function));
    expect(mocks.initSessionChannel).toHaveBeenCalledOnce();
    expect(container.querySelector('[data-testid="chat-root-child"]')).toBe(child);

    const handler = mocks.onSessionTerminated.mock.calls[0][1] as (reason: string) => void;
    handler('revoked');
    expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin');
    expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/chat/remote.example.test');

    unmount();
    expect(mocks.stopSessionTermination).toHaveBeenCalledOnce();
    expect(mocks.stopSessionChannel).toHaveBeenCalledOnce();
  });

  it('keeps remote realtime and presence active without installing origin-only behavior', () => {
    const { container, unmount } = render(ChatRoot, {
      props: { children }
    });

    expect(mocks.originCurrentUser.user).toBeUndefined();
    expect(mocks.originCurrentUser.loading).toBe(true);
    expect(mocks.lifecycle).not.toContain('synchronize');
    expect(mocks.lifecycle).not.toContain('session');
    expect(mocks.synchronizeAuthenticatedServers).not.toHaveBeenCalled();
    expect(mocks.resumeReturnNavigation).not.toHaveBeenCalled();
    expect(mocks.initSessionChannel).not.toHaveBeenCalled();

    const [[getPresenceAPIs]] = mocks.initPresenceTracking.mock.calls as [[() => unknown[]]];
    expect(
      getPresenceAPIs().map((api) => ({ serverId: (api as { serverId: string }).serverId }))
    ).toEqual([{ serverId: 'remote' }]);

    expect(container.querySelector('[data-testid="chat-root-child"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="chat-root-component-stub"]')).toHaveLength(2);

    unmount();

    expect(mocks.stopPresenceTracking).toHaveBeenCalledOnce();
    expect(mocks.stopSessionChannel).not.toHaveBeenCalled();
  });

  it("reads the viewer's presence choice again when another device changed it", () => {
    const { unmount } = render(ChatRoot, { props: { children } });
    const [[setup]] = mocks.watchStores.mock.calls as [[(store: unknown) => void]];
    let listener!: (update: unknown) => void;
    setup({
      serverId: 'remote',
      accountId: 'remote-user',
      onUpdate: (next: (update: unknown) => void) => {
        listener = next;
        return () => {};
      }
    });
    listener({ event: { event: { case: 'messagePosted' } } });
    expect(mocks.refreshPresencePreference).not.toHaveBeenCalled();
    listener({ event: { event: { case: 'viewerPresencePreferenceChanged' } } });
    expect(mocks.refreshPresencePreference).toHaveBeenCalledWith({
      serverId: 'remote',
      userId: 'remote-user'
    });
    unmount();
    expect(mocks.stopWatchingStores).toHaveBeenCalledOnce();
  });

  it('reports the device time zone once for a viewer without an explicit zone', async () => {
    vi.mocked(mocks.deviceTimezone).mockReturnValue('Europe/Berlin');
    mocks.updateSettings.mockResolvedValue({
      timezone: 'Europe/Berlin',
      timeFormat: undefined,
      shareTimezone: false
    });
    const remoteUser: CurrentUser = {
      ...originUser,
      id: 'remote-user',
      settings: {
        timezone: null,
        timeFormat: TimeFormat.TIME_FORMAT_AUTO,
        shareTimezone: false
      }
    };
    mocks.remoteCurrentUser.user = remoteUser;

    render(ChatRoot, {
      props: { children }
    });

    await expect.poll(() => mocks.remoteCurrentUser.user?.settings?.timezone).toBe('Europe/Berlin');
    expect(mocks.updateSettings).toHaveBeenCalledExactlyOnceWith({ timezone: 'Europe/Berlin' });
  });

  it.each(['America/New_York', ''])(
    'does not report over an explicit viewer time zone: %s',
    async (timezone) => {
      vi.mocked(mocks.deviceTimezone).mockReturnValue('Europe/Berlin');
      mocks.remoteCurrentUser.user = {
        ...originUser,
        id: 'remote-user',
        settings: {
          timezone,
          timeFormat: TimeFormat.TIME_FORMAT_AUTO,
          shareTimezone: false
        }
      };

      render(ChatRoot, {
        props: { children }
      });

      await Promise.resolve();
      expect(mocks.updateSettings).not.toHaveBeenCalled();
    }
  );

  it('does not immediately retry a failed time-zone report', async () => {
    vi.mocked(mocks.deviceTimezone).mockReturnValue('Europe/Berlin');
    mocks.updateSettings.mockRejectedValue(new Error('offline'));
    mocks.remoteCurrentUser.user = {
      ...originUser,
      id: 'remote-user',
      settings: {
        timezone: null,
        timeFormat: TimeFormat.TIME_FORMAT_AUTO,
        shareTimezone: false
      }
    };

    render(ChatRoot, {
      props: { children }
    });

    await vi.waitFor(() => expect(mocks.updateSettings).toHaveBeenCalledOnce());
    await Promise.resolve();
    await Promise.resolve();
    expect(mocks.updateSettings).toHaveBeenCalledOnce();
  });

  it('does not report the device time zone when the server lacks privacy support', async () => {
    vi.mocked(mocks.deviceTimezone).mockReturnValue('Europe/Berlin');
    mocks.remoteCurrentUser.user = {
      ...originUser,
      id: 'remote-user',
      settings: { timezone: null, timeFormat: TimeFormat.TIME_FORMAT_AUTO }
    };

    render(ChatRoot, {
      props: { children }
    });

    await Promise.resolve();
    expect(mocks.updateSettings).not.toHaveBeenCalled();
  });
});
