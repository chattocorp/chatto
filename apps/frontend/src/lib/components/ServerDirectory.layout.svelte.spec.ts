import '../../app.css';
import { flushSync } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { ServerDirectoryEntry } from '$lib/serverDirectory';

const mocks = vi.hoisted(() => ({
  loadServerDirectory: vi.fn(),
  getPublicServerInfo: vi.fn()
}));

vi.mock('$lib/serverDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/serverDirectory')>();
  return { ...actual, loadServerDirectory: mocks.loadServerDirectory };
});
vi.mock('$lib/api-client/server', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/server')>();
  return { ...actual, getPublicServerInfo: mocks.getPublicServerInfo };
});
vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: vi.fn(),
  startServerOAuthFlow: vi.fn(),
  startServerOAuthFlowWhenReady: vi.fn()
}));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    servers: [{ id: 'home', url: 'https://home.example', name: 'Home', iconUrl: null, addedAt: 1 }],
    isAuthenticated: () => true
  }
}));

import ServerDirectory from './ServerDirectory.svelte';

function entry(origin: string, name: string): ServerDirectoryEntry {
  return {
    origin,
    profile: { name, version: '0.5.0', description: null, iconUrl: null, bannerUrl: null },
    imageOrigin: 'https://home.example',
    sourceOrigins: ['https://home.example']
  };
}

/** Render the directory into a host element with a fixed width in pixels. */
function renderAtWidth(width: number) {
  const host = document.createElement('div');
  host.style.width = `${width}px`;
  document.body.append(host);
  const result = render(ServerDirectory, { target: host, props: {} });
  return { ...result, host };
}

function bannerHeight(card: Element): number {
  return (card.querySelector('[data-banner-fallback]') as HTMLElement).offsetHeight;
}

function logoSize(card: Element): number {
  return (card.querySelector('[data-testid$="-icon-action"]') as HTMLElement).offsetWidth;
}

afterEach(() => {
  document.body.querySelectorAll(':scope > div[style]').forEach((host) => host.remove());
});

describe('Server Directory layout', () => {
  it('shows three full cards per row in the widest page pane', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://a.example', 'A'),
        entry('https://b.example', 'B'),
        entry('https://c.example', 'C')
      ],
      failedSourceCount: 0,
      sourceCount: 1
    });
    // A 1280 px window with the sidebar open leaves about 912 px of pane
    // content; the page's Panel then leaves about 862 px for the cards.
    const { host } = renderAtWidth(912);

    await vi.waitFor(() =>
      expect(host.querySelectorAll('[data-testid="server-directory-entry"]')).toHaveLength(3)
    );
    const cards = [...host.querySelectorAll('[data-testid="server-directory-entry"]')];
    const tops = new Set(cards.map((card) => (card as HTMLElement).getBoundingClientRect().top));
    expect(tops.size).toBe(1);
    expect(bannerHeight(cards[0]!)).toBe(96);
  });

  it('shows two columns of icon tiles, including an address-lookup result, in a narrow space', async () => {
    const { host, input } = await renderWithLookupResult(366);

    const cards = [...host.querySelectorAll<HTMLElement>('[data-testid="server-directory-entry"]')];
    for (const card of cards) {
      expect(bannerHeight(card)).toBe(0);
      expect(logoSize(card)).toBe(64);
      expect(card.offsetWidth).toBeLessThan(366 / 2);
    }
    // The lookup result comes first; the two directory tiles share one row.
    const [, first, second] = cards;
    expect(first!.getBoundingClientRect().top).toBe(second!.getBoundingClientRect().top);
    // Tiles still show who recommends each server.
    for (const tile of [first!, second!]) {
      const sources = tile.querySelector<HTMLElement>(
        '[data-testid="server-recommendation-sources"]'
      )!;
      expect(sources.offsetWidth).toBeGreaterThan(tile.offsetWidth / 2);
      expect(sources.offsetHeight).toBeGreaterThan(1);
    }
    const button = host.querySelector<HTMLButtonElement>('form button[type="submit"]')!;
    expect(button.getBoundingClientRect().top).toBe(input.getBoundingClientRect().top);
  });

  it('keeps the full address-lookup card in a wide space', async () => {
    const { host } = await renderWithLookupResult(912);

    for (const card of host.querySelectorAll('[data-testid="server-directory-entry"]')) {
      expect(bannerHeight(card)).toBe(96);
    }
  });
});

/** Render the directory at a width and look up one server by address. */
async function renderWithLookupResult(width: number) {
  mocks.loadServerDirectory.mockResolvedValue({
    entries: [entry('https://a.example', 'A'), entry('https://b.example', 'B')],
    failedSourceCount: 0,
    sourceCount: 1
  });
  mocks.getPublicServerInfo.mockResolvedValue({
    name: 'Custom',
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    directLoginEnabled: true,
    accountCreationPolicy: 'open',
    welcomeMessage: null,
    description: null,
    iconUrl: null,
    bannerUrl: null,
    authProviders: []
  } satisfies PublicServerInfo);
  const { host } = renderAtWidth(width);

  await vi.waitFor(() =>
    expect(host.querySelectorAll('[data-testid="server-directory-entry"]')).toHaveLength(2)
  );
  const input = host.querySelector<HTMLInputElement>('#add-server-url')!;
  input.value = 'custom.example';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  host.querySelector('form')!.requestSubmit();
  await vi.waitFor(() =>
    expect(host.querySelectorAll('[data-testid="server-directory-entry"]')).toHaveLength(3)
  );
  return { host, input };
}
