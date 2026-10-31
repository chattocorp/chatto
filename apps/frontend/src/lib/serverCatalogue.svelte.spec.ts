import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PublicServerInfo } from '@chatto/client/api/server';
import type { ChattoClient } from '@chatto/client';

const mocks = vi.hoisted(() => ({
  client: null as ChattoClient | null,
  getPublicServerInfo: vi.fn<(url: string) => Promise<PublicServerInfo>>()
}));

vi.mock('@chatto/client/api/server', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@chatto/client/api/server')>()),
  getPublicServerInfo: mocks.getPublicServerInfo
}));

// Keep startup's route and registry together; these unrelated route concerns
// must not start private account requests or load language bundles in this test.
vi.mock('$lib/auth/loadAuth', () => ({ loadCurrentUser: vi.fn().mockResolvedValue(null) }));
vi.mock('$lib/i18n/messages', () => ({ preloadPublicLocaleMessages: vi.fn() }));
vi.mock('$lib/storage/legacySavedViews', () => ({ deleteLegacySavedViews: vi.fn() }));

// A real client like the frontend's, without device storage.
vi.mock('$lib/client', async () => {
  const { createClient } = await import('@chatto/client');
  mocks.client = createClient({ originServer: true });
  return {
    ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
    client: mocks.client,
    serverRegistry: mocks.client.registry
  };
});

import { serverRegistry } from '$lib/client';
import { load } from '../routes/+layout';
import {
  addSignedOutServer,
  findServerByUrl,
  firstAuthenticatedServerId,
  registerOriginServer
} from './serverCatalogue';

const origin = window.location.origin;

function profile(name: string): PublicServerInfo {
  return {
    name,
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
  };
}

function addRemote(id: string, url: string, token: string | null = null): void {
  serverRegistry.addServer({ id, url, name: id, iconUrl: null, addedAt: 1 });
  if (token) {
    serverRegistry.replaceServerAuthentication(id, {
      token,
      refreshToken: null,
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      oauthClientId: null,
      refreshRequestId: null,
      userId: null,
      userLogin: null,
      userDisplayName: null,
      userAvatarUrl: null,
      reauthRequiredAt: null
    });
  }
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  mocks.getPublicServerInfo.mockReset();
  // Store discovery of the added servers must not reach the network.
  mocks.getPublicServerInfo.mockRejectedValue(new TypeError('offline'));
});

afterEach(() => {
  serverRegistry.removeAll();
  vi.restoreAllMocks();
});

describe('registerOriginServer', () => {
  it('registers the origin from public data that the caller loaded', async () => {
    await registerOriginServer({ serverInfo: profile('Origin') });

    expect(serverRegistry.originServer).toMatchObject({ url: origin, name: 'Origin', token: null });
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
    const info = serverRegistry.getStore(serverRegistry.originServer!.id).serverInfo;
    expect(info.name).toBe('Origin');
    expect(info.loading).toBe(false);
    expect(info.error).toBeNull();

    await registerOriginServer({ serverInfo: profile('Other') });
    expect(serverRegistry.servers).toHaveLength(1);
  });

  it('registers a signed-in origin without asking for its public data', async () => {
    await registerOriginServer({ signedIn: true });

    // The store's discovery loads the real name later.
    expect(serverRegistry.originServer).toMatchObject({ url: origin, name: 'Chatto' });
  });

  it('asks the origin once for concurrent callers and registers a Chatto server', async () => {
    let respond: (info: PublicServerInfo) => void = () => {};
    mocks.getPublicServerInfo.mockReturnValueOnce(new Promise((resolve) => (respond = resolve)));

    const registrations = Promise.all([registerOriginServer(), registerOriginServer()]);
    expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();
    expect(mocks.getPublicServerInfo).toHaveBeenCalledWith(origin);
    respond(profile('Discovered'));
    await registrations;

    expect(serverRegistry.originServer?.name).toBe('Discovered');
    expect(serverRegistry.servers).toHaveLength(1);
    expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();
  });

  it('settles the unauthenticated origin after it registers it from public data', async () => {
    const settle = vi.spyOn(serverRegistry, 'settleOriginUnauthenticated');

    await registerOriginServer({ serverInfo: profile('Origin') });

    expect(settle).toHaveBeenCalledOnce();
  });

  it('keeps an origin that another caller registered while discovery ran', async () => {
    let respond: (info: PublicServerInfo) => void = () => {};
    mocks.getPublicServerInfo.mockReturnValueOnce(new Promise((resolve) => (respond = resolve)));

    const discovery = registerOriginServer();
    await registerOriginServer({ signedIn: true });
    respond(profile('Discovered'));
    await discovery;

    expect(serverRegistry.servers).toHaveLength(1);
    expect(serverRegistry.originServer?.name).toBe('Chatto');
  });

  it('registers nothing for a page origin without a Chatto backend', async () => {
    await registerOriginServer({ signedIn: true, protocol: 'chatto:' });

    expect(serverRegistry.servers).toHaveLength(0);
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
  });

  it('registers nothing when the origin is not a Chatto server', async () => {
    mocks.getPublicServerInfo.mockRejectedValueOnce(new Error('not Chatto'));

    await registerOriginServer();

    expect(serverRegistry.servers).toHaveLength(0);
  });
});

describe('root route and registry discovery', () => {
  it('reuses fresh origin discovery when registration creates the store', async () => {
    const info = profile('New origin');
    mocks.getPublicServerInfo.mockResolvedValue(info);

    const result = await load({ url: new URL('/chat/-', origin) } as never);

    expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();
    expect(result).toMatchObject({ serverInfo: info, user: null });
    expect(serverRegistry.getStore(serverRegistry.originServer!.id).serverInfo.name).toBe(
      info.name
    );
  });

  it('awaits the registry request for a known origin instead of starting another', async () => {
    let respond!: (info: PublicServerInfo) => void;
    mocks.getPublicServerInfo.mockReturnValue(new Promise((resolve) => (respond = resolve)));
    await registerOriginServer({ signedIn: true });
    const pending = load({ url: new URL('/chat/-', origin) } as never);
    expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();

    const info = profile('Known origin');
    respond(info);
    expect(await pending).toMatchObject({ serverInfo: info });
    expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();
  });

  it('keeps the fallback retry after fresh origin discovery fails', async () => {
    mocks.getPublicServerInfo
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(profile('Recovered'));

    await load({ url: new URL('/chat/-', origin) } as never);

    expect(mocks.getPublicServerInfo).toHaveBeenCalledTimes(2);
    expect(serverRegistry.originServer?.name).toBe('Recovered');
    const info = serverRegistry.getStore(serverRegistry.originServer!.id).serverInfo;
    expect(info.error).toBeNull();
    expect(info.loading).toBe(false);
  });
});

describe('findServerByUrl', () => {
  it('matches registered servers by canonical origin', () => {
    addRemote('remote', 'https://Remote.example.com');

    expect(findServerByUrl('https://remote.example.com:443/')?.id).toBe('remote');
    expect(findServerByUrl('https://remote.example.com:8443')).toBeUndefined();
    expect(findServerByUrl('not a url')).toBeUndefined();
  });
});

describe('addSignedOutServer', () => {
  it('registers a remote server without a session', () => {
    const id = addSignedOutServer('https://remote.example.com', {
      name: 'Remote',
      iconUrl: 'https://remote.example.com/icon.png'
    });

    expect(serverRegistry.getServer(id)).toMatchObject({
      url: 'https://remote.example.com',
      name: 'Remote',
      iconUrl: 'https://remote.example.com/icon.png',
      token: null,
      reauthRequiredAt: null
    });
    expect(serverRegistry.tryGetStore(id)).toBeDefined();
    expect(serverRegistry.isAuthenticated(id)).toBe(false);
  });

  it('returns a registered server with the same URL unchanged', () => {
    addRemote('existing', 'https://Remote.example.com', 'kept');

    const id = addSignedOutServer('https://remote.example.com', { name: 'Other' });

    expect(id).toBe('existing');
    expect(serverRegistry.servers).toHaveLength(1);
    expect(serverRegistry.getServer('existing')?.token).toBe('kept');
  });
});

describe('firstAuthenticatedServerId', () => {
  it('chooses the first signed-in server and can exclude one', () => {
    addRemote('first', 'https://first.example.com');
    addRemote('second', 'https://second.example.com', 'second-token');
    addRemote('third', 'https://third.example.com', 'third-token');

    expect(firstAuthenticatedServerId()).toBe('second');
    expect(firstAuthenticatedServerId('second')).toBe('third');
    expect(firstAuthenticatedServerId('third')).toBe('second');
  });

  it('prefers a signed-in origin', async () => {
    addRemote('remote', 'https://remote.example.com', 'remote-token');
    await registerOriginServer({ signedIn: true });
    const originId = serverRegistry.originServer!.id;
    serverRegistry.getStore(originId).currentUser.accept({
      id: 'origin-user',
      login: 'origin',
      displayName: 'Origin',
      hasVerifiedEmail: false,
      hasPassword: true,
      viewerCanDeleteAccount: true
    } as never);

    expect(firstAuthenticatedServerId()).toBe(originId);
    expect(firstAuthenticatedServerId(originId)).toBe('remote');
  });
});
