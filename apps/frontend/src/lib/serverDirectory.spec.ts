import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it, vi } from 'vitest';
import type { NeighborhoodServer } from '$lib/api-client/server';
import {
  canonicalServerOrigin,
  loadServerDirectory,
  serverOriginFromInput
} from '$lib/serverDirectory';

function neighbor(origin: string, overrides: Partial<NeighborhoodServer> = {}): NeighborhoodServer {
  return {
    origin,
    profile: {
      name: origin,
      version: '0.5.0',
      description: null,
      iconUrl: null,
      bannerUrl: null
    },
    directNeighbor: true,
    recommendedByOrigins: [],
    ...overrides
  };
}

describe('canonicalServerOrigin', () => {
  it('normalizes valid HTTP origins and rejects other schemes or credentials', () => {
    expect(canonicalServerOrigin('HTTPS://Example.COM:443/path')).toBe('https://example.com');
    expect(canonicalServerOrigin('ftp://example.com')).toBeNull();
    expect(canonicalServerOrigin('https://user@example.com')).toBeNull();
  });
});

describe('serverOriginFromInput', () => {
  it('accepts a hostname or full server URL and keeps only its origin', () => {
    expect(serverOriginFromInput('dev.preview.chatto.run')).toBe('https://dev.preview.chatto.run');
    expect(serverOriginFromInput('https://dev.preview.chatto.run/chat/-/RMch1OYtMwZ7sOJ')).toBe(
      'https://dev.preview.chatto.run'
    );
  });

  it('rejects credentials and unsupported schemes', () => {
    expect(serverOriginFromInput('https://user@example.com/path')).toBeNull();
    expect(serverOriginFromInput('ftp://example.com/path')).toBeNull();
  });
});

describe('loadServerDirectory', () => {
  it('asks only registered servers and merges their Neighborhoods', async () => {
    const listNeighborhood = vi.fn(async (source: string) => {
      if (source === 'https://home.example') {
        return [
          neighbor('https://a.example'),
          neighbor('https://b.example', {
            directNeighbor: false,
            recommendedByOrigins: ['https://a.example']
          })
        ];
      }
      return [neighbor('https://A.example/'), neighbor('https://c.example')];
    });

    const directory = await loadServerDirectory(
      ['https://home.example/chat', 'https://other.example', 'https://home.example'],
      { listNeighborhood }
    );

    expect(listNeighborhood.mock.calls.map(([source]) => source)).toEqual([
      'https://home.example',
      'https://other.example'
    ]);
    expect(directory).toEqual({
      sourceCount: 2,
      failedSourceCount: 0,
      entries: [
        {
          origin: 'https://a.example',
          profile: neighbor('https://a.example').profile,
          imageOrigin: 'https://home.example',
          sourceOrigins: ['https://home.example', 'https://other.example']
        },
        {
          origin: 'https://b.example',
          profile: neighbor('https://b.example').profile,
          imageOrigin: 'https://home.example',
          sourceOrigins: ['https://a.example']
        },
        {
          origin: 'https://c.example',
          profile: neighbor('https://c.example').profile,
          imageOrigin: 'https://other.example',
          sourceOrigins: ['https://other.example']
        }
      ]
    });
  });

  it('counts failed servers and treats older servers as empty', async () => {
    const listNeighborhood = vi.fn(async (source: string) => {
      if (source === 'https://old.example') {
        throw new ConnectError('missing', Code.Unimplemented);
      }
      if (source === 'https://down.example') throw new TypeError('offline');
      return [neighbor('https://a.example')];
    });

    const directory = await loadServerDirectory(
      ['https://old.example', 'https://down.example', 'https://home.example'],
      { listNeighborhood }
    );

    expect(directory.sourceCount).toBe(3);
    expect(directory.failedSourceCount).toBe(1);
    expect(directory.entries.map((entry) => entry.origin)).toEqual(['https://a.example']);
  });

  it('drops invalid origins, self entries, and entries without a recommender', async () => {
    const directory = await loadServerDirectory(['https://home.example'], {
      listNeighborhood: async () => [
        neighbor('ftp://invalid.example'),
        neighbor('https://home.example'),
        neighbor('https://orphan.example', { directNeighbor: false })
      ]
    });

    expect(directory.entries).toEqual([]);
  });

  it('rejects when the caller aborts', async () => {
    const controller = new AbortController();
    const load = loadServerDirectory(['https://home.example'], {
      signal: controller.signal,
      listNeighborhood: (_source, options) =>
        new Promise((_resolve, reject) =>
          options?.signal?.addEventListener('abort', () => reject(options.signal?.reason))
        )
    });
    controller.abort();

    await expect(load).rejects.toBeDefined();
  });
});
