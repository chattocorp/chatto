import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
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
    servers: [{ id: 'origin' }, { id: 'remote' }],
    lifecycle: [] as string[],
    synchronizeAuthenticatedServers: vi.fn(),
    resumeReturnNavigation: vi.fn(async () => false),
    initPresenceTracking: vi.fn(),
    stopPresenceTracking: vi.fn(),
    initSessionChannel: vi.fn(),
    stopSessionChannel: vi.fn(),
    updatePresenceEntries: vi.fn(),
    useProjectionEvent: vi.fn(),
    useSessionTerminated: vi.fn(),
    firstAuthenticatedServerId: vi.fn(() => 'remote'),
    clearServerAuthentication: vi.fn(),
    clearCachedUser: vi.fn(),
    hardRedirectAfterSignOut: vi.fn(),
    presenceCacheUpdate: vi.fn()
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
      getAPI: () => ({ serverId })
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

vi.mock('$lib/notifications/pushNotifications', () => ({
  getPushRegistrationTargets: () =>
    mocks.originCurrentUser.user
      ? [{ serverId: 'origin', userId: 'origin-user', vapidPublicKey: 'origin-vapid' }]
      : [{ serverId: 'remote', userId: 'remote-user', vapidPublicKey: 'remote-vapid' }]
}));

vi.mock('$lib/hooks/useEvent.svelte', () => ({
  useProjectionEvent: (...args: unknown[]) => {
    mocks.lifecycle.push('projection');
    mocks.useProjectionEvent(...args);
  },
  useSessionTerminated: (...args: unknown[]) => {
    mocks.lifecycle.push('session');
    mocks.useSessionTerminated(...args);
  }
}));

vi.mock('$lib/presenceTracking', () => ({
  initPresenceTracking: (...args: unknown[]) => {
    mocks.initPresenceTracking(...args);
    return mocks.stopPresenceTracking;
  }
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  updateAuthenticatedCurrentUserPresenceEntries: mocks.updatePresenceEntries
}));

vi.mock('$lib/state/presencePreference.svelte', () => ({
  presencePreference: { effectiveStatus: PresenceStatus.ONLINE }
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
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

vi.mock('$lib/api-client/presence', () => ({
  createPresenceAPI: vi.fn()
}));

vi.mock('$lib/api-client/viewer', () => ({
  viewerResponseToState: vi.fn()
}));

vi.mock('$lib/components/AuthStatusNotice.svelte', async () => ({
  default: (await import('./ChatRootTestStub.svelte')).default
}));

vi.mock('$lib/components/PushNotificationPrompt.svelte', async () => ({
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
    mocks.originCurrentUser.user = undefined;
    mocks.originCurrentUser.loading = true;
    mocks.lifecycle.length = 0;
    vi.clearAllMocks();
  });

  it('uses the origin viewer and bus installed by the application-root coordinator', () => {
    mocks.originCurrentUser.user = originUser;
    mocks.originCurrentUser.loading = false;
    const presenceCache = {
      update: mocks.presenceCacheUpdate
    } as unknown as PresenceCache;
    const profileCache = {
      update: vi.fn(),
      updateStatus: vi.fn(),
      remove: vi.fn(),
      clear: vi.fn()
    };

    const { container, unmount } = render(ChatRoot, {
      props: {
        user: originUser,
        profileCache,
        presenceCache,
        children
      }
    });

    expect(mocks.originCurrentUser.user).toBe(originUser);
    expect(mocks.originCurrentUser.loading).toBe(false);
    expect(mocks.lifecycle.slice(0, 2)).toEqual(['projection', 'session']);
    expect(mocks.synchronizeAuthenticatedServers).not.toHaveBeenCalled();
    expect(mocks.presenceCacheUpdate).toHaveBeenCalledWith(
      { serverId: 'origin', userId: 'origin-user' },
      PresenceStatus.ONLINE
    );
    const [[getPresenceAPIs, applyPresenceStatus]] = mocks.initPresenceTracking.mock.calls as [
      [() => unknown[], (status: PresenceStatus) => void]
    ];
    expect(getPresenceAPIs()).toEqual([{ serverId: 'origin' }, { serverId: 'remote' }]);

    applyPresenceStatus(PresenceStatus.AWAY);
    expect(mocks.updatePresenceEntries).toHaveBeenLastCalledWith(
      presenceCache,
      [
        expect.objectContaining({ serverId: 'origin', isAuthenticated: true }),
        expect.objectContaining({ serverId: 'remote', isAuthenticated: true })
      ],
      PresenceStatus.AWAY
    );
    expect(container.querySelector('[data-testid="chat-root-child"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="chat-root-component-stub"]')).toHaveLength(4);

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
    const profileCache = {
      update: vi.fn(),
      updateStatus: vi.fn(),
      remove: vi.fn(),
      clear: vi.fn()
    };

    const { container, unmount } = render(ChatRoot, {
      props: {
        user: null,
        profileCache,
        presenceCache,
        children
      }
    });

    expect(mocks.originCurrentUser.user).toBeUndefined();
    expect(mocks.originCurrentUser.loading).toBe(true);
    expect(mocks.lifecycle).not.toContain('synchronize');
    expect(mocks.lifecycle).not.toContain('projection');
    expect(mocks.lifecycle).not.toContain('session');
    expect(mocks.synchronizeAuthenticatedServers).not.toHaveBeenCalled();
    expect(mocks.resumeReturnNavigation).not.toHaveBeenCalled();
    expect(mocks.presenceCacheUpdate).not.toHaveBeenCalled();
    expect(mocks.initSessionChannel).not.toHaveBeenCalled();

    const [[getPresenceAPIs, applyPresenceStatus]] = mocks.initPresenceTracking.mock.calls as [
      [() => unknown[], (status: PresenceStatus) => void]
    ];
    expect(getPresenceAPIs()).toEqual([{ serverId: 'remote' }]);

    applyPresenceStatus(PresenceStatus.AWAY);
    expect(mocks.updatePresenceEntries).toHaveBeenLastCalledWith(
      presenceCache,
      [
        expect.objectContaining({ serverId: 'origin', isAuthenticated: false }),
        expect.objectContaining({ serverId: 'remote', isAuthenticated: true })
      ],
      PresenceStatus.AWAY
    );
    expect(container.querySelector('[data-testid="chat-root-child"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="chat-root-component-stub"]')).toHaveLength(3);

    unmount();

    expect(mocks.stopPresenceTracking).toHaveBeenCalledOnce();
    expect(mocks.stopSessionChannel).not.toHaveBeenCalled();
  });
});
