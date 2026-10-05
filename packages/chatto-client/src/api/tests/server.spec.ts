import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getPublicServerInfo,
  InvalidPublicServerError,
  listNeighborhoodServers
} from '../server.js';
import {
  GetServerResponse,
  ListNeighborhoodServersResponse
} from '@chatto/api-types/chatto/discovery/v1/server_pb';

/**
 * Public discovery uses its own HTTP transport without Chatto's interceptors.
 * Stub `fetch` so the tests see the real request and answer with Connect JSON.
 */
const browserFetch = vi.fn<typeof fetch>();

function respondWith(message: GetServerResponse | ListNeighborhoodServersResponse) {
  browserFetch.mockResolvedValueOnce(
    new Response(message.toJsonString(), { headers: { 'Content-Type': 'application/json' } })
  );
}

/** The URL, options, and headers of one `fetch` call. */
function sentRequest(call = 0) {
  const [url, init] = browserFetch.mock.calls[call] ?? [];
  return { url: String(url), init, headers: new Headers(init?.headers) };
}

describe('public server discovery', () => {
  beforeEach(() => {
    browserFetch.mockReset();
    vi.stubGlobal('fetch', browserFetch);
  });

  afterEach(() => vi.unstubAllGlobals());

  it('loads the cached Neighborhood without authentication', async () => {
    respondWith(
      new ListNeighborhoodServersResponse({
        servers: [
          {
            origin: 'https://one.example',
            profile: {
              name: 'One',
              version: '0.5.0',
              description: 'First',
              logoUrl: '/assets/neighborhood/logo',
              welcomeMessage: 'ignored'
            },
            directNeighbor: true,
            recommendedByOrigins: ['https://two.example']
          },
          { origin: 'https://nameless.example', profile: { name: '' }, recommendedByOrigins: [] },
          { origin: '', profile: { name: 'No origin' }, recommendedByOrigins: [] }
        ]
      })
    );

    await expect(listNeighborhoodServers('https://chat.example.test')).resolves.toEqual([
      {
        origin: 'https://one.example',
        profile: {
          name: 'One',
          version: '0.5.0',
          description: 'First',
          iconUrl: '/assets/neighborhood/logo',
          bannerUrl: null
        },
        directNeighbor: true,
        recommendedByOrigins: ['https://two.example']
      }
    ]);
    const { url, headers } = sentRequest();
    expect(url).toBe(
      'https://chat.example.test/api/connect/chatto.discovery.v1.ServerDiscoveryService/ListNeighborhoodServers'
    );
    expect(headers.get('Connect-Timeout-Ms')).toBe('10000');
    expect(headers.has('Authorization')).toBe(false);
  });

  it('loads public server metadata and maps the shared profile', async () => {
    respondWith(
      new GetServerResponse({
        profile: {
          name: 'Remote Chatto',
          version: '9.8.7',
          logoUrl: 'https://cdn/logo.webp',
          bannerUrl: 'https://cdn/banner.webp',
          welcomeMessage: 'welcome',
          description: 'description'
        },
        login: {
          directRegistrationEnabled: true,
          directLoginEnabled: false,
          authorizeUrl: '/oauth/authorize',
          providers: [
            {
              id: 'hub',
              type: 'oidc',
              label: 'Chatto Hub',
              loginUrl: '/auth/providers/hub',
              issuerUrl: 'https://id.example',
              autoProvision: true
            }
          ]
        }
      })
    );

    const info = await getPublicServerInfo('https://chat.example.test');

    const { url, headers } = sentRequest();
    expect(url).toBe(
      'https://chat.example.test/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer'
    );
    expect(headers.get('Content-Type')).toBe('application/json');
    expect(headers.get('Connect-Timeout-Ms')).toBe('10000');
    expect(info).toEqual({
      setupRequired: false,
      name: 'Remote Chatto',
      version: '9.8.7',
      authorizeUrl: '/oauth/authorize',
      directRegistrationEnabled: true,
      directLoginEnabled: false,
      accountCreationPolicy: 'open',
      welcomeMessage: 'welcome',
      description: 'description',
      iconUrl: 'https://cdn/logo.webp',
      bannerUrl: 'https://cdn/banner.webp',
      authProviders: [
        {
          id: 'hub',
          type: 'oidc',
          label: 'Chatto Hub',
          loginUrl: '/auth/providers/hub',
          issuerUrl: 'https://id.example',
          autoProvision: true
        }
      ]
    });
  });

  it('omits browser credentials, referrers, and redirects from public discovery', async () => {
    respondWith(new GetServerResponse({ profile: { name: 'Chatto', version: '0.5.0' } }));

    await getPublicServerInfo('https://chat.example.test');

    expect(sentRequest().init).toMatchObject({
      credentials: 'omit',
      redirect: 'error',
      referrerPolicy: 'no-referrer'
    });
  });

  it('uses profile defaults when optional public profile fields are absent', async () => {
    respondWith(new GetServerResponse({ profile: { name: 'Chatto', version: '' }, login: {} }));

    await expect(getPublicServerInfo('https://chat.example.test')).resolves.toMatchObject({
      name: 'Chatto',
      directLoginEnabled: true,
      welcomeMessage: null,
      description: null,
      iconUrl: null,
      bannerUrl: null
    });
  });

  it('rejects a response without a public server profile', async () => {
    respondWith(new GetServerResponse());

    await expect(getPublicServerInfo('https://invalid.example')).rejects.toBeInstanceOf(
      InvalidPublicServerError
    );
  });
});
