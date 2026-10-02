import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { authorizationLaunchTarget } from '$lib/test-utils/authorizationWindow';

const native = vi.hoisted(() => ({ available: false, authorize: vi.fn() }));
const activeServer = vi.hoisted(() => ({ id: 'origin' }));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: () => activeServer.id }));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return registered.servers;
    },
    findServerByUrl: (url: string) =>
      registered.servers.find((server) => new URL(server.url).origin === new URL(url).origin),
    getStore: vi.fn(() => ({ serverInfo: { init: initServerInfoMock } })),
    updateRegistration: updateRegistrationMock,
    replaceServerAuthentication: replaceServerAuthenticationMock,
    clearOriginAuthentication: clearOriginAuthenticationMock
  }
}));

vi.mock('$lib/desktop/nativeAuthorization', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/desktop/nativeAuthorization')>()),
  hasNativeAuthorization: () => native.available,
  authorizeNatively: native.authorize
}));

const {
  clearOriginAuthenticationMock,
  getPublicServerInfoMock,
  initServerInfoMock,
  gotoMock,
  registered,
  replaceServerAuthenticationMock,
  updateRegistrationMock
} = vi.hoisted(() => ({
  clearOriginAuthenticationMock: vi.fn(),
  getPublicServerInfoMock: vi.fn(),
  initServerInfoMock: vi.fn(() => Promise.resolve()),
  gotoMock: vi.fn(() => Promise.resolve()),
  registered: { servers: [] as RegisteredServer[] },
  replaceServerAuthenticationMock: vi.fn(() => true),
  updateRegistrationMock: vi.fn()
}));

vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('$app/paths', () => ({
  resolve: (route: string, params?: { serverId?: string }) =>
    params?.serverId ? `/chat/${params.serverId}` : route
}));
vi.mock('@chatto/client/api/server', () => ({ getPublicServerInfo: getPublicServerInfoMock }));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));

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

/** A sign-in window that records when the flow closes it. */
function fakePopup(): Window {
  return {
    closed: false,
    opener: {} as Window,
    close: vi.fn(function (this: { closed: boolean }) {
      this.closed = true;
    })
  } as unknown as Window;
}

const remote: RegisteredServer = {
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
};

/** A token response with a renewable bearer session. */
function tokenResponse(extra: Record<string, unknown> = {}): Response {
  return new Response(
    JSON.stringify({
      access_token: 'cht_ATtoken',
      refresh_token: 'cht_RT_token',
      expires_in: 900,
      refresh_token_expires_in: 7_776_000,
      ...extra
    }),
    { headers: { 'Content-Type': 'application/json' } }
  );
}

describe('remote server OAuth popup', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
    FakeBroadcastChannel.instances = [];
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel);
    vi.stubGlobal('sessionStorage', memoryStorage());
    vi.stubGlobal('localStorage', memoryStorage());
    registered.servers = [remote];
    activeServer.id = 'origin';
    getPublicServerInfoMock.mockReset();
    getPublicServerInfoMock.mockResolvedValue({
      name: 'Remote',
      authorizeUrl: '/oauth/authorize',
      iconUrl: null
    });
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
    const exchange = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      tokenResponse()
    );
    vi.stubGlobal('fetch', exchange);
    const { startRemoteReauthentication } = await import('./reauth');
    await startRemoteReauthentication(remote);
    expect(open).not.toHaveBeenCalled();
    expect(FakeBroadcastChannel.instances).toHaveLength(0);
    expect(JSON.parse(exchange.mock.calls[0]?.[1]?.body as string)).toMatchObject({
      code: 'native-code',
      client_id: 'eu.chattocorp.chatto.mobile',
      redirect_uri: 'eu.chattocorp.chatto.mobile:/oauth/callback'
    });
    expect(replaceServerAuthenticationMock).toHaveBeenCalledWith(
      'remote',
      expect.objectContaining({ token: 'cht_ATtoken' })
    );
    expect(gotoMock).toHaveBeenCalledWith('/chat/remote');
  });

  it('stays on the route of the server that the user signs in to', async () => {
    native.available = true;
    native.authorize.mockImplementation(async (_raw: string, state: string) => ({
      state,
      code: 'native-code'
    }));
    vi.stubGlobal('window', { open: vi.fn() });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => tokenResponse())
    );
    activeServer.id = 'remote';
    const { startRemoteReauthentication } = await import('./reauth');

    await startRemoteReauthentication(remote);

    expect(replaceServerAuthenticationMock).toHaveBeenCalledOnce();
    expect(gotoMock).not.toHaveBeenCalled();
  });

  it('does not exchange credentials or store a session after native cancellation', async () => {
    native.available = true;
    native.authorize.mockRejectedValue(new Error('Sign-in cancelled'));
    const exchange = vi.fn();
    vi.stubGlobal('fetch', exchange);
    const { startRemoteReauthentication } = await import('./reauth');
    await expect(startRemoteReauthentication(remote)).rejects.toThrow('Sign-in cancelled');
    expect(exchange).not.toHaveBeenCalled();
    expect(replaceServerAuthenticationMock).not.toHaveBeenCalled();
  });

  it('uses the built-in OAuth client identity for Chatto Desktop', async () => {
    const { oauthClientIdForLocation } = await import('./reauth');
    expect(
      oauthClientIdForLocation(
        {
          origin: 'chatto://desktop',
          protocol: 'chatto:',
          host: 'desktop',
          hostname: 'desktop'
        },
        'https://remote.example'
      )
    ).toBe('chatto://desktop');
  });

  it.each([
    'http://localhost:4001',
    'http://chatto.canberra.localhost:4000',
    'http://127.0.0.1:5173',
    'http://[::1]:4000',
    'https://chatto.localhost'
  ])('uses the built-in loopback identity from %s for a remote server', async (origin) => {
    const { oauthClientIdForLocation } = await import('./reauth');
    expect(oauthClientIdForLocation(new URL(origin), 'https://remote.example')).toBe(
      'chatto://loopback'
    );
  });

  it.each(['http://127.0.0.1:4010', 'http://chatto.other.localhost:4000'])(
    'keeps the origin CIMD identity for the local server %s',
    async (serverUrl) => {
      const { oauthClientIdForLocation } = await import('./reauth');
      expect(oauthClientIdForLocation(new URL('http://localhost:4000'), serverUrl)).toBe(
        'http://localhost:4000/oauth/frontend-client-metadata.json'
      );
    }
  );

  it('treats an unparsable server URL as a server that is not local', async () => {
    const { oauthClientIdForLocation } = await import('./reauth');
    expect(oauthClientIdForLocation(new URL('http://localhost:4000'), 'not a url')).toBe(
      'chatto://loopback'
    );
  });

  it('uses the origin CIMD identity on a public origin', async () => {
    const { oauthClientIdForLocation } = await import('./reauth');
    expect(
      oauthClientIdForLocation(new URL('https://chat.example'), 'https://remote.example')
    ).toBe('https://chat.example/oauth/frontend-client-metadata.json');
  });

  it('keeps the main client mounted while completing PKCE through a popup', async () => {
    const popup = fakePopup();
    const { owner, open } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        tokenResponse({ user: { id: 'user-1', login: 'alice', displayName: 'Alice' } })
      )
    );

    const { startRemoteReauthentication } = await import('./reauth');
    const completion = startRemoteReauthentication(remote);

    // window.open happens before the first asynchronous PKCE operation, so it
    // remains associated with the user's click and avoids popup blocking.
    expect(open).toHaveBeenCalledOnce();
    expect(open).toHaveBeenCalledWith(
      expect.stringMatching(/^https:\/\/app\.example\/servers\/authorize#.+/),
      expect.stringMatching(/^chatto-oauth-/),
      expect.stringContaining('width=560,height=760')
    );
    await vi.waitFor(() => expect(authorizationLaunchTarget(open)).toContain('/oauth/authorize?'));
    expect(popup.opener).toBeNull();

    const authorizeURL = new URL(authorizationLaunchTarget(open)!);
    const state = authorizeURL.searchParams.get('state');
    expect(state).toBeTruthy();
    expect(authorizeURL.searchParams.get('redirect_uri')).toBe(
      'https://app.example/servers/callback?mode=popup'
    );
    expect(authorizeURL.searchParams.get('client_id')).toBe(
      'https://app.example/oauth/frontend-client-metadata.json'
    );
    expect(authorizeURL.searchParams.has('provider_id')).toBe(false);

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
    expect(updateRegistrationMock).toHaveBeenCalledWith('remote', {
      name: 'Remote',
      iconUrl: null
    });
    expect(replaceServerAuthenticationMock).toHaveBeenCalledWith(
      'remote',
      expect.objectContaining({ token: 'cht_ATtoken', userId: 'user-1', reauthRequiredAt: null })
    );
    expect(initServerInfoMock).toHaveBeenCalledOnce();
    expect(gotoMock).toHaveBeenCalledWith('/chat/remote');
    expect(popup.close).toHaveBeenCalledOnce();
    expect(localStorage.length).toBe(0);
  });

  it('opens before server discovery completes', async () => {
    const popup = fakePopup();
    const { owner, open } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => tokenResponse())
    );

    let finishDiscovery: ((info: Record<string, unknown>) => void) | undefined;
    getPublicServerInfoMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishDiscovery = resolve;
        })
    );
    const { startRemoteReauthentication } = await import('./reauth');
    const completion = startRemoteReauthentication(remote);

    expect(open).toHaveBeenCalledOnce();
    expect(authorizationLaunchTarget(open)).toBeNull();

    finishDiscovery?.({
      name: 'Discovered Remote',
      authorizeUrl: '/oauth/authorize',
      iconUrl: null
    });

    await vi.waitFor(() => expect(authorizationLaunchTarget(open)).toContain('/oauth/authorize?'));
    const state = new URL(authorizationLaunchTarget(open)!).searchParams.get('state');
    FakeBroadcastChannel.instances
      .find((channel) => channel.name === `chatto:oauth-popup:${state}`)
      ?.emit({ type: 'chatto:oauth-popup-response', state, code: 'cht_ACcode' });

    await completion;
    expect(updateRegistrationMock).toHaveBeenCalledWith(
      'remote',
      expect.objectContaining({ name: 'Discovered Remote' })
    );
  });

  it('closes the popup when server discovery fails', async () => {
    const popup = fakePopup();
    const { owner } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    getPublicServerInfoMock.mockRejectedValueOnce(new Error('discovery failed'));

    const { startRemoteReauthentication } = await import('./reauth');
    await expect(startRemoteReauthentication(remote)).rejects.toThrow('discovery failed');

    expect(popup.close).toHaveBeenCalledOnce();
    expect(sessionStorage.getItem('chatto:oauth:flow')).toBeNull();
  });

  it('fails without navigating the main window when the popup is blocked', async () => {
    const { owner } = browserHarness(null);
    vi.stubGlobal('window', owner);

    const { startRemoteReauthentication } = await import('./reauth');
    await expect(startRemoteReauthentication(remote)).rejects.toThrow('could not be opened');

    expect(gotoMock).not.toHaveBeenCalled();
    expect(sessionStorage.getItem('chatto:oauth:flow')).toBeNull();
  });

  it('handles a sign-in window that closes while server data still loads', async () => {
    const popup = fakePopup();
    const { owner } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    const unhandled = vi.fn();
    process.on('unhandledRejection', unhandled);

    let rejectServerInfo: ((error: Error) => void) | undefined;
    getPublicServerInfoMock.mockImplementationOnce(
      () =>
        new Promise<never>((_resolve, reject) => {
          rejectServerInfo = reject;
        })
    );
    const { startRemoteReauthentication } = await import('./reauth');
    const completion = startRemoteReauthentication(remote);

    (popup as unknown as { closed: boolean }).closed = true;
    await new Promise((resolve) => setTimeout(resolve, 400));
    rejectServerInfo?.(new Error('discovery failed'));
    await expect(completion).rejects.toThrow('discovery failed');
    await new Promise((resolve) => setTimeout(resolve, 0));

    process.off('unhandledRejection', unhandled);
    expect(unhandled).not.toHaveBeenCalled();
  });

  it('reports a blocked sign-in window without an unhandled server-data rejection', async () => {
    const { owner } = browserHarness(null);
    vi.stubGlobal('window', owner);
    const unhandled = vi.fn();
    process.on('unhandledRejection', unhandled);
    getPublicServerInfoMock.mockRejectedValueOnce(new Error('discovery failed'));

    const { startRemoteReauthentication } = await import('./reauth');
    await expect(startRemoteReauthentication(remote)).rejects.toThrow('could not be opened');
    await new Promise((resolve) => setTimeout(resolve, 0));

    process.off('unhandledRejection', unhandled);
    expect(unhandled).not.toHaveBeenCalled();
  });
});

describe('completeServerOAuthFlow', () => {
  const flow = {
    remoteUrl: 'https://remote.example',
    serverName: 'Remote',
    serverIconUrl: null,
    verifier: 'verifier',
    clientId: 'client'
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
    registered.servers = [remote];
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('does not exchange the code for a server that is not registered', async () => {
    registered.servers = [];
    const exchange = vi.fn();
    vi.stubGlobal('fetch', exchange);
    const { completeServerOAuthFlow } = await import('./reauth');

    await expect(completeServerOAuthFlow(flow, 'code', 'https://app.example/cb')).rejects.toThrow(
      'no longer registered'
    );
    expect(exchange).not.toHaveBeenCalled();
  });

  it('stores no session when the server is removed during the exchange', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        registered.servers = [];
        return tokenResponse();
      })
    );
    const { completeServerOAuthFlow } = await import('./reauth');

    await expect(completeServerOAuthFlow(flow, 'code', 'https://app.example/cb')).rejects.toThrow(
      'no longer registered'
    );
    expect(replaceServerAuthenticationMock).not.toHaveBeenCalled();
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
