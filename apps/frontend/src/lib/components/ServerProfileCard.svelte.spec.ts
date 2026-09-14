import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import ServerProfileCard from './ServerProfileCard.svelte';

function profile(overrides: Partial<PublicServerInfo> = {}): PublicServerInfo {
  return {
    name: 'Remote Chatto',
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    directLoginEnabled: true,
    accountCreationPolicy: 'open',
    welcomeMessage: null,
    description: 'A remote server.',
    iconUrl: '/assets/logo.webp',
    bannerUrl: 'https://chat.example/assets/banner.webp',
    authProviders: [],
    ...overrides
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('ServerProfileCard public images', () => {
  it('loads same-origin images through the private public-image path', async () => {
    const browserFetch = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(new Blob(['image'], { type: 'image/webp' }), {
          status: 200,
          headers: { 'Content-Type': 'image/webp' }
        })
    );
    vi.stubGlobal('fetch', browserFetch);
    vi.spyOn(URL, 'createObjectURL')
      .mockReturnValueOnce('blob:banner')
      .mockReturnValueOnce('blob:logo');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});

    const { container } = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile()
    });

    await vi.waitFor(() => expect(browserFetch).toHaveBeenCalledTimes(2));
    expect(browserFetch.mock.calls.map(([source]) => source).sort()).toEqual([
      'https://chat.example/assets/banner.webp',
      'https://chat.example/assets/logo.webp'
    ]);
    for (const [, options] of browserFetch.mock.calls) {
      expect(options).toMatchObject({
        credentials: 'omit',
        redirect: 'error',
        referrerPolicy: 'no-referrer'
      });
    }
    await vi.waitFor(() => {
      expect(
        Array.from(container.querySelectorAll('img'))
          .map(({ src }) => src)
          .sort()
      ).toEqual(['blob:banner', 'blob:logo']);
    });
  });

  it('does not request profile images from another origin', () => {
    const browserFetch = vi.fn();
    vi.stubGlobal('fetch', browserFetch);

    const { container } = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile({
        iconUrl: 'https://tracker.example/logo.webp',
        bannerUrl: 'https://tracker.example/banner.webp'
      })
    });

    expect(browserFetch).not.toHaveBeenCalled();
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain('R');
  });
});
