import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it, vi } from 'vitest';
import {
  canonicalServerOrigin,
  loadServerDirectory,
  serverOriginFromInput
} from './serverDirectory';
import type { PublicServerInfo } from '$lib/api-client/server';

function recommendation(origin: string, testimonial: string | null = null) {
  return { origin, testimonial };
}

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
  it('keeps first-seen order, deduplicates origins, and tolerates failed sources and profiles', async () => {
    const listNeighbors = vi.fn(async (origin: string) => {
      if (origin === 'https://broken.example') throw new Error('offline');
      if (origin === 'https://one.example') {
        return [
          recommendation('https://z.example', 'A quiet place for careful conversations.'),
          recommendation('HTTPS://A.EXAMPLE:443/', 'A lively maker community.'),
          recommendation('https://invalid.example/path')
        ];
      }
      return [
        recommendation('https://a.example', 'A second endorsement.'),
        recommendation('mailto:not-a-server')
      ];
    });
    const getServerInfo = vi.fn(async (origin: string) => {
      if (origin === 'https://invalid.example') throw new Error('not Chatto');
      return profile(origin);
    });

    const snapshot = await loadServerDirectory(
      ['https://one.example', 'https://broken.example', 'https://two.example'],
      { listNeighbors, getServerInfo }
    );

    expect(snapshot).toMatchObject({ failedSourceCount: 1, sourceCount: 3 });
    expect(snapshot.entries.map((entry) => entry.origin)).toEqual([
      'https://z.example',
      'https://a.example',
      'https://invalid.example'
    ]);
    expect(snapshot.entries.map((entry) => entry.sourceOrigins)).toEqual([
      ['https://one.example'],
      ['https://one.example', 'https://two.example'],
      ['https://one.example']
    ]);
    expect(snapshot.entries[1]?.recommendations).toEqual([
      {
        sourceOrigin: 'https://one.example',
        testimonial: 'A lively maker community.'
      },
      { sourceOrigin: 'https://two.example', testimonial: 'A second endorsement.' }
    ]);
    expect(snapshot.entries.at(-1)?.profile).toBeNull();
  });

  it('deduplicates repeated recommendations from the same source', async () => {
    const snapshot = await loadServerDirectory(['https://source.example'], {
      listNeighbors: vi.fn(async () => [
        recommendation('https://neighbor.example', 'First testimonial'),
        recommendation('HTTPS://NEIGHBOR.EXAMPLE:443/path', 'Ignored duplicate')
      ]),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });

    expect(snapshot.entries).toEqual([
      {
        origin: 'https://neighbor.example',
        profile: profile('https://neighbor.example'),
        sourceOrigins: ['https://source.example'],
        recommendations: [
          { sourceOrigin: 'https://source.example', testimonial: 'First testimonial' }
        ]
      }
    ]);
  });

  it('normalizes and bounds untrusted testimonial text', async () => {
    const oversized = `  ${'界'.repeat(505)}  `;
    const snapshot = await loadServerDirectory(['https://source.example'], {
      listNeighbors: vi.fn(async () => [
        recommendation('https://bounded.example', oversized),
        recommendation('https://empty.example', '   ')
      ]),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });

    expect(snapshot.entries[0]?.recommendations[0]?.testimonial).toBe('界'.repeat(500));
    expect(snapshot.entries[1]?.recommendations[0]?.testimonial).toBeNull();
  });

  it('canonicalizes and deduplicates registered source origins', async () => {
    const listNeighbors = vi.fn(async () => [recommendation('https://neighbor.example')]);

    const snapshot = await loadServerDirectory(
      ['HTTPS://SOURCE.EXAMPLE:443/path', 'https://source.example', 'not a URL'],
      {
        listNeighbors,
        getServerInfo: vi.fn(async (origin: string) => profile(origin))
      }
    );

    expect(listNeighbors).toHaveBeenCalledOnce();
    expect(listNeighbors).toHaveBeenCalledWith(
      'https://source.example',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(snapshot.sourceCount).toBe(1);
    expect(snapshot.entries[0]?.sourceOrigins).toEqual(['https://source.example']);
  });

  it('limits each source to the server-side directory maximum', async () => {
    const advertised = Array.from({ length: 105 }, (_, index) =>
      recommendation(`https://n${index}.example`)
    );
    const getServerInfo = vi.fn(async (origin: string) => profile(origin));

    const snapshot = await loadServerDirectory(['https://source.example'], {
      listNeighbors: vi.fn(async () => advertised),
      getServerInfo
    });

    expect(snapshot.entries).toHaveLength(100);
    expect(getServerInfo).toHaveBeenCalledTimes(100);
  });

  it('does not turn query cancellation into a cached partial-failure result', async () => {
    const controller = new AbortController();
    controller.abort(new DOMException('cancelled', 'AbortError'));

    await expect(
      loadServerDirectory(['https://source.example'], {
        signal: controller.signal,
        listNeighbors: vi.fn(async () => [recommendation('https://neighbor.example')]),
        getServerInfo: vi.fn(async () => profile('Neighbor'))
      })
    ).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('treats an older server without ListNeighbors as an empty source', async () => {
    const snapshot = await loadServerDirectory(['https://old.example'], {
      listNeighbors: vi.fn(async () => {
        throw new ConnectError('not implemented', Code.Unimplemented);
      }),
      getServerInfo: vi.fn(async () => profile('unused'))
    });

    expect(snapshot).toEqual({ entries: [], failedSourceCount: 0, sourceCount: 1 });
  });

  it('loads at most six source directories concurrently', async () => {
    let active = 0;
    let maximumActive = 0;
    let releaseRequests!: () => void;
    const requestGate = new Promise<void>((resolve) => {
      releaseRequests = resolve;
    });
    const listNeighbors = vi.fn(async () => {
      active += 1;
      maximumActive = Math.max(maximumActive, active);
      await requestGate;
      active -= 1;
      return [];
    });

    const loading = loadServerDirectory(
      Array.from({ length: 12 }, (_, index) => `https://source-${index}.example`),
      { listNeighbors, getServerInfo: vi.fn(async (origin: string) => profile(origin)) }
    );

    await vi.waitFor(() => expect(listNeighbors).toHaveBeenCalledTimes(6));
    expect(maximumActive).toBe(6);
    releaseRequests();
    await loading;
    expect(listNeighbors).toHaveBeenCalledTimes(12);
    expect(maximumActive).toBe(6);
  });
});
