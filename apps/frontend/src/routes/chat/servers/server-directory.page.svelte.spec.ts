import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  servers: [] as Array<{
    id: string;
    url: string;
    name: string;
    iconUrl: string | null;
    addedAt: number;
  }>,
  authenticated: new Set<string>(),
  loadServerDirectory: vi.fn(),
  getPublicServerInfo: vi.fn(),
  startServerOAuthFlow: vi.fn(),
  startRemoteReauthentication: vi.fn(),
  goto: vi.fn()
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn()
}));
vi.mock('$lib/navigation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/navigation')>();
  return { ...actual, serverIdToSegment: (serverId: string) => serverId };
});
vi.mock('$lib/api-client/server', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/server')>();
  return { ...actual, getPublicServerInfo: mocks.getPublicServerInfo };
});
vi.mock('$lib/serverDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/serverDirectory')>();
  return { ...actual, loadServerDirectory: mocks.loadServerDirectory };
});
vi.mock('$lib/auth/reauth', () => ({
  startServerOAuthFlow: mocks.startServerOAuthFlow,
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    isAuthenticated: (serverId: string) => mocks.authenticated.has(serverId)
  }
}));

import Page from './+page.svelte';

function profile(name: string, overrides: Partial<PublicServerInfo> = {}): PublicServerInfo {
  return {
    name,
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    directLoginEnabled: true,
    accountCreationPolicy: 'open',
    welcomeMessage: null,
    description: `${name} description`,
    iconUrl: `https://cdn.example/${name}/logo.webp`,
    bannerUrl: `https://cdn.example/${name}/banner.webp`,
    authProviders: [],
    ...overrides
  };
}

function button(container: HTMLElement, label: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
    (candidate) => candidate.textContent?.trim() === label
  );
}

describe('Server Directory page', () => {
  beforeEach(() => {
    queryClient.clear();
    mocks.servers = [
      {
        id: 'joined',
        url: 'https://a.example',
        name: 'Already joined',
        iconUrl: null,
        addedAt: 1
      },
      {
        id: 'source',
        url: 'https://source.example',
        name: 'Source',
        iconUrl: null,
        addedAt: 2
      }
    ];
    mocks.authenticated = new Set(['joined']);
    mocks.loadServerDirectory.mockReset();
    mocks.getPublicServerInfo.mockReset();
    mocks.startServerOAuthFlow.mockReset();
    mocks.startServerOAuthFlow.mockResolvedValue(undefined);
    mocks.startRemoteReauthentication.mockReset();
    mocks.startRemoteReauthentication.mockResolvedValue(undefined);
    mocks.goto.mockReset();
    mocks.goto.mockResolvedValue(undefined);
  });

  it('keeps directory response order and marks registered entries as joined', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://z.example',
          profile: profile('Zulu'),
          sourceOrigins: ['https://source.example']
        },
        {
          origin: 'https://a.example',
          profile: profile('Alpha'),
          sourceOrigins: ['https://source.example']
        }
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="server-directory-entry"]')).toHaveLength(2);
    });
    const entries = Array.from(
      container.querySelectorAll<HTMLElement>('[data-testid="server-directory-entry"]')
    );
    expect(entries.map((entry) => entry.dataset.origin)).toEqual([
      'https://z.example',
      'https://a.example'
    ]);
    expect(entries[0]?.textContent).toContain('Zulu description');
    expect(entries[1]?.textContent).toContain('Joined');
    expect(entries[1]?.querySelector('img')?.src).toContain('/Alpha/banner.webp');
  });

  it('hides unavailable entries and reports partial source failures', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://offline.example',
          profile: null,
          sourceOrigins: ['https://source.example']
        },
        {
          origin: 'https://online.example',
          profile: profile('Online'),
          sourceOrigins: ['https://source.example']
        }
      ],
      failedSourceCount: 1,
      sourceCount: 2
    });

    const { container } = render(Page);

    await vi.waitFor(() => {
      expect(container.textContent).toContain('Some joined servers could not provide');
    });
    const entries = container.querySelectorAll<HTMLElement>(
      '[data-testid="server-directory-entry"]'
    );
    expect(entries).toHaveLength(1);
    expect(entries[0]?.dataset.origin).toBe('https://online.example');
    expect(container.textContent).not.toContain('offline.example');
    expect(container.textContent).not.toContain('public profile could not be loaded');
    expect(button(container, 'Sign-in unavailable')).toBeUndefined();
  });

  it('starts the OAuth flow for an advertised server', async () => {
    const remoteProfile = profile('Remote');
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://remote.example',
          profile: remoteProfile,
          sourceOrigins: ['https://source.example']
        }
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => {
      expect(
        container.querySelector('[data-testid="server-directory-entry-icon-action"]')
      ).toBeTruthy();
    });
    const iconAction = container.querySelector<HTMLButtonElement>(
      '[data-testid="server-directory-entry-icon-action"]'
    )!;
    expect(iconAction.getAttribute('aria-label')).toBe('Join: Remote');
    expect(iconAction.querySelector('.shimmer-hover.rounded-xl')).toBeTruthy();
    iconAction.click();

    await vi.waitFor(() => {
      expect(mocks.startServerOAuthFlow).toHaveBeenCalledWith(
        'https://remote.example',
        remoteProfile
      );
    });
  });

  it('opens an advertised server that is already joined', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://a.example',
          profile: profile('Alpha'),
          sourceOrigins: ['https://source.example']
        }
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Open')).toBeDefined());
    button(container, 'Open')?.click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/joined');
    });
  });

  it('probes a custom address and shows the same profile card', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 2
    });
    mocks.getPublicServerInfo.mockResolvedValue(profile('Custom'));

    const { container } = render(Page);
    const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
    input.value = 'custom.example';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => {
      expect(mocks.getPublicServerInfo).toHaveBeenCalledWith(
        'https://custom.example',
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      );
      expect(container.textContent).toContain('Custom description');
    });
  });

  it('shows compact recommendation provenance with the full accessible source list', async () => {
    mocks.servers = [
      { id: 'one', url: 'https://one.example', name: 'One', iconUrl: null, addedAt: 1 },
      { id: 'two', url: 'https://two.example', name: 'Two', iconUrl: null, addedAt: 2 },
      { id: 'three', url: 'https://three.example', name: 'Three', iconUrl: null, addedAt: 3 }
    ];
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://single.example',
          profile: profile('Single'),
          sourceOrigins: ['https://one.example']
        },
        {
          origin: 'https://double.example',
          profile: profile('Double'),
          sourceOrigins: ['https://one.example', 'https://two.example']
        },
        {
          origin: 'https://triple.example',
          profile: profile('Triple'),
          sourceOrigins: [
            'https://one.example',
            'https://two.example',
            'https://three.example'
          ]
        }
      ],
      failedSourceCount: 0,
      sourceCount: 3
    });

    const { container } = render(Page);
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="server-recommendation-sources"]')).toHaveLength(
        3
      );
    });
    const attributions = Array.from(
      container.querySelectorAll<HTMLElement>('[data-testid="server-recommendation-sources"]')
    );
    expect(attributions[0]?.textContent).toContain('Recommended by One');
    expect(attributions[1]?.textContent).toContain('Recommended by One and Two');
    expect(attributions[2]?.textContent).toContain('Recommended by One, Two, and 1 more');
    expect(attributions[2]?.getAttribute('aria-label')).toContain('One, Two, and Three');
    expect(attributions[2]?.title).toContain('One, Two, and Three');
  });
});
