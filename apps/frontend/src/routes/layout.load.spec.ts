import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    preloadPublicLocaleMessages: vi.fn(),
    getPublicServerInfo: vi.fn(),
    loadCurrentUser: vi.fn(),
    isBackendCapableOrigin: vi.fn(),
    init: vi.fn(),
    probeOrigin: vi.fn(),
    settleOriginUnauthenticated: vi.fn()
  }
}));

vi.mock('$lib/i18n/messages', () => ({
  preloadPublicLocaleMessages: mocks.preloadPublicLocaleMessages
}));

vi.mock('$lib/api-client/server', () => ({
  getPublicServerInfo: mocks.getPublicServerInfo
}));

vi.mock('$lib/auth/loadAuth', () => ({
  loadCurrentUser: mocks.loadCurrentUser
}));

vi.mock('$lib/runtimeOrigin', () => ({
  isBackendCapableOrigin: mocks.isBackendCapableOrigin
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    init: mocks.init,
    probeOrigin: mocks.probeOrigin,
    settleOriginUnauthenticated: mocks.settleOriginUnauthenticated
  }
}));

import { load } from './+layout';

const serverInfo = {
  name: 'Example server',
  iconUrl: null
};

describe('root layout load', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.preloadPublicLocaleMessages.mockResolvedValue(undefined);
    mocks.getPublicServerInfo.mockResolvedValue(serverInfo);
    mocks.loadCurrentUser.mockResolvedValue({ id: 'viewer-1' });
    mocks.isBackendCapableOrigin.mockReturnValue(true);
    mocks.probeOrigin.mockResolvedValue(undefined);
  });

  it('initialises and settles the registry before child routes load', async () => {
    const result = await load({ url: new URL('https://chat.example.test/chat/-') } as never);

    expect(mocks.init).toHaveBeenCalledOnce();
    expect(mocks.probeOrigin).toHaveBeenCalledWith(true, undefined, serverInfo);
    expect(mocks.settleOriginUnauthenticated).not.toHaveBeenCalled();
    expect(result).toMatchObject({ serverInfo, user: { id: 'viewer-1' } });
  });

  it('settles an unauthenticated origin after the discovery result is available', async () => {
    mocks.loadCurrentUser.mockResolvedValue(null);

    await load({ url: new URL('https://chat.example.test/chat/-') } as never);

    expect(mocks.probeOrigin).toHaveBeenCalledWith(false, undefined, serverInfo);
    expect(mocks.settleOriginUnauthenticated).toHaveBeenCalledOnce();
  });

  it('keeps a second discovery probe available after an initial request fails', async () => {
    mocks.getPublicServerInfo.mockResolvedValue(null);
    mocks.loadCurrentUser.mockResolvedValue(null);

    await load({ url: new URL('https://chat.example.test/chat/-') } as never);

    expect(mocks.probeOrigin).toHaveBeenCalledWith(false, undefined, undefined);
  });
});
