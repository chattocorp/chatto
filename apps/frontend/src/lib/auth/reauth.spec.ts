import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const native = vi.hoisted(() => ({ available: false, authorize: vi.fn() }));
vi.mock('$lib/desktop/nativeAuthorization', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/desktop/nativeAuthorization')>()),
  hasNativeAuthorization: () => native.available,
  authorizeNatively: native.authorize
}));

const {
  addServerMock,
  clearOriginAuthenticationMock,
  generateServerIdMock,
  getPublicServerInfoMock,
  initServerInfoMock,
  gotoMock,
  replaceServerAuthenticationMock,
  updateServerMock
} = vi.hoisted(() => ({
  addServerMock: vi.fn(),
  clearOriginAuthenticationMock: vi.fn(),
  generateServerIdMock: vi.fn(() => 'remote-example'),
  getPublicServerInfoMock: vi.fn(),
  initServerInfoMock: vi.fn(() => Promise.resolve()),
  gotoMock: vi.fn(() => Promise.resolve()),
  replaceServerAuthenticationMock: vi.fn(),
  updateServerMock: vi.fn()
}));

vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('$app/paths', () => ({
  resolve: (_route: string, params?: { serverId?: string }) =>
    params?.serverId ? `/chat/${params.serverId}` : '/login'
}));
vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo: getPublicServerInfoMock }));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  generateServerId: generateServerIdMock,
  serverRegistry: {
    servers: [],
    addServer: addServerMock,
    getStore: vi.fn(() => ({ serverInfo: { init: initServerInfoMock } })),
    updateServer: updateServerMock,
    replaceServerAuthentication: replaceServerAuthenticationMock,
    clearOriginAuthentication: clearOriginAuthenticationMock
  }
}));

class FakeBroadcastChannel {
  static instances: FakeBroadcastChannel[] = [];

  onmessage: ((event: MessageEvent) => void) | null = null;
  closed = false;

  constructor(readonly name: string) {
    FakeBroadcastChannel.instances.push(this);
  }

  postMessage() {}

  close() {
    this.closed = true;
  }

  emit(data: unknown) {
    this.onmessage?.({ data } as MessageEvent);
  }
}

function memoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value)
  };
}

function browserHarness(openResult: Window | null) {
  const listeners = new Set<(event: MessageEvent) => void>();
  const open = vi.fn(() => openResult);
  const owner = {
    location: { origin: 'https://app.example' },
    screen: { availWidth: 1280, availHeight: 900 },
    screenX: 0,
    screenY: 0,
    outerWidth: 1280,
    outerHeight: 900,
    open,
    addEventListener: (type: string, listener: (event: MessageEvent) => void) => {
      if (type === 'message') listeners.add(listener);
    },
    removeEventListener: (type: string, listener: (event: MessageEvent) => void) => {
      if (type === 'message') listeners.delete(listener);
    },
    setInterval,
    clearInterval,
    setTimeout,
    clearTimeout
  } as unknown as Window;
  return { owner, open };
}

describe('remote server OAuth popup', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
    FakeBroadcastChannel.instances = [];
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel);
    vi.stubGlobal('sessionStorage', memoryStorage());
    getPublicServerInfoMock.mockReset();
    native.available = false;
    native.authorize.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('completes native PKCE without opening a popup and persists the mobile session', async () => {
    native.available = true;
    native.authorize.mockImplementation(async (raw: string, state: string) => {
      const url = new URL(raw);
      expect(url.searchParams.get('client_id')).toBe('eu.chattocorp.chatto.mobile');
      expect(url.searchParams.get('redirect_uri')).toBe(
        'eu.chattocorp.chatto.mobile:/oauth/callback'
      );
      expect(url.searchParams.get('code_challenge_method')).toBe('S256');
      expect(url.searchParams.get('code_challenge')).toBeTruthy();
      expect(url.searchParams.get('state')).toBe(state);
      return { state, code: 'native-code' };
    });
    const open = vi.fn();
    vi.stubGlobal('window', { open });
    const exchange = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(
          JSON.stringify({
            access_token: 'cht_ATtoken',
            refresh_token: 'cht_RT_token',
            expires_in: 900,
            refresh_token_expires_in: 7_776_000
          }),
          { headers: { 'Content-Type': 'application/json' } }
        )
    );
    vi.stubGlobal('fetch', exchange);
    const { startServerOAuthFlow } = await import('./reauth');
    await startServerOAuthFlow('https://remote.example', {
      name: 'Remote',
      authorizeUrl: '/oauth/authorize',
      iconUrl: null
    });
    expect(open).not.toHaveBeenCalled();
    expect(FakeBroadcastChannel.instances).toHaveLength(0);
    expect(JSON.parse(exchange.mock.calls[0]?.[1]?.body as string)).toMatchObject({
      code: 'native-code',
      client_id: 'eu.chattocorp.chatto.mobile',
      redirect_uri: 'eu.chattocorp.chatto.mobile:/oauth/callback'
    });
    expect(addServerMock).toHaveBeenCalledOnce();
    expect(gotoMock).toHaveBeenCalledWith('/chat/remote-example');
  });

  it('does not exchange credentials or register a server after native cancellation', async () => {
    native.available = true;
    native.authorize.mockRejectedValue(new Error('Sign-in cancelled'));
    const exchange = vi.fn();
    vi.stubGlobal('fetch', exchange);
    const { startServerOAuthFlow } = await import('./reauth');
    await expect(
      startServerOAuthFlow('https://remote.example', {
        name: 'Remote',
        authorizeUrl: '/oauth/authorize',
        iconUrl: null
      })
    ).rejects.toThrow('Sign-in cancelled');
    expect(exchange).not.toHaveBeenCalled();
    expect(addServerMock).not.toHaveBeenCalled();
  });

  it('uses the built-in OAuth client identity for Chatto Desktop', async () => {
    const { oauthClientIdForLocation } = await import('./reauth');
    expect(
      oauthClientIdForLocation({
        origin: 'chatto://desktop',
        protocol: 'chatto:',
        host: 'desktop'
      })
    ).toBe('chatto://desktop');
  });

  it('keeps the main client mounted while completing PKCE through a popup', async () => {
    const popup = {
      closed: false,
      opener: {} as Window,
      location: { href: '' },
      close: vi.fn(function (this: { closed: boolean }) {
        this.closed = true;
      })
    } as unknown as Window;
    const { owner, open } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              access_token: 'cht_ATtoken',
              refresh_token: 'cht_RT_token',
              expires_in: 900,
              refresh_token_expires_in: 7_776_000,
              user: { id: 'user-1', login: 'alice', displayName: 'Alice' }
            }),
            { headers: { 'Content-Type': 'application/json' } }
          )
      )
    );

    const { startServerOAuthFlow } = await import('./reauth');
    const beforeNavigate = vi.fn();
    const completion = startServerOAuthFlow(
      'https://remote.example',
      {
        name: 'Remote',
        authorizeUrl: '/oauth/authorize',
        iconUrl: null
      },
      beforeNavigate,
      'authling'
    );

    // window.open happens before the first asynchronous PKCE operation, so it
    // remains associated with the user's click and avoids popup blocking.
    expect(open).toHaveBeenCalledOnce();
    expect(open).toHaveBeenCalledWith(
      'about:blank',
      expect.stringMatching(/^chatto-oauth-/),
      expect.stringContaining('width=560,height=760')
    );
    await vi.waitFor(() => expect(popup.location.href).toContain('/oauth/authorize?'));
    expect(popup.opener).toBeNull();

    const authorizeURL = new URL(popup.location.href);
    const state = authorizeURL.searchParams.get('state');
    expect(state).toBeTruthy();
    expect(authorizeURL.searchParams.get('redirect_uri')).toBe(
      'https://app.example/servers/callback?mode=popup'
    );
    expect(authorizeURL.searchParams.get('client_id')).toBe(
      'https://app.example/oauth/frontend-client-metadata.json'
    );
    expect(authorizeURL.searchParams.get('provider_id')).toBe('authling');

    const responseChannel = FakeBroadcastChannel.instances.find(
      (channel) => channel.name === `chatto:oauth-popup:${state}`
    );
    expect(responseChannel).toBeDefined();
    responseChannel!.emit({
      type: 'chatto:oauth-popup-response',
      state,
      code: 'cht_ACcode'
    });

    await completion;

    expect(fetch).toHaveBeenCalledWith(
      'https://remote.example/oauth/token',
      expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining(
          '"redirect_uri":"https://app.example/servers/callback?mode=popup"'
        )
      })
    );
    expect(JSON.parse(vi.mocked(fetch).mock.calls[0]![1]!.body as string)).toMatchObject({
      client_id: 'https://app.example/oauth/frontend-client-metadata.json'
    });
    expect(addServerMock).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'remote-example',
        url: 'https://remote.example'
      }),
      expect.objectContaining({ token: 'cht_ATtoken', userId: 'user-1' })
    );
    expect(initServerInfoMock).toHaveBeenCalledOnce();
    expect(beforeNavigate).toHaveBeenCalledOnce();
    expect(beforeNavigate.mock.invocationCallOrder[0]).toBeLessThan(
      gotoMock.mock.invocationCallOrder[0]!
    );
    expect(gotoMock).toHaveBeenCalledWith('/chat/remote-example');
    expect(popup.close).toHaveBeenCalledOnce();
  });

  it('opens before server discovery completes without selecting a login provider', async () => {
    const popup = {
      closed: false,
      opener: {} as Window,
      location: { href: '' },
      close: vi.fn(function (this: { closed: boolean }) {
        this.closed = true;
      })
    } as unknown as Window;
    const { owner, open } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              access_token: 'cht_ATtoken',
              refresh_token: 'cht_RT_token',
              expires_in: 900,
              refresh_token_expires_in: 7_776_000
            }),
            {
              headers: { 'Content-Type': 'application/json' }
            }
          )
      )
    );

    let finishDiscovery: ((info: Record<string, unknown>) => void) | undefined;
    getPublicServerInfoMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishDiscovery = resolve;
        })
    );
    const { startRemoteReauthentication } = await import('./reauth');
    const completion = startRemoteReauthentication({
      id: 'remote',
      url: 'https://remote.example',
      name: 'Saved Remote',
      iconUrl: null,
      token: null,
      userId: null,
      userLogin: null,
      userDisplayName: null,
      userAvatarUrl: null,
      reauthRequiredAt: null,
      addedAt: 0
    });

    expect(open).toHaveBeenCalledOnce();
    expect(popup.location.href).toBe('');

    finishDiscovery?.({
      name: 'Discovered Remote',
      authorizeUrl: '/oauth/authorize',
      iconUrl: null,
      authProviders: [{ id: 'authling' }]
    });

    await vi.waitFor(() => expect(popup.location.href).toContain('/oauth/authorize?'));
    const authorizeURL = new URL(popup.location.href);
    expect(authorizeURL.searchParams.has('provider_id')).toBe(false);

    const state = authorizeURL.searchParams.get('state');
    FakeBroadcastChannel.instances
      .find((channel) => channel.name === `chatto:oauth-popup:${state}`)
      ?.emit({ type: 'chatto:oauth-popup-response', state, code: 'cht_ACcode' });

    await completion;
    expect(addServerMock).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Discovered Remote' }),
      expect.objectContaining({ token: 'cht_ATtoken' })
    );
  });

  it('closes the blank popup when server discovery fails', async () => {
    const popup = {
      closed: false,
      opener: {} as Window,
      location: { href: '' },
      close: vi.fn(function (this: { closed: boolean }) {
        this.closed = true;
      })
    } as unknown as Window;
    const { owner } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    getPublicServerInfoMock.mockRejectedValueOnce(new Error('discovery failed'));

    const { startRemoteReauthentication } = await import('./reauth');
    await expect(
      startRemoteReauthentication({
        id: 'remote',
        url: 'https://remote.example',
        name: 'Remote',
        iconUrl: null,
        token: null,
        userId: null,
        userLogin: null,
        userDisplayName: null,
        userAvatarUrl: null,
        reauthRequiredAt: null,
        addedAt: 0
      })
    ).rejects.toThrow('discovery failed');

    expect(popup.close).toHaveBeenCalledOnce();
    expect(sessionStorage.getItem('chatto:oauth:flow')).toBeNull();
  });

  it('opens a join window before pending server data settles and closes it on rejection', async () => {
    const popup = {
      closed: false,
      opener: {} as Window,
      location: { href: '' },
      close: vi.fn(function (this: { closed: boolean }) {
        this.closed = true;
      })
    } as unknown as Window;
    const { owner, open } = browserHarness(popup);
    vi.stubGlobal('window', owner);

    let rejectServerInfo: ((error: Error) => void) | undefined;
    const serverInfo = new Promise<never>((_resolve, reject) => {
      rejectServerInfo = reject;
    });
    const { startServerOAuthFlowWhenReady } = await import('./reauth');
    const completion = startServerOAuthFlowWhenReady('https://remote.example', serverInfo);

    expect(open).toHaveBeenCalledOnce();
    expect(popup.location.href).toBe('');

    rejectServerInfo?.(new Error('join unavailable'));
    await expect(completion).rejects.toThrow('join unavailable');
    expect(popup.close).toHaveBeenCalledOnce();
    expect(sessionStorage.getItem('chatto:oauth:flow')).toBeNull();
  });

  it('fails without navigating the main window when the popup is blocked', async () => {
    const { owner } = browserHarness(null);
    vi.stubGlobal('window', owner);

    const { startServerOAuthFlow } = await import('./reauth');
    await expect(
      startServerOAuthFlow('https://remote.example', {
        name: 'Remote',
        authorizeUrl: '/oauth/authorize',
        iconUrl: null
      })
    ).rejects.toThrow('could not be opened');

    expect(gotoMock).not.toHaveBeenCalled();
    expect(sessionStorage.getItem('chatto:oauth:flow')).toBeNull();
  });
});

describe('origin server reauthentication', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
    vi.stubGlobal('sessionStorage', memoryStorage());
    vi.stubGlobal('window', {
      location: {
        pathname: '/chat/origin',
        search: '?room=general'
      }
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('opens sign-in without an external identity linking error', async () => {
    const { beginOriginReauthentication } = await import('./reauth');

    beginOriginReauthentication();

    expect(clearOriginAuthenticationMock).toHaveBeenCalledOnce();
    expect(gotoMock).toHaveBeenCalledWith('/login?redirect=%2Fchat%2Forigin%3Froom%3Dgeneral', {
      invalidateAll: true
    });
    expect(sessionStorage.getItem('returnUrl')).toBe('/chat/origin?room=general');
  });
});

describe('authorization window dimensions', () => {
  it('reserves room for browser chrome on smaller screens', async () => {
    const { authorizationWindowFeatures } = await import('../oauth/authorizationWindow');
    const { owner } = browserHarness(null);
    Object.assign(owner.screen, { availWidth: 500, availHeight: 700 });
    expect(authorizationWindowFeatures(owner)).toContain('width=468,height=600');
  });
});
