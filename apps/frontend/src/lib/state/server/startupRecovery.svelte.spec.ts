import { afterEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  loadCurrentUser: vi.fn(),
  invalidateAll: vi.fn()
}));

vi.mock('$lib/auth/loadAuth', () => ({
  loadCurrentUser: mocks.loadCurrentUser,
  clearCachedUser: vi.fn()
}));

vi.mock('$app/navigation', () => ({ invalidateAll: mocks.invalidateAll }));

vi.mock('$lib/api-client/server', () => ({
  getPublicServerInfo: vi.fn(async () => ({
    name: 'Chatto', version: '0.5.0', welcomeMessage: null, description: null,
    iconUrl: null, bannerUrl: null, directRegistrationEnabled: true,
    directLoginEnabled: true
  }))
}));

import { serverRegistry } from './registry.svelte';
import { emptyServerSession } from './sessions.svelte';

describe('origin startup recovery', () => {
  afterEach(() => {
    serverRegistry.removeAll();
    vi.clearAllMocks();
  });

  it('does not unlock a disk view when viewer loading returns a cached fallback', async () => {
    serverRegistry.removeAll();
    serverRegistry.addServer({
      id: 'origin', url: window.location.origin, name: 'Chatto', iconUrl: null,
      addedAt: Date.now()
    }, { ...emptyServerSession(), userId: 'U1' });
    const store = serverRegistry.getStore('origin');
    await store.serverInfo.init();
    store.restoreSavedView({
      version: 1, serverId: 'origin', userId: 'U1', serverName: 'Chatto',
      savedAt: Date.now(), rooms: [{ id: 'R1', name: 'general', messages: [] }]
    }, true);
    mocks.loadCurrentUser.mockResolvedValue({ id: 'U1' });

    await serverRegistry.recoverServer('origin');

    expect(mocks.loadCurrentUser).toHaveBeenCalledOnce();
    expect(store.startupPresentationOnly).toBe(true);
    expect(store.isAuthenticated).toBe(false);
    expect(mocks.invalidateAll).not.toHaveBeenCalled();
  });
});
