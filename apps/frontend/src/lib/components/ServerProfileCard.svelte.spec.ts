import '../../app.css';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import type { PublicServerInfo } from '@chatto/client/api/server';
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
  it('activates the whole card with the keyboard and description clicks', async () => {
    const onactivate = vi.fn();
    const { container } = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile({ iconUrl: null, bannerUrl: null }),
      onactivate,
      actionLabel: 'Join'
    });
    const action = container.querySelector<HTMLButtonElement>(
      '[data-testid="server-profile-card-action"]'
    )!;
    action.focus();
    await userEvent.keyboard('{Enter}');
    expect(onactivate).toHaveBeenCalledOnce();
    await userEvent.keyboard(' ');
    expect(onactivate).toHaveBeenCalledTimes(2);
    await userEvent.click(container.querySelector('[data-testid="server-card-description"] p')!);
    expect(onactivate).toHaveBeenCalledTimes(3);
    const card = container.querySelector('article')!.getBoundingClientRect();
    const target = action.getBoundingClientRect();
    expect(target.width).toBeCloseTo(card.width - 2, 0);
    expect(target.height).toBeCloseTo(card.height - 2, 0);
  });

  it('has no card action unless the caller supplies one', () => {
    const { container } = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile({ iconUrl: null, bannerUrl: null })
    });
    expect(container.querySelector('button, a')).toBeNull();
  });

  it('preserves modified link clicks from the description', () => {
    const { container } = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile({ iconUrl: null, bannerUrl: null }),
      href: 'https://chat.example'
    });
    const clicks: MouseEvent[] = [];
    container.querySelector('a')!.addEventListener('click', (event) => {
      event.preventDefault();
      clicks.push(event);
    });
    const description = container.querySelector('[data-testid="server-card-description"]')!;
    description.dispatchEvent(new MouseEvent('click', { bubbles: true, ctrlKey: true }));
    description.dispatchEvent(new MouseEvent('click', { bubbles: true, metaKey: true }));
    expect(clicks).toHaveLength(2);
    expect(clicks[0]?.ctrlKey).toBe(true);
    expect(clicks[1]?.metaKey).toBe(true);
  });

  it('does not activate a disabled card through its description', async () => {
    const onactivate = vi.fn();
    const { container } = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile({ iconUrl: null, bannerUrl: null }),
      onactivate,
      disabled: true
    });
    await userEvent.click(container.querySelector('[data-testid="server-card-description"] p')!);
    expect(onactivate).not.toHaveBeenCalled();
  });

  it('keeps long descriptions scrollable without increasing the card height', async () => {
    const short = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile({ iconUrl: null, bannerUrl: null })
    });
    const long = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile({
        iconUrl: null,
        bannerUrl: null,
        description: 'A long introduction with room for every detail.\n'.repeat(30)
      })
    });
    const viewport = long.container.querySelector<HTMLElement>(
      '[data-testid="server-card-description"]'
    )!;
    expect(viewport.getAttribute('tabindex')).toBe('0');
    expect(viewport.scrollHeight).toBeGreaterThan(viewport.clientHeight);
    await vi.waitFor(() => expect(viewport.style.maskImage).toMatch(/transparent\)$/));
    await vi.waitFor(() =>
      expect(
        short.container.querySelector<HTMLElement>('[data-testid="server-card-description"]')!.style
          .maskImage
      ).not.toContain('transparent')
    );
    expect(long.container.querySelector('article')!.getBoundingClientRect().height).toBe(
      short.container.querySelector('article')!.getBoundingClientRect().height
    );
    viewport.focus();
    expect(document.activeElement).toBe(viewport);
    viewport.scrollTop = viewport.scrollHeight;
    expect(viewport.scrollTop).toBeGreaterThan(0);
    await vi.waitFor(() => expect(viewport.style.maskImage).toContain('transparent, black'));
    expect(viewport.style.maskImage).not.toMatch(/transparent\)$/);
  });
  it('loads same-origin images through the private public-image path', async () => {
    const png = Uint8Array.from(
      atob(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=='
      ),
      (character) => character.charCodeAt(0)
    );
    const browserFetch = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(new Blob([png], { type: 'image/png' }), {
          status: 200,
          headers: { 'Content-Type': 'image/png' }
        })
    );
    vi.stubGlobal('fetch', browserFetch);

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
      const images = Array.from(container.querySelectorAll('img'));
      expect(images).toHaveLength(2);
      expect(images.every((image) => image.src.startsWith('blob:') && image.naturalWidth > 0)).toBe(
        true
      );
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

  it('falls back to plain branding when image data is invalid', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(new Blob(['invalid image'], { type: 'image/png' }), {
            status: 200,
            headers: { 'Content-Type': 'image/png' }
          })
      )
    );

    const { container } = render(ServerProfileCard, {
      origin: 'https://chat.example',
      profile: profile()
    });

    await vi.waitFor(() => {
      expect(container.querySelector('img')).toBeNull();
      expect(container.querySelector('[data-banner-fallback]')).not.toBeNull();
      expect(container.textContent).toContain('R');
    });
  });
});
