import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    saveReturnUrl: vi.fn(),
    serverId: 'origin' as string | null,
    origin: true,
    reauthRequiredAt: null as number | null,
    store: {
      currentUser: {
        loading: false,
        user: { id: 'viewer-1' } as { id: string } | undefined,
        load: vi.fn()
      }
    }
  }
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$lib/auth/returnNavigation', () => ({
  saveReturnUrl: mocks.saveReturnUrl
}));

vi.mock('$lib/navigation', () => ({
  segmentToServerId: () => mocks.serverId
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    tryGetStore: () => (mocks.serverId ? mocks.store : undefined),
    getServer: () =>
      mocks.serverId ? { id: mocks.serverId, reauthRequiredAt: mocks.reauthRequiredAt } : undefined,
    isOriginServer: () => mocks.origin
  }
}));

import { load } from './+layout';

function routeLoad(user: { id: string } | null = { id: 'viewer-1' }) {
  return load({
    params: { serverId: '-' },
    parent: async () => ({ user }),
    url: new URL('https://chat.example.test/chat/-/overview')
  } as never);
}

async function expectLoginRedirect(user?: { id: string } | null): Promise<void> {
  await expect(routeLoad(user)).rejects.toMatchObject({ status: 302, location: '/login' });
  expect(mocks.saveReturnUrl).toHaveBeenCalledWith('/chat/-/overview');
}

describe('server route layout load', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.serverId = 'origin';
    mocks.origin = true;
    mocks.reauthRequiredAt = null;
    mocks.store.currentUser.loading = false;
    mocks.store.currentUser.user = { id: 'viewer-1' };
    mocks.store.currentUser.load.mockResolvedValue(undefined);
  });

  it('redirects an unresolved server before the layout component mounts', async () => {
    mocks.serverId = null;

    await expectLoginRedirect();
  });

  it('uses the parent origin viewer without a second viewer request', async () => {
    await expect(routeLoad()).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.store.currentUser.load).not.toHaveBeenCalled();
  });

  it('redirects an unauthenticated origin and records its return URL', async () => {
    await expectLoginRedirect(null);
  });

  it('waits for the initial remote viewer request before deciding access', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.currentUser.loading = true;
    mocks.store.currentUser.user = undefined;
    mocks.store.currentUser.load.mockImplementation(async () => {
      mocks.store.currentUser.loading = false;
      mocks.store.currentUser.user = { id: 'remote-viewer' };
    });

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.store.currentUser.load).toHaveBeenCalledOnce();
    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });

  it('keeps the shell mounted for reauthentication recovery', async () => {
    mocks.reauthRequiredAt = Date.now();
    mocks.store.currentUser.user = undefined;

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });

  it('keeps the shell mounted when the initial remote viewer request requires reauthentication', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.currentUser.loading = true;
    mocks.store.currentUser.user = undefined;
    mocks.store.currentUser.load.mockImplementation(async () => {
      mocks.store.currentUser.loading = false;
      mocks.reauthRequiredAt = Date.now();
    });

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });
});
