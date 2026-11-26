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
    getStore: vi.fn(() => ({ serverInfo: { init: initServerInfoMock } })),
    updateRegistration: updateRegistrationMock,
    replaceServerAuthentication: replaceServerAuthenticationMock,
    clearOriginAuthentication: clearOriginAuthenticationMock
  }
}));

vi.mock('$lib/serverCatalogue', () => ({
  findServerByUrl: (url: string) =>
    registered.servers.find((server) => new URL(server.url).origin === new URL(url).origin)
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
  invalidateAllMock,
  registered,
  replaceServerAuthenticationMock,
  updateRegistrationMock
} = vi.hoisted(() => ({
  clearOriginAuthenticationMock: vi.fn(),
  getPublicServerInfoMock: vi.fn(),
  initServerInfoMock: vi.fn(() => Promise.resolve()),
  gotoMock: vi.fn(() => Promise.resolve()),
  invalidateAllMock: vi.fn(() => Promise.resolve()),
  registered: { servers: [] as RegisteredServer[] },
  replaceServerAuthenticationMock: vi.fn(() => true),
  updateRegistrationMock: vi.fn()
}));

vi.mock('$app/navigation', () => ({ goto: gotoMock, invalidateAll: invalidateAllMock }));
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

  it.each([undefined, 'chatto-id', 'signup'])(
    'completes native PKCE with provider hint %s and persists the mobile session',
    async (providerId) => {
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
        expect(url.searchParams.get('provider_id')).toBe(
          providerId === 'signup' ? null : (providerId ?? null)
        );
        expect(url.searchParams.get('screen_hint')).toBe(providerId === 'signup' ? 'signup' : null);
        return { state, code: 'native-code', iss: 'https://remote.example' };
      });
      const open = vi.fn();
      vi.stubGlobal('window', { open });
      const exchange = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
        tokenResponse()
      );
      vi.stubGlobal('fetch', exchange);
      const { startRemoteReauthentication } = await import('./reauth');
      await startRemoteReauthentication(
        remote,
        providerId === 'signup' ? undefined : providerId,
        providerId === 'signup' ? 'signup' : undefined
      );
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
    }
  );

  it.each(['valid', 'wrong issuer', 'wrong state', 'duplicate code'])(
    'checks the Desktop browser callback before exchanging the code (%s)',
    async (variant) => {
      const redirect = 'chatto://desktop/servers/callback?mode=popup';
      const start = vi.fn(async (origin: string, raw: string) => {
        const url = new URL(raw);
        expect(origin).toBe(remote.url);
        expect(url.origin).toBe(origin);
        expect(url.searchParams.get('redirect_uri')).toBe(redirect);
        expect(url.searchParams.get('provider_id')).toBe('pocket-id');
        expect(url.searchParams.get('code_challenge_method')).toBe('S256');
        const callback = new URL(redirect);
        callback.searchParams.set(
          'state',
          variant === 'wrong state' ? 'wrong' : url.searchParams.get('state')!
        );
        callback.searchParams.set(
          'iss',
          variant === 'wrong issuer' ? 'https://other.example' : origin
        );
        callback.searchParams.set('code', 'desktop-code');
        if (variant === 'duplicate code') callback.searchParams.append('code', 'another');
        return callback.href;
      });
      vi.stubGlobal('window', {
        chattoDesktop: { browserAuthorization: { start, cancel: vi.fn() } }
      });
      const exchange = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
        tokenResponse()
      );
      vi.stubGlobal('fetch', exchange);
      const { startRemoteReauthentication } = await import('./reauth');
      const result = startRemoteReauthentication(remote, 'pocket-id');
      if (variant === 'valid') {
        await result;
        expect(JSON.parse(exchange.mock.calls[0]?.[1]?.body as string)).toMatchObject({
          code: 'desktop-code',
          client_id: 'chatto://desktop',
          redirect_uri: redirect,
          code_verifier: expect.any(String)
        });
        expect(gotoMock).toHaveBeenCalledWith('/chat/remote');
      } else {
        await expect(result).rejects.toThrow();
        expect(exchange).not.toHaveBeenCalled();
      }
    }
  );

  it.each([
    [
      'install-in-applications',
      'Move Chatto Desktop to Applications, open it there and try signing in again.'
    ],
    [
      'callback-registration',
      'Set Chatto Desktop as the app for chatto: links in your system settings, then try signing in again.'
    ],
    [
      'missing-xdg-mime',
      'Install xdg-utils so Desktop can register chatto: links, then try signing in again.'
    ]
  ])('explains how to recover from %s', async (error, message) => {
    vi.stubGlobal('window', {
      chattoDesktop: { browserAuthorization: { start: async () => ({ error }), cancel: vi.fn() } }
    });
    const { startRemoteReauthentication } = await import('./reauth');
    const { popupErrorMessage } = await import('$lib/oauth/popupError');
    const result = startRemoteReauthentication(remote);
    await expect(result).rejects.toMatchObject({ reason: error });
    expect(await result.catch((cause) => popupErrorMessage(cause, 'Failed'))).toBe(message);
  });

  it('cancels a Desktop sign-in while the browser is open and ignores its late callback', async () => {
    const pending = Promise.withResolvers<string>();
    const start = vi.fn((_origin: string, _url: string) => pending.promise);
    const cancel = vi.fn();
    vi.stubGlobal('window', { chattoDesktop: { browserAuthorization: { start, cancel } } });
    const exchange = vi.fn();
    vi.stubGlobal('fetch', exchange);
    const { startRemoteReauthentication, cancelServerOAuthFlow } = await import('./reauth');
    const result = startRemoteReauthentication(remote);
    await vi.waitFor(() => expect(start).toHaveBeenCalledOnce());
    const state = new URL(start.mock.calls[0][1]).searchParams.get('state');
    cancelServerOAuthFlow(remote.url);
    await expect(result).rejects.toMatchObject({ reason: 'cancelled' });
    expect(cancel).toHaveBeenCalledWith(state);
    pending.resolve(
      'chatto://desktop/servers/callback?mode=popup&code=late&state=' +
        state +
        '&iss=' +
        encodeURIComponent(remote.url)
    );
    await pending.promise;
    expect(exchange).not.toHaveBeenCalled();
  });

  it('stays on the route of the server that the user signs in to', async () => {
    native.available = true;
    native.authorize.mockImplementation(async (_raw: string, state: string) => ({
      state,
      code: 'native-code',
      iss: 'https://remote.example'
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
    expect(invalidateAllMock).toHaveBeenCalledOnce();
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

  it.each([undefined, 'chatto-id', 'signup'])(
    'keeps the main client mounted while completing popup PKCE with provider hint %s',
    async (providerId) => {
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
      const completion = startRemoteReauthentication(
        remote,
        providerId === 'signup' ? undefined : providerId,
        providerId === 'signup' ? 'signup' : undefined
      );

      // window.open happens before the first asynchronous PKCE operation, so it
      // remains associated with the user's click and avoids popup blocking.
      expect(open).toHaveBeenCalledOnce();
      expect(open).toHaveBeenCalledWith(
        expect.stringMatching(/^https:\/\/app\.example\/servers\/authorize#.+/),
        expect.stringMatching(/^chatto-oauth-/),
        expect.stringContaining('width=560,height=800')
      );
      await vi.waitFor(() =>
        expect(authorizationLaunchTarget(open)).toContain('/oauth/authorize?')
      );
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
      expect(authorizeURL.searchParams.get('provider_id')).toBe(
        providerId === 'signup' ? null : (providerId ?? null)
      );
      expect(authorizeURL.searchParams.get('screen_hint')).toBe(
        providerId === 'signup' ? 'signup' : null
      );

      const responseChannel = FakeBroadcastChannel.instances.find(
        (channel) => channel.name === `chatto:oauth-popup:${state}`
      );
      expect(responseChannel).toBeDefined();
      responseChannel!.emit({
        type: 'chatto:oauth-popup-response',
        state,
        code: 'cht_ACcode',
        iss: 'https://remote.example'
      });

      await completion;

      const [tokenUrl, tokenRequest] = vi.mocked(fetch).mock.calls[0]!;
      expect(String(tokenUrl)).toBe('https://remote.example/oauth/token');
      // The exchange sends no cookies and follows no redirect.
      expect(tokenRequest).toMatchObject({
        method: 'POST',
        credentials: 'omit',
        redirect: 'error',
        body: expect.stringContaining(
          '"redirect_uri":"https://app.example/servers/callback?mode=popup"'
        )
      });
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
    }
  );

  it('does not exchange a native code that a different server issued', async () => {
    native.available = true;
    native.authorize.mockImplementation(async (_raw: string, state: string) => ({
      state,
      code: 'native-code',
      iss: 'https://other.example'
    }));
    const exchange = vi.fn();
    vi.stubGlobal('fetch', exchange);
    const { startRemoteReauthentication } = await import('./reauth');

    await expect(startRemoteReauthentication(remote)).rejects.toThrow('different server');

    expect(exchange).not.toHaveBeenCalled();
    expect(replaceServerAuthenticationMock).not.toHaveBeenCalled();
  });

  it.each([
    { iss: 'https://other.example', accepted: false },
    { iss: 'not a URL', accepted: false },
    { iss: 'https://remote.example', accepted: true },
    // A response without an issuer can come from a different server.
    { iss: '', accepted: false },
    { iss: undefined, accepted: false }
  ])('exchanges a popup code from issuer $iss: $accepted', async ({ iss, accepted }) => {
    const popup = fakePopup();
    const { owner, open } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    const exchange = vi.fn(async () => tokenResponse());
    vi.stubGlobal('fetch', exchange);
    const { startRemoteReauthentication } = await import('./reauth');
    const completion = startRemoteReauthentication(remote);
    completion.catch(() => {});

    await vi.waitFor(() => expect(authorizationLaunchTarget(open)).toContain('/oauth/authorize?'));
    const state = new URL(authorizationLaunchTarget(open)!).searchParams.get('state');
    FakeBroadcastChannel.instances
      .find((channel) => channel.name === `chatto:oauth-popup:${state}`)
      ?.emit({ type: 'chatto:oauth-popup-response', state, code: 'cht_ACcode', iss });

    if (accepted) {
      await completion;
      expect(replaceServerAuthenticationMock).toHaveBeenCalledOnce();
    } else {
      await expect(completion).rejects.toThrow('different server');
      expect(exchange).not.toHaveBeenCalled();
      expect(replaceServerAuthenticationMock).not.toHaveBeenCalled();
    }
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
      ?.emit({
        type: 'chatto:oauth-popup-response',
        state,
        code: 'cht_ACcode',
        iss: 'https://remote.example'
      });

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
  });

  it('fails without navigating the main window when the popup is blocked', async () => {
    const { owner } = browserHarness(null);
    vi.stubGlobal('window', owner);

    const { startRemoteReauthentication } = await import('./reauth');
    await expect(startRemoteReauthentication(remote)).rejects.toMatchObject({ reason: 'blocked' });

    expect(gotoMock).not.toHaveBeenCalled();
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

    const cancelled = expect(completion).rejects.toMatchObject({ reason: 'cancelled' });
    popup.close();
    await cancelled;
    rejectServerInfo?.(new Error('discovery failed'));
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

  it('keeps waiting when a provider page hides the window, until the user cancels', async () => {
    const popup = fakePopup();
    const { owner } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    const { startRemoteReauthentication, cancelServerOAuthFlow } = await import('./reauth');
    const completion = startRemoteReauthentication(remote);
    let settled = false;
    completion.then(
      () => (settled = true),
      () => (settled = true)
    );
    // A Cross-Origin-Opener-Policy page makes the handle report `closed`.
    (popup as unknown as { closed: boolean }).closed = true;
    await new Promise((resolve) => setTimeout(resolve, 600));
    expect(settled).toBe(false);

    const rejected = expect(completion).rejects.toMatchObject({ reason: 'cancelled' });
    cancelServerOAuthFlow(remote.url);
    await rejected;
    expect(gotoMock).not.toHaveBeenCalled();
  });

  it('without a channel, identifies a closed remote sign-in window as cancellation', async () => {
    vi.stubGlobal('BroadcastChannel', undefined);
    const popup = fakePopup();
    const { owner } = browserHarness(popup);
    vi.stubGlobal('window', owner);
    const { startRemoteReauthentication } = await import('./reauth');
    const completion = startRemoteReauthentication(remote);
    const rejected = expect(completion).rejects.toMatchObject({ reason: 'cancelled' });
    (popup as unknown as { closed: boolean }).closed = true;
    await rejected;
    expect(gotoMock).not.toHaveBeenCalled();
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

  it('fits provider sign-in pages on larger screens', async () => {
    const { authorizationWindowFeatures } = await import('../oauth/authorizationWindow');
    const { owner } = browserHarness(null);
    Object.assign(owner.screen, { availWidth: 1920, availHeight: 1080 });
    expect(authorizationWindowFeatures(owner)).toContain('width=560,height=800');
  });
});
