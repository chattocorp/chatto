import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { NeighborhoodServerProfile, PublicServerInfo } from '@chatto/client/api/server';
import type { ServerDirectoryEntry } from '$lib/serverDirectory';
import type { KnownServers } from '@chatto/client/server/knownServers';

type MockServer = {
  id: string;
  url: string;
  name: string;
  iconUrl: string | null;
  addedAt: number;
};

const mocks = vi.hoisted(() => ({
  servers: [] as MockServer[],
  known: null as KnownServers | null,
  /** Servers that the directory added. A reactive map, so the directory updates. */
  added: null as Map<string, MockServer> | null,
  authenticated: new Set<string>(),
  loadServerDirectory: vi.fn(),
  getPublicServerInfo: vi.fn(),
  addSignedOutServer: vi.fn(),
  toastError: vi.fn(),
  goto: vi.fn(),
  pushState: vi.fn(),
  pageState: null as Map<string, unknown> | null
}));

vi.mock('$app/state', async () => {
  const { SvelteMap } = await import('svelte/reactivity');
  mocks.pageState = new SvelteMap();
  return {
    page: {
      get state() {
        return { modal: mocks.pageState!.get('modal') };
      }
    }
  };
});

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/client', async () => {
  const { KnownServers } = await import('@chatto/client/server/knownServers');
  mocks.known = new KnownServers();
  const { SvelteMap } = await import('svelte/reactivity');
  mocks.added = new SvelteMap<string, MockServer>();
  return {
    ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
    knownServers: mocks.known,
    serverRegistry: {
      get servers() {
        return [...mocks.servers, ...mocks.added!.values()];
      },
      isAuthenticated: (serverId: string) => mocks.authenticated.has(serverId),
      tryGetStore: () => undefined
    }
  };
});

// The real catalogue finds servers in the mocked registry; joining is mocked.
vi.mock('$lib/serverCatalogue', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/serverCatalogue')>()),
  addSignedOutServer: mocks.addSignedOutServer
}));

vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$lib/ui/toast', () => ({ toast: { error: mocks.toastError } }));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: mocks.pushState,
  replaceState: vi.fn()
}));
vi.mock('$lib/navigation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/navigation')>();
  return { ...actual, serverIdToSegment: (serverId: string) => serverId };
});
vi.mock('@chatto/client/api/server', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@chatto/client/api/server')>();
  return { ...actual, getPublicServerInfo: mocks.getPublicServerInfo };
});
vi.mock('$lib/serverDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/serverDirectory')>();
  return { ...actual, loadServerDirectory: mocks.loadServerDirectory };
});
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

/** Wait for the directory to load, open the address lookup, and enter a value. */
async function enterServerAddress(container: HTMLElement, value: string) {
  await vi.waitFor(() =>
    expect(
      container.querySelector('#add-server-url') ?? button(container, 'Connect by address')
    ).toBeTruthy()
  );
  button(container, 'Connect by address')?.click();
  flushSync();
  const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  container.querySelector('form')!.requestSubmit();
}

function link(container: HTMLElement, label: string): HTMLAnchorElement | undefined {
  return Array.from(container.querySelectorAll<HTMLAnchorElement>('a')).find(
    (candidate) => candidate.textContent?.trim() === label
  );
}

describe('Server Directory page', () => {
  beforeEach(() => {
    for (const server of mocks.known!.servers) mocks.known!.forget(server.origin);
    vi.spyOn(history, 'back').mockImplementation(() => mocks.pageState!.delete('modal'));
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
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 0
    });
    mocks.getPublicServerInfo.mockReset();
    mocks.toastError.mockReset();
    mocks.addSignedOutServer.mockReset();
    mocks.added?.clear();
    mocks.addSignedOutServer.mockImplementation(
      (url: string, { name, iconUrl }: { name: string; iconUrl: string | null }) => {
        const id = new URL(url).hostname;
        mocks.added!.set(id, { id, url, name, iconUrl, addedAt: Date.now() });
        return id;
      }
    );
    mocks.goto.mockReset();
    mocks.pushState.mockReset();
    mocks.pageState!.clear();
    mocks.pushState.mockImplementation((_url, state) => mocks.pageState!.set('modal', state.modal));
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
    expect(container.querySelectorAll('[data-testid="saved-server-entry"]')).toHaveLength(2);
    const savedCards = container.querySelectorAll('[data-testid="saved-server-entry"]');
    expect(savedCards[0]?.textContent).toContain('Joined');
    expect(savedCards[1]?.textContent).toContain('Needs login');
    expect(container.textContent).not.toContain('No recommended servers yet');
  });

  it('opens the address lookup from the pane header', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);

    await vi.waitFor(() => expect(container.textContent).toContain('Remote description'));
    expect(container.querySelector('#add-server-url')).toBeNull();

    const connect = button(container, 'Connect by address')!;
    expect(container.querySelector('.pane-page')?.firstElementChild?.contains(connect)).toBe(true);
    connect.click();
    flushSync();

    expect(container.querySelector('#add-server-url')).not.toBeNull();
    expect(container.querySelector('#add-server-url')?.closest('dialog')).not.toBeNull();
    expect(button(container, 'Connect by address')).toBeDefined();
    expect(container.textContent).toContain('Remote description');
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

  it('keeps recommendation order after filtering saved servers', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://z.example', cached('Zulu'), ['https://source.example']),
        entry('https://a.example', cached('Alpha'), ['https://source.example']),
        entry('https://b.example', cached('Bravo'), ['https://source.example'])
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
      'https://b.example'
    ]);
    expect(entries[0]?.textContent).toContain('Zulu description');
    expect(entries[1]?.textContent).toContain('Bravo description');
    expect(entries[1]?.querySelector('img')).toBeNull();
    const grid = container.querySelector('[data-testid="server-directory-grid"]')!;
    expect(grid.querySelectorAll('article')).toHaveLength(4);
    expect(entries.every((entry) => entry.parentElement === grid)).toBe(true);
    expect(container.textContent).not.toContain('Your servers');
    expect(container.textContent).not.toContain('Recommended servers');
    expect(container.textContent).not.toContain('Choose a server or connect by address.');
  });

  it('reports partial source failures', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://online.example', cached('Online'))],
      failedSourceCount: 1,
      sourceCount: 2
    });

    const { container } = render(Page);

    await vi.waitFor(() => {
      expect(container.textContent).toContain('Some servers could not provide');
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
            iconUrl: '/assets/neighborhood/logo',
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

  it.each(['0.5.0', '0.5.0-beta.9', '0.5.0-beta.15-custom', 'dev'])(
    'adds a recommended %s server and opens it',
    async (version) => {
      const remoteProfile = { ...profile('Remote'), version };
      mocks.getPublicServerInfo.mockResolvedValue(remoteProfile);
      mocks.loadServerDirectory.mockResolvedValue({
        entries: [entry('https://remote.example', cached('Remote', { version }))],
        failedSourceCount: 0,
        sourceCount: 2
      });

      const { container } = render(Page);
      await vi.waitFor(() => {
        expect(
          container.querySelector('[data-testid="server-directory-entry-action"]')
        ).toBeTruthy();
      });
      const iconAction = container.querySelector<HTMLButtonElement>(
        '[data-testid="server-directory-entry-action"]'
      )!;
      expect(iconAction.getAttribute('aria-label')).toBe('Join');
      iconAction.click();

      // The entry moves to saved servers, so the user cannot join it again.
      await vi.waitFor(() => expect(button(container, 'Join')).toBeUndefined());
      expect(
        container.querySelector('[data-testid="saved-server-entry"] a[href="/chat/remote.example"]')
      ).toBeTruthy();
      expect(mocks.goto).toHaveBeenCalledWith('/chat/remote.example', { replaceState: false });
      expect(mocks.getPublicServerInfo).toHaveBeenCalledWith(
        'https://remote.example',
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      );
      expect(mocks.addSignedOutServer).toHaveBeenCalledExactlyOnceWith('https://remote.example', {
        name: 'Remote',
        iconUrl: remoteProfile.iconUrl
      });
      expect(mocks.toastError).not.toHaveBeenCalled();
    }
  );

  it('stops a join when the current version is no longer compatible', async () => {
    const fetchImage = vi.fn().mockResolvedValue(new Response('', { status: 404 }));
    vi.stubGlobal('fetch', fetchImage);
    mocks.getPublicServerInfo.mockResolvedValue(
      profile('Remote', {
        version: '0.4.19',
        bannerUrl: 'https://remote.example/banner.png'
      })
    );
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
    // A refreshed profile uses the destination's images, not the cached source's.
    await vi.waitFor(() =>
      expect(fetchImage).toHaveBeenCalledWith(
        'https://remote.example/banner.png',
        expect.any(Object)
      )
    );
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
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
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
  });

  it('shows a failed join as a toast instead of a directory panel error', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'), ['https://source.example'])],
      failedSourceCount: 0,
      sourceCount: 1
    });
    mocks.getPublicServerInfo.mockRejectedValue(new TypeError('Failed to fetch'));
    const { container } = render(Page);
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="server-directory-entry-action"]')).toBeTruthy()
    );
    container
      .querySelector<HTMLButtonElement>('[data-testid="server-directory-entry-action"]')!
      .click();
    const message = 'Could not connect. Check the URL and try again.';
    await vi.waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith(message));
    expect(container.textContent).not.toContain(message);
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(
      container.querySelector<HTMLButtonElement>('[data-testid="server-directory-entry-action"]')!
        .disabled
    ).toBe(false);
  });

  it('shows a generic error when the server cannot be registered', async () => {
    mocks.getPublicServerInfo.mockResolvedValue(profile('Remote'));
    mocks.addSignedOutServer.mockImplementation(() => {
      throw new Error('The server could not be registered.');
    });
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(Page);
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    button(container, 'Join')?.click();

    await vi.waitFor(() => expect(mocks.toastError).toHaveBeenCalledOnce());
    expect(mocks.toastError).toHaveBeenCalledWith('Something went wrong');
    expect(button(container, 'Join')).toBeDefined();
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
    expect(button(container, 'Sign-in unavailable')).toBeUndefined();

    const iconAction = container.querySelector<HTMLAnchorElement>(
      '[data-testid="server-directory-entry-action"]'
    )!;
    expect(iconAction.href).toBe('https://old.example/');
    expect(iconAction.target).toBe('_blank');
    expect(iconAction.rel).toBe('noopener noreferrer');
    expect(iconAction.getAttribute('aria-label')).toBe('Open in new tab');
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
  });

  it('lists saved servers once, including signed-out servers, separately from recommendations', async () => {
    mocks.authenticated.clear();
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://a.example', cached('Alpha')),
        entry('https://new.example', cached('New'))
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });
    const { container } = render(Page);
    await vi.waitFor(() =>
      expect(container.querySelectorAll('[data-testid="server-directory-entry"]')).toHaveLength(1)
    );
    expect(container.querySelectorAll('[data-testid="saved-server-entry"]')).toHaveLength(2);
    expect(
      container.querySelector('[data-testid="saved-server-entry"] a[href="/chat/joined"]')
    ).toBeTruthy();
    expect(container.querySelector('[data-testid="saved-server-entry"]')?.textContent).toContain(
      'Already joined'
    );
    expect(
      container.querySelector('[data-testid="server-directory-entry"]')?.textContent
    ).toContain('New');
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('shows the address form immediately for an empty standalone catalogue', () => {
    mocks.servers = [];
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 0
    });
    const { container } = render(Page);
    expect(container.querySelector('#add-server-url')).toBeTruthy();
    expect(container.textContent).toContain('Chat just got real!');
    expect(container.textContent).toContain('Chatto communities live on independent servers');
    expect(container.textContent).not.toContain('Your servers');
    expect(container.textContent).not.toContain('Recommended servers');
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
  });

  it('links to the Chatto website without discovering or registering a server', () => {
    mocks.servers = [];
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 0
    });
    const { container } = render(Page);
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
    const website = container.querySelector('a[href="https://chatto.run"]');
    expect(website?.textContent).toBe('www.chatto.run');
    expect(website?.parentElement?.tagName).toBe('P');
    expect(container.textContent).toContain('Want to learn more?');
    expect(container.textContent).toContain('learn how to run your own server');
    expect(website?.getAttribute('rel')).toContain('noreferrer');
    expect(container.textContent).not.toContain('Chatto HQ');
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('opens a newly joined server and preserves the directory in history', async () => {
    mocks.getPublicServerInfo.mockResolvedValue(profile('Remote'));
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(ServerDirectory);
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    button(container, 'Join')?.click();
    await vi.waitFor(() => expect(link(container, 'Open')).toBeDefined());
    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/remote.example', { replaceState: false });
    });
    expect(mocks.goto).toHaveBeenCalledTimes(1);
  });

  it('ignores a lookup that completes after its modal closes', async () => {
    let finishLookup!: (value: PublicServerInfo) => void;
    mocks.getPublicServerInfo.mockImplementationOnce(
      () => new Promise<PublicServerInfo>((resolve) => (finishLookup = resolve))
    );
    const { container } = render(Page);
    await enterServerAddress(container, 'https://cancelled.example');
    await vi.waitFor(() => expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce());
    const signal = mocks.getPublicServerInfo.mock.calls[0]![1].signal as AbortSignal;
    flushSync(() => mocks.pageState!.delete('modal'));
    await vi.waitFor(() => expect(signal.aborted).toBe(true));
    button(container, 'Connect by address')!.click();
    flushSync();
    finishLookup(profile('Cancelled'));
    await vi.waitFor(() => expect(container.querySelector('form')).not.toBeNull());
    await Promise.resolve();
    expect(mocks.known!.servers).toHaveLength(0);
    expect(mocks.pageState!.get('modal')).toEqual({ type: 'serverAddress' });
  });

  it.each([true, false])(
    'opens a modal lookup directly with saved servers: %s',
    async (hasSaved) => {
      if (!hasSaved) {
        mocks.servers = [];
        mocks.known!.remember('https://known.example', profile('Known'));
      }
      const customProfile = profile('Custom');
      mocks.getPublicServerInfo.mockResolvedValue(customProfile);

      const { container } = render(Page);
      await enterServerAddress(container, 'custom.example');
      await vi.waitFor(() => expect(link(container, 'Open')).toBeDefined());
      expect(mocks.goto).toHaveBeenCalledExactlyOnceWith('/chat/custom.example', {
        replaceState: false
      });
      expect(mocks.addSignedOutServer).toHaveBeenCalledExactlyOnceWith('https://custom.example', {
        name: 'Custom',
        iconUrl: customProfile.iconUrl
      });
      expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();
      expect(mocks.getPublicServerInfo).toHaveBeenCalledWith(
        'https://custom.example',
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      );
    }
  );

  it('replaces first-run onboarding with known servers without registration', async () => {
    mocks.servers = [];
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 0
    });
    mocks.getPublicServerInfo.mockResolvedValue(profile('Custom'));
    const { container } = render(Page);
    await enterServerAddress(container, 'custom.example');
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    expect(container.textContent).not.toContain('Chat just got real!');
    expect(container.textContent).not.toContain('Want to learn more?');
    expect(container.querySelector('form')).toBeNull();
    expect(container.querySelector('h1')?.textContent).toContain('Server Directory');
    expect(container.querySelector('h2')?.textContent).toContain('Custom');
    expect(mocks.known!.servers.map((server) => server.origin)).toEqual(['https://custom.example']);
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.loadServerDirectory).toHaveBeenLastCalledWith(
      ['https://custom.example'],
      expect.anything()
    );
  });

  it('requires sign-in support even when an unknown version may connect', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 2
    });
    mocks.getPublicServerInfo.mockResolvedValue(
      profile('Custom build', { version: 'custom-build', authorizeUrl: '' })
    );

    const { container } = render(Page);
    await enterServerAddress(container, 'custom.example');

    await vi.waitFor(() => expect(button(container, 'Sign-in unavailable')).toBeDefined());
    expect(button(container, 'Sign-in unavailable')!.disabled).toBe(true);
    expect(link(container, 'Open in new tab')).toBeUndefined();
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
  });

  it('opens the full provenance from the compact recommendation line without contacting servers', async () => {
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
        container.querySelectorAll('[data-testid="server-recommendation-source"]')
      ).toHaveLength(3);
    });
    expect(container.textContent).toContain('Recommended by One');
    const infoButtons = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[data-testid="server-recommendation-source"]')
    );
    expect(infoButtons[2]!.textContent).toContain('+2');
    infoButtons[2]!.click();
    expect(mocks.pushState).toHaveBeenCalledWith('', {
      modal: {
        type: 'serverRecommendation',
        name: 'Triple',
        origin: 'https://triple.example',
        sources: [
          { name: 'One', origin: 'https://one.example' },
          { name: 'Two', origin: 'https://two.example' },
          { name: 'Three', origin: 'https://three.example' }
        ]
      }
    });
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
  });

  it('keeps registered and explicitly added servers before recommendations, without duplicates', async () => {
    mocks.known!.remember('https://mine.example', cached('Mine'));
    mocks.known!.remember('https://a.example', cached('Also registered'));
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [
        entry('https://other.example', cached('Other')),
        entry('https://mine.example', cached('Mine'))
      ],
      failedSourceCount: 0,
      sourceCount: 2
    });
    const { container } = render(Page);
    await vi.waitFor(() => expect(container.textContent).toContain('Other description'));
    const cards = Array.from(
      container.querySelectorAll<HTMLElement>('[data-testid="server-directory-grid"] > article')
    );
    expect(cards.map((card) => card.dataset.origin)).toEqual([
      'https://a.example',
      'https://source.example',
      'https://mine.example',
      'https://other.example'
    ]);
    expect(cards[0]!.textContent).toContain('Joined');
    expect(cards[2]!.textContent).toContain('Added by you');
    expect(cards[2]!.querySelector('[data-testid="server-recommendation-source"]')).toBeNull();
    expect(cards[3]!.textContent).toContain('Recommended by Source');
  });
});
