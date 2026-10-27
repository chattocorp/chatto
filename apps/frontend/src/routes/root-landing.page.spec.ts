import { beforeEach, describe, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ origin: undefined as { id: string } | undefined }));
vi.mock('$lib/client', () => ({
  serverRegistry: {
    get originServer() {
      return mocks.origin;
    }
  }
}));
import { load } from './+page';
import { load as loginLoad } from './login/+page';

async function expectRedirect(
  url: string,
  user: { id: string } | null,
  location: string
): Promise<void> {
  await expect(
    load({
      parent: async () => ({ user }),
      url: new URL(url)
    } as never)
  ).rejects.toMatchObject({ status: 302, location });
}

describe('root landing load', () => {
  beforeEach(() => {
    mocks.origin = { id: 'origin' };
  });
  it('redirects unauthenticated visitors to login before rendering', async () => {
    await expectRedirect('https://chat.example.test/', null, '/login');
  });

  it('preserves query parameters when redirecting authenticated visitors to chat', async () => {
    await expectRedirect(
      'https://chat.example.test/?welcome=true',
      { id: 'user-1' },
      '/chat?welcome=true'
    );
  });

  it.each(['https://static.example/', 'chatto://desktop/'])(
    'opens the directory on a host without an origin server: %s',
    async (url) => {
      mocks.origin = undefined;
      await expectRedirect(url, null, '/chat/servers');
    }
  );

  it('keeps an existing origin server on login when discovery is unavailable', async () => {
    await expectRedirect('https://chat.example.test/', null, '/login');
  });

  it('redirects the old standalone login entry to the directory', async () => {
    mocks.origin = undefined;
    await expect(
      loginLoad({
        parent: async () => ({ user: null }),
        url: new URL('https://static.example/login')
      } as never)
    ).rejects.toMatchObject({ location: '/chat/servers' });
  });

  it('preserves an explicit authorization continuation when discovery is unavailable', async () => {
    mocks.origin = undefined;
    await expect(
      loginLoad({
        parent: async () => ({ user: null }),
        url: new URL('https://server.example/login?redirect=%2Foauth%2Fauthorize%3Fstate%3Dexample')
      } as never)
    ).resolves.toMatchObject({ redirectUrl: '/oauth/authorize?state=example' });
  });
});
