import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { NeighborhoodServerProfile, PublicServerInfo } from '@chatto/client/api/server';
import type { ServerDirectoryEntry } from '$lib/serverDirectory';

type MockServer = {
  id: string;
  url: string;
  name: string;
  iconUrl: string | null;
  addedAt: number;
};

const mocks = vi.hoisted(() => ({
  servers: [] as MockServer[],
  /** Servers that the directory added. A reactive map, so the directory updates. */
  added: null as Map<string, MockServer> | null,
  authenticated: new Set<string>(),
  loadServerDirectory: vi.fn(),
  getPublicServerInfo: vi.fn(),
  addSignedOutServer: vi.fn(),
  toastError: vi.fn(),
  goto: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/client', async () => {
  const { SvelteMap } = await import('svelte/reactivity');
  mocks.added = new SvelteMap<string, MockServer>();
  return {
    ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
    serverRegistry: {
      get servers() {
        return [...mocks.servers, ...mocks.added!.values()];
      },
      isAuthenticated: (serverId: string) => mocks.authenticated.has(serverId)
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
  pushState: vi.fn(),
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

  it('leads with recommendations and opens the address lookup from a button', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(ServerDirectory, { inDialog: true });

    await vi.waitFor(() => expect(container.textContent).toContain('Remote description'));
    expect(container.querySelector('#add-server-url')).toBeNull();

    button(container, 'Connect by address')!.click();
    flushSync();

    expect(container.querySelector('#add-server-url')).not.toBeNull();
    expect(button(container, 'Connect by address')).toBeUndefined();
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
    expect(container.textContent).toContain('Recommended servers (2)');
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

  it('adds a recommended server with its current profile and stays in the directory', async () => {
    const remoteProfile = profile('Remote');
    mocks.getPublicServerInfo.mockResolvedValue(remoteProfile);
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

    // The entry changes to the joined state, so the user cannot join it again.
    await vi.waitFor(() => expect(button(container, 'Open')).toBeDefined());
    expect(button(container, 'Join')).toBeUndefined();
    expect(container.textContent).toContain('Joined');
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.getPublicServerInfo).toHaveBeenCalledWith(
      'https://remote.example',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(mocks.addSignedOutServer).toHaveBeenCalledExactlyOnceWith('https://remote.example', {
      name: 'Remote',
      iconUrl: remoteProfile.iconUrl
    });
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
      expect(
        container.querySelector('[data-testid="server-directory-entry-icon-action"]')
      ).toBeTruthy()
    );
    container
      .querySelector<HTMLButtonElement>('[data-testid="server-directory-entry-icon-action"]')!
      .click();
    const message = 'Could not connect. Check the URL and try again.';
    await vi.waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith(message));
    expect(container.textContent).not.toContain(message);
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(
      container.querySelector<HTMLButtonElement>(
        '[data-testid="server-directory-entry-icon-action"]'
      )!.disabled
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
    expect(externalAction.querySelector('.iconify')).toBeTruthy();
    expect(button(container, 'Sign-in unavailable')).toBeUndefined();

    const iconAction = container.querySelector<HTMLAnchorElement>(
      '[data-testid="server-directory-entry-icon-action"]'
    )!;
    expect(iconAction.href).toBe('https://old.example/');
    expect(iconAction.target).toBe('_blank');
    expect(iconAction.rel).toBe('noopener noreferrer');
    expect(iconAction.getAttribute('aria-label')).toBe('Open in new tab: Old server');
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
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

  it('opens an incompatible joined server without a session instead of signing in', async () => {
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
    await vi.waitFor(() => expect(button(container, 'Open')).toBeDefined());
    expect(link(container, 'Open in new tab')).toBeUndefined();
    button(container, 'Open')?.click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/joined', { replaceState: false });
    });
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
  });

  it('keeps the dialog open after a join and replaces its history entry on open', async () => {
    mocks.getPublicServerInfo.mockResolvedValue(profile('Remote'));
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [entry('https://remote.example', cached('Remote'))],
      failedSourceCount: 0,
      sourceCount: 2
    });

    const { container } = render(ServerDirectory, { inDialog: true });
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    button(container, 'Join')?.click();
    await vi.waitFor(() => expect(button(container, 'Open')).toBeDefined());
    expect(mocks.goto).not.toHaveBeenCalled();

    button(container, 'Open')?.click();
    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/remote.example', { replaceState: true });
    });
  });

  it('adds a server found by address without another profile request', async () => {
    const customProfile = profile('Custom');
    mocks.getPublicServerInfo.mockResolvedValue(customProfile);

    const { container } = render(ServerDirectory, { inDialog: true });
    await enterServerAddress(container, 'custom.example');
    await vi.waitFor(() => expect(button(container, 'Join')).toBeDefined());
    mocks.getPublicServerInfo.mockClear();

    button(container, 'Join')?.click();

    await vi.waitFor(() => expect(button(container, 'Open')).toBeDefined());
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.addSignedOutServer).toHaveBeenCalledExactlyOnceWith('https://custom.example', {
      name: 'Custom',
      iconUrl: customProfile.iconUrl
    });
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
  });

  it('probes a custom address and shows the same profile card', async () => {
    mocks.loadServerDirectory.mockResolvedValue({
      entries: [],
      failedSourceCount: 0,
      sourceCount: 2
    });
    mocks.getPublicServerInfo.mockResolvedValue(profile('Custom'));

    const { container } = render(Page);
    await enterServerAddress(container, 'custom.example');

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
    await enterServerAddress(container, 'custom.example');

    await vi.waitFor(() => expect(link(container, 'Open in new tab')).toBeDefined());
    const externalAction = link(container, 'Open in new tab')!;
    expect(externalAction.href).toBe('https://custom.example/');
    expect(externalAction.target).toBe('_blank');
    expect(externalAction.rel).toBe('noopener noreferrer');
    expect(mocks.addSignedOutServer).not.toHaveBeenCalled();
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
