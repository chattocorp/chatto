import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it, vi } from 'vitest';
import {
  canonicalServerOrigin,
  loadServerDirectory,
  serverOriginFromInput
} from './serverDirectory';
import type { PublicServerInfo } from '$lib/api-client/server';

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
    expect(serverOriginFromInput('dev.preview.chatto.run')).toBe(
      'https://dev.preview.chatto.run'
    );
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
          'https://z.example',
          'HTTPS://A.EXAMPLE:443/',
          'https://invalid.example/path'
        ];
      }
      return ['https://a.example', 'mailto:not-a-server'];
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
    expect(snapshot.entries.at(-1)?.profile).toBeNull();
  });

  it('limits each source to the server-side directory maximum', async () => {
    const advertised = Array.from({ length: 105 }, (_, index) => `https://n${index}.example`);
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
        listNeighbors: vi.fn(async () => ['https://neighbor.example']),
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
});
