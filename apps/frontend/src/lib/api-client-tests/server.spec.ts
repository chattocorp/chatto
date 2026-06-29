import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getPublicServerInfo } from '@chatto/api-client/server';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getServer: vi.fn()
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

describe('getPublicServerInfo', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.getServer.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({ getServer: mocks.getServer });
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
        authorizeUrl: '/oauth/authorize',
        providers: [
          {
            id: 'hub',
            type: 'oidc',
            label: 'Chatto Hub',
            loginUrl: '/auth/providers/hub'
          }
        ]
      }
    });

    const info = await getPublicServerInfo('https://chat.example.test');

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect'
    });
    expect(mocks.getServer).toHaveBeenCalledWith({}, { signal: undefined });
    expect(info).toEqual({
      name: 'Remote Chatto',
      version: '9.8.7',
      authorizeUrl: '/oauth/authorize',
      directRegistrationEnabled: true,
      welcomeMessage: 'welcome',
      description: 'description',
      iconUrl: 'https://cdn/logo.webp',
      bannerUrl: 'https://cdn/banner.webp',
      authProviders: [
        {
          id: 'hub',
          type: 'oidc',
          label: 'Chatto Hub',
          loginUrl: '/auth/providers/hub'
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
      welcomeMessage: null,
      description: null,
      iconUrl: null,
      bannerUrl: null
    });
  });
});
