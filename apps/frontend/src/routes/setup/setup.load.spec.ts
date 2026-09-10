import { beforeEach, describe, expect, it, vi } from 'vitest';
const { getPublicServerInfo } = vi.hoisted(() => ({ getPublicServerInfo: vi.fn() }));
vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo }));
import { load } from './+page';

describe('setup route', () => {
  beforeEach(() => vi.clearAllMocks());
  it('loads a fresh setup status', async () => {
    getPublicServerInfo.mockResolvedValue({ setupRequired: true });
    await expect(load({ url: new URL('https://chat.example/setup') } as never)).resolves.toEqual({ setupServer: { setupRequired: true } });
  });
  it('redirects completed or disabled setup to login', async () => {
    getPublicServerInfo.mockResolvedValue({ setupRequired: false });
    await expect(load({ url: new URL('https://chat.example/setup') } as never)).rejects.toMatchObject({ status: 302, location: '/login' });
  });
  it('does not probe a standalone desktop origin', async () => {
    await expect(load({ url: new URL('chatto://desktop/setup') } as never)).rejects.toMatchObject({ location: '/login' });
    expect(getPublicServerInfo).not.toHaveBeenCalled();
  });
});
