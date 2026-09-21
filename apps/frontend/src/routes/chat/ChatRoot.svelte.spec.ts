import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createRawSnippet } from 'svelte';
import type { CurrentUser } from '$lib/api-client/viewer';
import type { PresenceCache } from '$lib/state/presenceCache.svelte';

const mocks = vi.hoisted(() => {
  const originCurrentUser = {
    user: undefined as CurrentUser | undefined,
    loading: true
  };
  const remoteCurrentUser = {
    user: { id: 'remote-user' } as CurrentUser,
    loading: false
  };
  const originStore = {
    currentUser: originCurrentUser,
    get isAuthenticated() {
      return originCurrentUser.user !== undefined;
    },
    voiceCall: { isInAnyCall: false },
    serverInfo: { supportsRealtimeProjection: true },
    realtimeSync: { serverId: 'origin-sync' }
  };
  const remoteStore = {
    currentUser: remoteCurrentUser,
    isAuthenticated: true,
    voiceCall: { isInAnyCall: false },
    serverInfo: { supportsRealtimeProjection: true },
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
    initSessionChannel: vi.fn(),
    stopSessionChannel: vi.fn(),
    presenceStatuses: { origin: 1, remote: 4 },
    useSessionTerminated: vi.fn(),
    firstAuthenticatedServerId: vi.fn(() => 'remote'),
    clearServerAuthentication: vi.fn(),
    clearCachedUser: vi.fn(),
    hardRedirectAfterSignOut: vi.fn(),
    presenceCacheUpdate: vi.fn(),
    deviceTimezone: vi.fn<() => string | null>(() => null),
    updateSettings: vi.fn(async () => ({
      timezone: 'Europe/Berlin',
      timeFormat: undefined,
      shareTimezone: false
    }))
  };
});

vi.mock('$lib/state/server/registry.svelte', () => ({
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
    clearServerAuthentication: mocks.clearServerAuthentication
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: (serverId: string) => ({
      serverId,
      getAPI: () =>
        ({
          serverId,
          updateSettings: mocks.updateSettings
        }) as unknown
    })
  }
}));

vi.mock('$lib/state/server/eventBus.svelte', () => ({
  eventBusManager: {
    synchronizeAuthenticatedServers: (registrations: unknown[], activeServerId: string | null) => {
      mocks.lifecycle.push('synchronize');
      mocks.synchronizeAuthenticatedServers(registrations, activeServerId);
    }
  }
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

vi.mock('$lib/hooks/useEvent.svelte', () => ({
  useSessionTerminated: (...args: unknown[]) => {
    mocks.lifecycle.push('session');
    mocks.useSessionTerminated(...args);
  }
}));

vi.mock('$lib/presenceTracking', () => ({
  initPresenceTracking: (...args: unknown[]) => {
    mocks.initPresenceTracking(...args);
    return { sync: () => (args[0] as () => unknown[])(), stop: mocks.stopPresenceTracking };
  }
}));

vi.mock('$lib/state/server/presencePreference.svelte', () => ({
  presencePreferences: {
    get: ({ serverId }: { serverId: 'origin' | 'remote' }) => ({
      status: mocks.presenceStatuses[serverId]
    })
  }
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  scheduleCustomStatusExpiry: vi.fn()
}));

vi.mock('$lib/auth/loadAuth', () => ({
  clearCachedUser: mocks.clearCachedUser
}));

vi.mock('$lib/auth/returnNavigation', () => ({
  resumeReturnNavigation: mocks.resumeReturnNavigation
}));

vi.mock('$lib/auth/signOut', () => ({
  hardRedirectAfterSignOut: mocks.hardRedirectAfterSignOut,
  isExplicitSignOutRedirectInProgress: () => false
}));

vi.mock('$lib/auth/sessionChannel', () => ({
  initSessionChannel: (...args: unknown[]) => {
    mocks.initSessionChannel(...args);
    return mocks.stopSessionChannel;
  }
}));

vi.mock('$lib/api-client/memberDirectory', () => ({
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

vi.mock('$lib/api-client/presence', () => ({
  createPresenceAPI: vi.fn()
}));

vi.mock('$lib/api-client/viewer', () => ({
  viewerResponseToState: vi.fn()
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
    mocks.presenceStatuses.origin = PresenceStatus.ONLINE;
    mocks.presenceStatuses.remote = PresenceStatus.OFFLINE;
    mocks.originCurrentUser.user = undefined;
    mocks.originCurrentUser.loading = true;
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
    vi.clearAllMocks();
  });

  it('uses the origin viewer and bus installed by the application-root coordinator', () => {
    mocks.originCurrentUser.user = originUser;
    mocks.originCurrentUser.loading = false;
    const presenceCache = {
      update: mocks.presenceCacheUpdate
    } as unknown as PresenceCache;

    const { container, unmount } = render(ChatRoot, {
      props: {
        user: originUser,
        presenceCache,
        children
      }
    });

    expect(mocks.originCurrentUser.user).toBe(originUser);
    expect(mocks.originCurrentUser.loading).toBe(false);
    expect(mocks.lifecycle[0]).toBe('session');
    expect(mocks.synchronizeAuthenticatedServers).not.toHaveBeenCalled();
    expect(mocks.presenceCacheUpdate).toHaveBeenCalledWith(
      { serverId: 'origin', userId: 'origin-user' },
      PresenceStatus.ONLINE
    );
    const [[getPresenceAPIs]] = mocks.initPresenceTracking.mock.calls as [[() => unknown[]]];
    expect(
      getPresenceAPIs().map((api) => ({ serverId: (api as { serverId: string }).serverId }))
    ).toEqual([{ serverId: 'origin' }, { serverId: 'remote' }]);

    expect(mocks.presenceCacheUpdate).toHaveBeenCalledWith(
      { serverId: 'remote', userId: 'remote-user' },
      PresenceStatus.OFFLINE
    );
    expect(container.querySelector('[data-testid="chat-root-child"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="chat-root-component-stub"]')).toHaveLength(3);

    const [[handleCrossTabLogout]] = mocks.initSessionChannel.mock.calls as [[() => void]];
    handleCrossTabLogout();
    expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin');
    expect(mocks.firstAuthenticatedServerId).toHaveBeenCalledWith('origin');
    expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/chat/remote.example.test');

    unmount();

    expect(mocks.originCurrentUser.user).toBe(originUser);
    expect(mocks.stopPresenceTracking).toHaveBeenCalledOnce();
    expect(mocks.stopSessionChannel).toHaveBeenCalledOnce();
  });

  it('keeps remote realtime and presence active without installing origin-only behavior', () => {
    const presenceCache = {
      update: mocks.presenceCacheUpdate
    } as unknown as PresenceCache;

    const { container, unmount } = render(ChatRoot, {
      props: {
        user: null,
        presenceCache,
        children
      }
    });

    expect(mocks.originCurrentUser.user).toBeUndefined();
    expect(mocks.originCurrentUser.loading).toBe(true);
    expect(mocks.lifecycle).not.toContain('synchronize');
    expect(mocks.lifecycle).not.toContain('session');
    expect(mocks.synchronizeAuthenticatedServers).not.toHaveBeenCalled();
    expect(mocks.resumeReturnNavigation).not.toHaveBeenCalled();
    expect(mocks.presenceCacheUpdate).toHaveBeenCalledWith(
      { serverId: 'remote', userId: 'remote-user' },
      PresenceStatus.OFFLINE
    );
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
    const presenceCache = { update: mocks.presenceCacheUpdate } as unknown as PresenceCache;

    render(ChatRoot, {
      props: { user: null, presenceCache, children }
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
      const presenceCache = { update: mocks.presenceCacheUpdate } as unknown as PresenceCache;

      render(ChatRoot, {
        props: { user: null, presenceCache, children }
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
    const presenceCache = { update: mocks.presenceCacheUpdate } as unknown as PresenceCache;

    render(ChatRoot, {
      props: { user: null, presenceCache, children }
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
    const presenceCache = { update: mocks.presenceCacheUpdate } as unknown as PresenceCache;

    render(ChatRoot, {
      props: { user: null, presenceCache, children }
    });

    await Promise.resolve();
    expect(mocks.updateSettings).not.toHaveBeenCalled();
  });
});
