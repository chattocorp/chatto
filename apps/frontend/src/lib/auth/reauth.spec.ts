import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const {
  addServerMock,
  clearOriginAuthenticationMock,
  generateServerIdMock,
  initServerInfoMock,
  gotoMock,
  replaceServerAuthenticationMock,
  updateServerMock
} = vi.hoisted(() => ({
  addServerMock: vi.fn(),
  clearOriginAuthenticationMock: vi.fn(),
  generateServerIdMock: vi.fn(() => 'remote-example'),
  initServerInfoMock: vi.fn(() => Promise.resolve()),
  gotoMock: vi.fn(() => Promise.resolve()),
  replaceServerAuthenticationMock: vi.fn(),
  updateServerMock: vi.fn(),
  clearCachedUserMock: vi.fn()
}));

vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('$app/paths', () => ({
  resolve: (_route: string, params?: { serverId?: string }) =>
    params?.serverId ? `/chat/${params.serverId}` : '/login'
}));
vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo: vi.fn() }));
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
vi.mock('./loadAuth', () => ({ clearCachedUser: vi.fn() }));

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
  });

  afterEach(() => {
    vi.unstubAllGlobals();
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
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            access_token: 'cht_ATtoken',
            user: { id: 'user-1', login: 'alice', displayName: 'Alice' }
          }),
          { headers: { 'Content-Type': 'application/json' } }
        )
      )
    );

    const { startServerOAuthFlow } = await import('./reauth');
    const beforeNavigate = vi.fn();
    const completion = startServerOAuthFlow('https://remote.example', {
      name: 'Remote',
      authorizeUrl: '/oauth/authorize',
      iconUrl: null
    }, beforeNavigate);

    // window.open happens before the first asynchronous PKCE operation, so it
    // remains associated with the user's click and avoids popup blocking.
    expect(open).toHaveBeenCalledOnce();
    expect(open).toHaveBeenCalledWith(
      'about:blank',
      expect.stringMatching(/^chatto-oauth-/),
      expect.stringContaining('width=520,height=600')
    );
    await vi.waitFor(() => expect(popup.location.href).toContain('/oauth/authorize?'));
    expect(popup.opener).toBeNull();

    const authorizeURL = new URL(popup.location.href);
    const state = authorizeURL.searchParams.get('state');
    expect(state).toBeTruthy();
    expect(authorizeURL.searchParams.get('redirect_uri')).toBe(
      'https://app.example/servers/callback?mode=popup'
    );

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
        body: expect.stringContaining('"redirect_uri":"https://app.example/servers/callback?mode=popup"')
      })
    );
    expect(addServerMock).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'remote-example',
        url: 'https://remote.example',
        token: 'cht_ATtoken',
        userId: 'user-1'
      })
    );
    expect(initServerInfoMock).toHaveBeenCalledOnce();
    expect(beforeNavigate).toHaveBeenCalledOnce();
    expect(beforeNavigate.mock.invocationCallOrder[0]).toBeLessThan(
      gotoMock.mock.invocationCallOrder[0]!
    );
    expect(gotoMock).toHaveBeenCalledWith('/chat/remote-example');
    expect(popup.close).toHaveBeenCalledOnce();
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
