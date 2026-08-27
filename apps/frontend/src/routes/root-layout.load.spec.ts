import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getPublicServerInfo, loadCurrentUser, preloadPublicLocaleMessages } = vi.hoisted(() => ({
  getPublicServerInfo: vi.fn(),
  loadCurrentUser: vi.fn(),
  preloadPublicLocaleMessages: vi.fn()
}));

vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo }));
vi.mock('$lib/auth/loadAuth', () => ({ loadCurrentUser }));
vi.mock('$lib/i18n/messages', () => ({ preloadPublicLocaleMessages }));

import { load } from './+layout';

describe('root layout origin loading', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    preloadPublicLocaleMessages.mockResolvedValue(undefined);
    getPublicServerInfo.mockResolvedValue({ name: 'Web server' });
    loadCurrentUser.mockResolvedValue({ id: 'viewer' });
  });

  it('loads discovery and viewer state from an HTTP origin', async () => {
    const data = await load({ url: new URL('https://chat.example/welcome') } as never);

    expect(getPublicServerInfo).toHaveBeenCalledWith('https://chat.example');
    expect(loadCurrentUser).toHaveBeenCalledOnce();
    expect(data).toMatchObject({
      serverInfo: { name: 'Web server' },
      user: { id: 'viewer' }
    });
  });

  it('does not probe the Chatto Desktop application origin', async () => {
    const data = await load({ url: new URL('chatto://desktop/login') } as never);

    expect(preloadPublicLocaleMessages).toHaveBeenCalledOnce();
    expect(getPublicServerInfo).not.toHaveBeenCalled();
    expect(loadCurrentUser).not.toHaveBeenCalled();
    expect(data).toMatchObject({ serverInfo: null, serverInfoLoaded: true, user: null });
  });
});
