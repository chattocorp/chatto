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
    serverInfo: { supportsRealtimeProjection: true },
    realtimeSync: { serverId: 'origin-sync' }
  };
  const remoteStore = {
    currentUser: remoteCurrentUser,
    isAuthenticated: true,
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
    getStore: (serverId: string) =>
      serverId === 'origin' ? mocks.originStore : mocks.remoteStore,
    tryGetStore: (serverId: string) => {
      if (serverId === 'origin') return mocks.originStore;
      if (serverId === 'remote') return mocks.remoteStore;
      return undefined;
    },
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
    synchronizeAuthenticatedServers: (
      registrations: unknown[],
      activeServerId: string | null
    ) => {
      mocks.lifecycle.push('synchronize');
      mocks.synchronizeAuthenticatedServers(registrations, activeServerId);
    }
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'remote'
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
  default: (await import('./AuthenticatedRootTestStub.svelte')).default
}));

vi.mock('$lib/components/PushNotificationPrompt.svelte', async () => ({
  default: (await import('./AuthenticatedRootTestStub.svelte')).default
}));

vi.mock('$lib/components/PushNotificationSetup.svelte', async () => ({
  default: (await import('./AuthenticatedRootTestStub.svelte')).default
}));

vi.mock('$lib/components/WelcomeBanner.svelte', async () => ({
  default: (await import('./AuthenticatedRootTestStub.svelte')).default
}));

import AuthenticatedRoot from './AuthenticatedRoot.svelte';

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
  render: () => '<div data-testid="authenticated-child">Authenticated child</div>'
}));

describe('AuthenticatedRoot', () => {
  beforeEach(() => {
    mocks.originCurrentUser.user = undefined;
    mocks.originCurrentUser.loading = true;
    mocks.lifecycle.length = 0;
    vi.clearAllMocks();
  });

  it('installs the origin viewer before the initial multi-server realtime reconciliation', () => {
    const presenceCache = {
      update: mocks.presenceCacheUpdate
    } as unknown as PresenceCache;
    const profileCache = {
      update: vi.fn(),
      updateStatus: vi.fn(),
      remove: vi.fn(),
      clear: vi.fn()
    };

    const { container, unmount } = render(AuthenticatedRoot, {
      props: {
        user: originUser,
        profileCache,
        presenceCache,
        children
      }
    });

    expect(mocks.originCurrentUser.user).toMatchObject({
      id: 'origin-user',
      presenceStatus: PresenceStatus.ONLINE
    });
    expect(mocks.originCurrentUser.loading).toBe(false);
    expect(mocks.lifecycle.slice(0, 3)).toEqual(['synchronize', 'projection', 'session']);
    expect(mocks.synchronizeAuthenticatedServers.mock.calls[0]).toEqual([
      [
        expect.objectContaining({ serverId: 'origin' }),
        expect.objectContaining({ serverId: 'remote' })
      ],
      'remote'
    ]);
    expect(mocks.presenceCacheUpdate).toHaveBeenCalledWith(
      { serverId: 'origin', userId: 'origin-user' },
      PresenceStatus.ONLINE
    );
    const [[getPresenceAPIs, applyPresenceStatus]] = mocks.initPresenceTracking.mock.calls as [[
      () => unknown[],
      (status: PresenceStatus) => void
    ]];
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
    expect(container.querySelector('[data-testid="authenticated-child"]')).not.toBeNull();

    unmount();

    expect(mocks.originCurrentUser.user).toBeUndefined();
    expect(mocks.stopPresenceTracking).toHaveBeenCalledOnce();
    expect(mocks.stopSessionChannel).toHaveBeenCalledOnce();
  });
});
