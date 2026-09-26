import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import type { CurrentUser } from '$lib/api-client/viewer';

const mocks = vi.hoisted(() => ({ discovery: vi.fn(), viewer: vi.fn() }));
vi.mock('$lib/api-client/server', async (original) => ({
  ...(await original<typeof import('$lib/api-client/server')>()),
  getPublicServerInfo: mocks.discovery
}));
vi.mock('$lib/api-client/viewer', async (original) => ({
  ...(await original<typeof import('$lib/api-client/viewer')>()),
  getCurrentUserViaConnect: mocks.viewer
}));

import { serverRegistry as registry } from './registry.svelte';
import { emptyServerSession } from './sessions.svelte';

const profile = {
  name: 'Recovered server',
  version: '0.5.0',
  welcomeMessage: null,
  description: null,
  iconUrl: null,
  bannerUrl: null,
  directRegistrationEnabled: true,
  directLoginEnabled: true
};
const user = {
  id: 'test-user',
  login: 'test',
  displayName: 'Test',
  avatarUrl: null
} as CurrentUser;

async function register() {
  registry.addServer(
    {
      id: 'retry-test',
      url: 'https://retry.test',
      name: 'Saved server',
      iconUrl: null,
      addedAt: 1
    },
    { ...emptyServerSession(), token: 'test-token' }
  );
  const store = registry.getStore('retry-test');
  await vi.waitFor(() => {
    expect(store.serverInfo.loading).toBe(false);
    expect(store.currentUser.loading).toBe(false);
  });
  return store;
}

describe('registered server recovery', () => {
  beforeEach(() => {
    registry.removeAll();
    localStorage.clear();
    mocks.discovery.mockReset().mockRejectedValue(new Error('offline'));
    mocks.viewer.mockReset().mockRejectedValue(new Error('offline'));
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });
  afterEach(() => {
    registry.removeAll();
    vi.restoreAllMocks();
  });

  it('restores discovery and the retained viewer after startup failed', async () => {
    const store = await register();
    expect(registry.needsRecovery('retry-test')).toBe(true);
    mocks.discovery.mockResolvedValue(profile);
    mocks.viewer.mockResolvedValue(user);
    await registry.recoverServer('retry-test');
    expect(store.serverInfo.error).toBeNull();
    expect(store.isAuthenticated).toBe(true);
    expect(registry.getServer('retry-test')?.userId).toBe(user.id);
    expect(registry.needsRecovery('retry-test')).toBe(false);
  });

  it('retries a viewer-only failure and stops on an authentication rejection', async () => {
    mocks.discovery.mockResolvedValue(profile);
    await register();
    expect(registry.needsRecovery('retry-test')).toBe(true);
    mocks.viewer.mockImplementation(async (config) => {
      await config.renewBearerToken?.(true);
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });
    await registry.recoverServer('retry-test');
    expect(registry.getServer('retry-test')?.reauthRequiredAt).not.toBeNull();
    expect(registry.needsRecovery('retry-test')).toBe(false);
    await registry.recoverServer('retry-test');
    expect(mocks.viewer).toHaveBeenCalledTimes(2);
    expect(mocks.discovery).toHaveBeenCalledTimes(1);
  });

  it('retries viewer verification after an API rejection with retained bearer credentials', async () => {
    mocks.discovery.mockResolvedValue(profile);
    await register();
    mocks.viewer.mockRejectedValueOnce(
      new ConnectError('authentication required', Code.Unauthenticated)
    );
    await registry.recoverServer('retry-test');
    expect(registry.getServer('retry-test')?.reauthRequiredAt).toBeNull();
    expect(registry.needsRecovery('retry-test')).toBe(true);

    mocks.viewer.mockResolvedValue(user);
    await registry.recoverServer('retry-test');
    expect(registry.needsRecovery('retry-test')).toBe(false);
    expect(registry.getStore('retry-test').currentUser.verifiedUserId).toBe(user.id);
  });

  it('deduplicates concurrent recovery and cannot restore a removed server', async () => {
    await register();
    mocks.discovery.mockResolvedValue(profile);
    let resolve!: (viewer: CurrentUser) => void;
    mocks.viewer.mockImplementation(
      () =>
        new Promise<CurrentUser>((finish) => {
          resolve = finish;
        })
    );
    const first = registry.recoverServer('retry-test');
    const second = registry.recoverServer('retry-test');
    await vi.waitFor(() => expect(mocks.viewer).toHaveBeenCalledTimes(2));
    registry.removeServer('retry-test');
    resolve(user);
    await Promise.all([first, second]);
    expect(registry.getServer('retry-test')).toBeUndefined();
    expect(registry.sessions.get('retry-test')).toBeUndefined();
    expect(registry.needsRecovery('retry-test')).toBe(false);
  });

  it('replaces a remote account before publishing its response without fetching it again', async () => {
    mocks.discovery.mockResolvedValue(profile);
    const previous = await register();
    registry.sessions.update('retry-test', { userId: 'old-user' });
    previous.currentUser.loading = false;
    previous.projection.rooms.set(
      'private-room',
      new RoomWithViewerState({ room: { id: 'private-room', name: 'private' } })
    );
    mocks.viewer.mockResolvedValue(user);
    await registry.recoverServer('retry-test');
    const current = registry.getStore('retry-test');
    expect(current).not.toBe(previous);
    expect(previous.currentUser.user).toBeUndefined();
    expect(current.currentUser.user).toEqual(user);
    expect(current.projection.rooms.has('private-room')).toBe(false);
    expect(mocks.viewer).toHaveBeenCalledTimes(2);
    expect(current.isAuthenticated).toBe(true);
  });
});
