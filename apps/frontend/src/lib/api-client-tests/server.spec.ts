import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getPublicNeighbors,
  getPublicServerInfo,
  InvalidPublicServerError
} from '$lib/api-client/server';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getServer: vi.fn(),
  listNeighbors: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('public server discovery', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.getServer.mockReset();
    mocks.listNeighbors.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getServer: mocks.getServer,
      listNeighbors: mocks.listNeighbors
    });
  });

  it('loads structured public Neighbors without authentication', async () => {
    const signal = AbortSignal.timeout(1000);
    mocks.listNeighbors.mockResolvedValue({
      origins: ['https://one.example', 'https://two.example'],
      neighbors: [
        { origin: 'https://one.example', testimonial: 'A kind place.' },
        { origin: 'https://two.example' }
      ]
    });

    await expect(getPublicNeighbors('https://chat.example.test', { signal })).resolves.toEqual([
      { origin: 'https://one.example', testimonial: 'A kind place.' },
      { origin: 'https://two.example', testimonial: null }
    ]);
    expect(mocks.listNeighbors).toHaveBeenCalledWith({}, { signal });
  });

  it('falls back to origin-only Neighbor responses from older servers', async () => {
    mocks.listNeighbors.mockResolvedValue({
      origins: ['https://one.example', 'https://two.example'],
      neighbors: []
    });

    await expect(getPublicNeighbors('https://chat.example.test')).resolves.toEqual([
      { origin: 'https://one.example', testimonial: null },
      { origin: 'https://two.example', testimonial: null }
    ]);
  });

  it('loads public server metadata and maps the shared profile', async () => {
    mocks.getServer.mockResolvedValue({
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
    });

    const info = await getPublicServerInfo('https://chat.example.test');

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect',
      useBinaryFormat: false
    });
    expect(mocks.getServer).toHaveBeenCalledWith({}, { signal: undefined });
    expect(info).toEqual({
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

  it('uses profile defaults when optional public profile fields are absent', async () => {
    mocks.getServer.mockResolvedValue({
      profile: {
        name: 'Chatto',
        version: ''
      },
      login: {}
    });

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
    mocks.getServer.mockResolvedValue({});

    await expect(getPublicServerInfo('https://invalid.example')).rejects.toBeInstanceOf(
      InvalidPublicServerError
    );
  });
});
