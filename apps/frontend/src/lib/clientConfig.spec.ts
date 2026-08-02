import { afterEach, describe, expect, it, vi } from 'vitest';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe('client configuration', () => {
  it('loads the Authling selection from the frontend origin', async () => {
    const fetchMock = vi.fn(async () =>
      Response.json({
        version: 1,
        authling: {
          issuer: 'https://id.example/',
          client_id: 'https://client.example/oauth/client-metadata.json'
        }
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    const { getClientConfiguration } = await import('./clientConfig');
    await expect(getClientConfiguration()).resolves.toEqual({
      version: 1,
      authling: {
        issuer: 'https://id.example',
        clientId: 'https://client.example/oauth/client-metadata.json'
      }
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/client-config.json',
      expect.objectContaining({ cache: 'no-store', credentials: 'same-origin' })
    );
  });

  it('supports clients without an Authling selection', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => Response.json({ version: 1 }))
    );
    const { getClientConfiguration } = await import('./clientConfig');
    await expect(getClientConfiguration()).resolves.toEqual({ version: 1, authling: null });
  });

  it.each([
    [{ version: 2 }, 'version is not supported'],
    [{ version: 1, authling: {} }, 'configuration is incomplete'],
    [
      {
        version: 1,
        authling: { issuer: 'http://id.example', client_id: 'https://client.example/meta.json' }
      },
      'issuer URL is invalid'
    ],
    [
      {
        version: 1,
        authling: { issuer: 'https://id.example?tenant=one', client_id: 'client' }
      },
      'issuer URL is invalid'
    ]
  ])('rejects invalid configuration %#', async (document, message) => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => Response.json(document))
    );
    const { getClientConfiguration } = await import('./clientConfig');
    await expect(getClientConfiguration()).rejects.toThrow(message);
  });
});
