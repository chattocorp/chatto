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

function link(container: HTMLElement, label: string): HTMLAnchorElement | undefined {
  return Array.from(container.querySelectorAll<HTMLAnchorElement>('a')).find(
    (candidate) => candidate.textContent?.trim() === label
  );
}

function recommendation(
  sourceOrigin = 'https://source.example',
  testimonial: string | null = null
) {
  return { sourceOrigin, testimonial };
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
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
        },
        {
          origin: 'https://a.example',
          profile: profile('Alpha'),
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
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
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
        },
        {
          origin: 'https://online.example',
          profile: profile('Online'),
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
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
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
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

  it('hands an incompatible advertised server off to its own client', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://old.example',
          profile: profile('Old server', { version: '0.4.19', authorizeUrl: '' }),
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
        }
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(link(container, 'Open in new tab')).toBeDefined());

    const externalAction = link(container, 'Open in new tab')!;
    expect(externalAction.href).toBe('https://old.example/');
    expect(externalAction.target).toBe('_blank');
    expect(externalAction.rel).toBe('noopener noreferrer');
    expect(externalAction.querySelector('.iconify')).toBeTruthy();
    expect(button(container, 'Sign-in unavailable')).toBeUndefined();

    const iconAction = container.querySelector<HTMLAnchorElement>(
      '[data-testid="server-directory-entry-icon-action"]'
    )!;
    expect(iconAction.href).toBe('https://old.example/');
    expect(iconAction.target).toBe('_blank');
    expect(iconAction.rel).toBe('noopener noreferrer');
    expect(iconAction.getAttribute('aria-label')).toBe('Open in new tab: Old server');
    expect(mocks.startServerOAuthFlow).not.toHaveBeenCalled();
  });

  it('opens an advertised server that is already joined', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://a.example',
          profile: profile('Alpha', { version: '0.4.19' }),
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
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

  it('keeps sign-in for an incompatible joined server without a session', async () => {
    mocks.authenticated.clear();
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://a.example',
          profile: profile('Alpha', { version: '0.4.19' }),
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
        }
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Sign in')).toBeDefined());
    expect(link(container, 'Open in new tab')).toBeUndefined();
    button(container, 'Sign in')?.click();

    await vi.waitFor(() => {
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'joined' })
      );
    });
  });

  it('keeps sign-in unavailable for a supported server without an OAuth URL', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://closed.example',
          profile: profile('Closed', { authorizeUrl: '' }),
          sourceOrigins: ['https://source.example'],
          recommendations: [recommendation()]
        }
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Sign-in unavailable')).toBeDefined());
    expect(button(container, 'Sign-in unavailable')?.disabled).toBe(true);
    expect(link(container, 'Open in new tab')).toBeUndefined();
    expect(
      container.querySelector('[data-testid="server-directory-entry-icon-action"]')
    ).toBeNull();
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

  it('hands a custom server with an unknown version off to its own client', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 2
    });
    mocks.getPublicServerInfo.mockResolvedValue(
      profile('Custom build', { version: 'custom-build', authorizeUrl: '' })
    );

    const { container } = render(Page);
    const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
    input.value = 'custom.example';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => expect(link(container, 'Open in new tab')).toBeDefined());
    const externalAction = link(container, 'Open in new tab')!;
    expect(externalAction.href).toBe('https://custom.example/');
    expect(externalAction.target).toBe('_blank');
    expect(externalAction.rel).toBe('noopener noreferrer');
    expect(mocks.startServerOAuthFlow).not.toHaveBeenCalled();
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
          sourceOrigins: ['https://one.example'],
          recommendations: [recommendation('https://one.example')]
        },
        {
          origin: 'https://double.example',
          profile: profile('Double'),
          sourceOrigins: ['https://one.example', 'https://two.example'],
          recommendations: [
            recommendation('https://one.example'),
            recommendation('https://two.example')
          ]
        },
        {
          origin: 'https://triple.example',
          profile: profile('Triple'),
          sourceOrigins: ['https://one.example', 'https://two.example', 'https://three.example'],
          recommendations: [
            recommendation('https://one.example'),
            recommendation('https://two.example'),
            recommendation('https://three.example')
          ]
        }
      ],
      failedSourceCount: 0,
      sourceCount: 3
    });

    const { container } = render(Page);
    await vi.waitFor(() => {
      expect(
        container.querySelectorAll('[data-testid="server-recommendation-sources"]')
      ).toHaveLength(3);
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

  it('shows source-specific testimonials in a labelled tapestry card', async () => {
    mocks.servers = [
      {
        id: 'one',
        url: 'https://one.example',
        name: 'One',
        iconUrl: 'https://cdn.example/one.webp',
        addedAt: 1
      },
      { id: 'two', url: 'https://two.example', name: 'Two', iconUrl: null, addedAt: 2 }
    ];
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        {
          origin: 'https://remote.example',
          profile: profile('Remote'),
          sourceOrigins: ['https://one.example', 'https://two.example'],
          recommendations: [
            recommendation(
              'https://one.example',
              'A **thoughtful** place for long conversations.\n\nPeople listen.'
            ),
            recommendation('https://two.example', 'Friendly people and excellent moderation.')
          ]
        }
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="server-testimonial"]')).toHaveLength(2);
    });
    const section = container.querySelector<HTMLElement>('[data-testid="server-testimonials"]')!;
    expect(section.getAttribute('aria-label')).toBe('Testimonials for Remote');
    expect(section.querySelectorAll('p')).toHaveLength(3);
    expect(section.querySelector('strong')?.textContent).toBe('thoughtful');
    expect(
      section.querySelector<HTMLImageElement>('[data-testid="server-testimonial-source-icon"] img')
        ?.src
    ).toBe('https://cdn.example/one.webp');
    expect(section.textContent).toContain('One');
    expect(section.textContent).toContain('Two');
    expect(section.textContent).not.toContain('Recommended by');
    const firstReview = section.querySelector<HTMLElement>('[data-testid="server-testimonial"]')!;
    expect(Array.from(firstReview.children, (child) => child.tagName)).toEqual([
      'FIGCAPTION',
      'BLOCKQUOTE'
    ]);
    expect(firstReview.querySelector('blockquote')).toHaveClass('bg-surface-emphasized');
    const profileCard = container.querySelector('[data-testid="server-directory-entry"]')!;
    expect(profileCard.contains(section)).toBe(false);
    expect(profileCard.nextElementSibling).toBe(section);
    expect(container.querySelector('.columns-1')).not.toBeNull();
  });
});
