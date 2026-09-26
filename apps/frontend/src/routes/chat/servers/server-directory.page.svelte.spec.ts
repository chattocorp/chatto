import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { NeighborhoodServerProfile, PublicServerInfo } from '$lib/api-client/server';
import type { ServerDirectoryEntry } from '$lib/serverDirectory';

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
  startServerOAuthFlowWhenReady: vi.fn(),
  startRemoteReauthentication: vi.fn(),
  toastError: vi.fn(),
  goto: vi.fn(),
  pageState: {} as App.PageState
}));

vi.mock('$app/state', () => ({
  page: {
    get state() {
      return mocks.pageState;
    }
  }
}));

vi.mock('$lib/ui/toast', () => ({ toast: { error: mocks.toastError } }));

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
  startServerOAuthFlowWhenReady: mocks.startServerOAuthFlowWhenReady,
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

import ServerDirectory from '$lib/components/ServerDirectory.svelte';
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

/** A cached Neighborhood profile, as returned by a registered server. */
function cached(name: string, overrides: Partial<NeighborhoodServerProfile> = {}) {
  return {
    name,
    version: '0.5.0',
    description: `${name} description`,
    iconUrl: `https://cdn.example/${name}/logo.webp`,
    bannerUrl: `https://cdn.example/${name}/banner.webp`,
    ...overrides
  };
}

function entry(
  origin: string,
  profileValue: NeighborhoodServerProfile,
  sourceOrigins = ['https://source.example']
): ServerDirectoryEntry {
  return { origin, profile: profileValue, imageOrigin: 'https://source.example', sourceOrigins };
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

describe('Server Directory page', () => {
  beforeEach(() => {
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
    mocks.toastError.mockReset();
    mocks.startServerOAuthFlow.mockReset();
    mocks.startServerOAuthFlow.mockResolvedValue(undefined);
    mocks.pageState = {};
    mocks.startServerOAuthFlowWhenReady.mockReset();
    // Like the real flow, the window opens first and then waits for the profile.
    mocks.startServerOAuthFlowWhenReady.mockImplementation(
      async (_origin: string, serverInfo: Promise<unknown>) => {
        await serverInfo;
      }
    );
    mocks.startRemoteReauthentication.mockReset();
    mocks.startRemoteReauthentication.mockResolvedValue(undefined);
    mocks.goto.mockReset();
    mocks.goto.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('loads the Neighborhoods of registered servers without a consent step', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);

    await vi.waitFor(() => expect(mocks.loadServerDirectory).toHaveBeenCalledOnce());
    expect(mocks.loadServerDirectory).toHaveBeenCalledWith(
      ['https://a.example', 'https://source.example'],
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(button(container, 'Discover servers')).toBeUndefined();
    expect(container.textContent).not.toContain('can see your IP address');
    await vi.waitFor(() => expect(container.textContent).toContain('No recommended servers yet'));
  });

  it('retries after every registered server failed', async () => {
    mocks.loadServerDirectory.mockResolvedValueOnce({
      entries: [],
      failedSourceCount: 2,
      sourceCount: 2
    });
    mocks.loadServerDirectory.mockResolvedValueOnce({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Try Again')).toBeDefined());
    button(container, 'Try Again')?.click();

    await vi.waitFor(() => expect(container.textContent).toContain('Remote description'));
    expect(mocks.loadServerDirectory).toHaveBeenCalledTimes(2);
  });

  it('keeps directory response order and marks registered entries as joined', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://z.example', cached('Zulu'), ['https://source.example']),
        entry('https://a.example', cached('Alpha'), ['https://source.example'])
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
    expect(entries[1]?.querySelector('img')).toBeNull();
    expect(container.textContent).toContain('Servers (2)');
  });

  it('reports partial source failures', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://online.example', cached('Online'))],
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
  });

  it('loads cached images only from the registered server that supplied them', async () => {
    const fetchImage = vi.fn(async () => new Response(null, { status: 404 }));
    vi.stubGlobal('fetch', fetchImage);
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry(
          'https://remote.example',
          cached('Remote', {
            iconUrl: 'https://source.example/assets/neighborhood/logo',
            bannerUrl: 'https://remote.example/banner.webp'
          })
        )
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);

    await vi.waitFor(() =>
      expect(fetchImage).toHaveBeenCalledWith(
        'https://source.example/assets/neighborhood/logo',
        expect.objectContaining({ credentials: 'omit', redirect: 'error' })
      )
    );
    expect(fetchImage).not.toHaveBeenCalledWith(
      'https://remote.example/banner.webp',
      expect.anything()
    );
    expect(container.textContent).toContain('Remote description');
  });

  it('opens sign-in from the click and then loads current sign-in data', async () => {
    const remoteProfile = profile('Remote');
    let resolveProfile: (value: PublicServerInfo) => void = () => {};
    mocks.getPublicServerInfo.mockReturnValue(
      new Promise<PublicServerInfo>((resolveValue) => (resolveProfile = resolveValue))
    );
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
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

    // The flow starts synchronously, before the current profile loads.
    expect(mocks.startServerOAuthFlowWhenReady).toHaveBeenCalledWith(
      'https://remote.example',
      expect.any(Promise),
      { replaceHistory: expect.any(Function) }
    );
    // The full page never replaces its own history entry.
    expect(mocks.startServerOAuthFlowWhenReady.mock.calls[0]?.[2].replaceHistory()).toBe(false);
    expect(mocks.getPublicServerInfo).toHaveBeenCalledWith(
      'https://remote.example',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    resolveProfile(remoteProfile);
    await expect(mocks.startServerOAuthFlowWhenReady.mock.calls[0]?.[1]).resolves.toBe(
      remoteProfile
    );
    expect(mocks.toastError).not.toHaveBeenCalled();
  });

  it('stops a join when the current version is no longer compatible', async () => {
    mocks.getPublicServerInfo.mockResolvedValue(profile('Remote', { version: '0.4.19' }));
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    button(container, 'Join')?.click();

    await vi.waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith('Sign-in unavailable'));
    await vi.waitFor(() => expect(link(container, 'Open in new tab')).toBeDefined());
  });

  it('stops a join when the server no longer supports sign-in', async () => {
    mocks.getPublicServerInfo.mockResolvedValue(profile('Remote', { authorizeUrl: '' }));
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    button(container, 'Join')?.click();

    await vi.waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith('Sign-in unavailable'));
    await vi.waitFor(() => expect(button(container, 'Sign-in unavailable')?.disabled).toBe(true));
  });

  it('shows OAuth failures as a toast instead of a directory panel error', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'), ['https://source.example'])],
      failedSourceCount: 0,
      sourceCount: 1
    });
    mocks.getPublicServerInfo.mockResolvedValue(profile('Remote'));
    mocks.startServerOAuthFlowWhenReady.mockRejectedValueOnce(new Error('Sign-in window closed'));
    const { container } = render(Page);
    await vi.waitFor(() =>
      expect(
        container.querySelector('[data-testid="server-directory-entry-icon-action"]')
      ).toBeTruthy()
    );
    container
      .querySelector<HTMLButtonElement>('[data-testid="server-directory-entry-icon-action"]')!
      .click();
    await vi.waitFor(() =>
      expect(mocks.toastError).toHaveBeenCalledWith('Failed to start sign-in.')
    );
    expect(container.textContent).not.toContain('Failed to start sign-in.');
    expect(
      container.querySelector<HTMLButtonElement>(
        '[data-testid="server-directory-entry-icon-action"]'
      )!.disabled
    ).toBe(false);
  });

  it('hands an incompatible advertised server off to its own client', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://old.example', cached('Old server', { version: '0.4.19' }), [
          'https://source.example'
        ])
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
    expect(mocks.startServerOAuthFlowWhenReady).not.toHaveBeenCalled();
    expect(mocks.startServerOAuthFlow).not.toHaveBeenCalled();
  });

  it('opens an advertised server that is already joined', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://a.example', cached('Alpha', { version: '0.4.19' }), [
          'https://source.example'
        ])
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Open')).toBeDefined());
    button(container, 'Open')?.click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/joined', { replaceState: false });
    });
  });

  it('replaces the dialog history entry when it opens a joined server', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://a.example', cached('Alpha'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(ServerDirectory, { inDialog: true });
    await vi.waitFor(() => expect(button(container, 'Open')).toBeDefined());
    button(container, 'Open')?.click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/joined', { replaceState: true });
    });
  });

  it('keeps sign-in for an incompatible joined server without a session', async () => {
    mocks.authenticated.clear();
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://a.example', cached('Alpha', { version: '0.4.19' }), [
          'https://source.example'
        ])
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
        expect.objectContaining({ id: 'joined' }),
        { replaceHistory: expect.any(Function) }
      );
    });
  });

  it('replaces the dialog history entry when it joins or signs in to a server', async () => {
    mocks.authenticated.clear();
    mocks.getPublicServerInfo.mockResolvedValue(profile('Remote'));
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://remote.example', cached('Remote')),
        entry('https://a.example', cached('Alpha'))
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });

    mocks.pageState = { modal: { type: 'addServer' } };

    const { container } = render(ServerDirectory, { inDialog: true });
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    button(container, 'Join')?.click();
    await vi.waitFor(() => expect(mocks.startServerOAuthFlowWhenReady).toHaveBeenCalled());
    button(container, 'Sign in')?.click();
    await vi.waitFor(() => expect(mocks.startRemoteReauthentication).toHaveBeenCalled());

    const joinOptions = mocks.startServerOAuthFlowWhenReady.mock.calls[0]?.[2];
    const signInOptions = mocks.startRemoteReauthentication.mock.calls[0]?.[1];
    expect(joinOptions.replaceHistory()).toBe(true);
    expect(signInOptions.replaceHistory()).toBe(true);

    // Sign-in can finish after the user closed the dialog. The chat entry
    // that is current then must stay in history.
    mocks.pageState = {};
    expect(joinOptions.replaceHistory()).toBe(false);
    expect(signInOptions.replaceHistory()).toBe(false);
  });

  it('starts sign-in for a server found by address from the click', async () => {
    const customProfile = profile('Custom');
    mocks.getPublicServerInfo.mockResolvedValue(customProfile);
    mocks.pageState = { modal: { type: 'addServer' } };

    const { container } = render(ServerDirectory, { inDialog: true });
    const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
    input.value = 'custom.example';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    container.querySelector('form')!.requestSubmit();
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    mocks.getPublicServerInfo.mockClear();

    button(container, 'Join')?.click();

    // The loaded profile is current, so the window opens without another request.
    expect(mocks.startServerOAuthFlow).toHaveBeenCalledWith(
      'https://custom.example',
      customProfile,
      { replaceHistory: expect.any(Function) }
    );
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
    expect(mocks.startServerOAuthFlow.mock.calls[0]?.[2].replaceHistory()).toBe(true);
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
    expect(mocks.startServerOAuthFlowWhenReady).not.toHaveBeenCalled();
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
        entry('https://single.example', cached('Single'), ['https://one.example']),
        entry('https://double.example', cached('Double'), [
          'https://one.example',
          'https://two.example'
        ]),
        entry('https://triple.example', cached('Triple'), [
          'https://one.example',
          'https://two.example',
          'https://three.example'
        ])
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
});
