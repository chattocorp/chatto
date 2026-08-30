import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it, vi } from 'vitest';
import {
  canonicalServerOrigin,
  createServerDirectoryDiscovery,
  SERVER_DIRECTORY_LIMITS,
  serverOriginFromInput,
  type ServerDirectoryDiscovery,
  type ServerDirectorySnapshot
} from './serverDirectory';
import type { PublicNeighbor, PublicServerInfo } from '$lib/api-client/server';

function recommendation(origin: string, testimonial: string | null = null): PublicNeighbor {
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

function observe(discovery: ServerDirectoryDiscovery): {
  snapshots: ServerDirectorySnapshot[];
  latest: () => ServerDirectorySnapshot;
} {
  const snapshots: ServerDirectorySnapshot[] = [];
  discovery.subscribe((snapshot) => snapshots.push(snapshot));
  return { snapshots, latest: () => snapshots.at(-1)! };
}

async function startAndSettle(discovery: ServerDirectoryDiscovery): Promise<void> {
  discovery.start();
  await discovery.whenIdle();
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

describe('ServerDirectoryDiscovery', () => {
  it('requires a mutual recommendation and preserves bounded testimonial text', async () => {
    const oversized = `  ${'界'.repeat(505)}  `;
    const listNeighbors = vi.fn(async (origin: string) => {
      if (origin === 'https://source.example') {
        return [
          recommendation('HTTPS://NEIGHBOR.EXAMPLE:443/path', oversized),
          recommendation('https://neighbor.example', 'ignored duplicate')
        ];
      }
      return [recommendation('https://source.example', 'The return recommendation.')];
    });
    const discovery = createServerDirectoryDiscovery(['HTTPS://SOURCE.EXAMPLE:443/path'], {
      listNeighbors,
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(listNeighbors).toHaveBeenCalledTimes(2);
    const neighbor = observed
      .latest()
      .entries.find((entry) => entry.origin === 'https://neighbor.example');
    expect(neighbor).toMatchObject({
      sourceOrigins: ['https://source.example'],
      recommendations: [{ sourceOrigin: 'https://source.example', testimonial: '界'.repeat(500) }]
    });
  });

  it('shows a direct root recommendation even when it is not mutual', async () => {
    const discovery = createServerDirectoryDiscovery(['https://source.example'], {
      listNeighbors: vi.fn(async (origin: string) =>
        origin === 'https://source.example' ? [recommendation('https://unilateral.example')] : []
      ),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(observed.latest().entries.map(({ origin }) => origin)).toEqual([
      'https://unilateral.example'
    ]);
    expect(observed.latest()).toMatchObject({
      failedSourceCount: 0,
      failedCandidateCount: 0,
      nonMutualCandidateCount: 1
    });
  });

  it('does not let two direct but non-mutual candidates promote each other', async () => {
    const directories: Record<string, PublicNeighbor[]> = {
      'https://a.example': [recommendation('https://b.example')],
      'https://c.example': [recommendation('https://x.example')],
      'https://b.example': [recommendation('https://x.example')],
      'https://x.example': [recommendation('https://b.example')]
    };
    const discovery = createServerDirectoryDiscovery(['https://a.example', 'https://c.example'], {
      listNeighbors: vi.fn(async (origin: string) => directories[origin] ?? []),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(observed.latest().entries.map(({ origin }) => origin)).toEqual([
      'https://b.example',
      'https://x.example'
    ]);
    expect(observed.latest().entries.map(({ sourceOrigins }) => sourceOrigins)).toEqual([
      ['https://a.example'],
      ['https://c.example']
    ]);
    expect(observed.latest().profileRequestCount).toBe(2);
  });

  it('hides a unilateral recommendation from a recursive source', async () => {
    const directories: Record<string, PublicNeighbor[]> = {
      'https://a.example': [recommendation('https://b.example')],
      'https://b.example': [
        recommendation('https://a.example'),
        recommendation('https://c.example')
      ],
      'https://c.example': []
    };
    const discovery = createServerDirectoryDiscovery(['https://a.example'], {
      listNeighbors: vi.fn(async (origin: string) => directories[origin] ?? []),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(observed.latest().entries.map(({ origin }) => origin)).toEqual([
      'https://b.example',
      'https://a.example'
    ]);
    expect(observed.latest().nonMutualCandidateCount).toBe(1);
  });

  it('discovers two mutual hops and does not expand the second hop', async () => {
    const directories: Record<string, PublicNeighbor[]> = {
      'https://a.example': [recommendation('https://b.example')],
      'https://b.example': [
        recommendation('https://a.example'),
        recommendation('https://c.example')
      ],
      'https://c.example': [
        recommendation('https://b.example'),
        recommendation('https://too-far.example')
      ]
    };
    const listNeighbors = vi.fn(async (origin: string) => directories[origin] ?? []);
    const discovery = createServerDirectoryDiscovery(['https://a.example'], {
      listNeighbors,
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(observed.latest().entries.map(({ origin }) => origin)).toEqual([
      'https://b.example',
      'https://a.example',
      'https://c.example'
    ]);
    expect(listNeighbors).not.toHaveBeenCalledWith('https://too-far.example', expect.anything());
  });

  it('deduplicates cycles and canonical aliases before requests', async () => {
    const listNeighbors = vi.fn(async (origin: string) =>
      origin === 'https://a.example'
        ? [recommendation('https://b.example'), recommendation('HTTPS://B.EXAMPLE:443/path')]
        : [recommendation('https://a.example')]
    );
    const getServerInfo = vi.fn(async (origin: string) => profile(origin));
    const discovery = createServerDirectoryDiscovery(
      ['HTTPS://A.EXAMPLE:443/path', 'https://a.example', 'not a URL'],
      { listNeighbors, getServerInfo }
    );
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(listNeighbors.mock.calls.map(([origin]) => origin).sort()).toEqual([
      'https://a.example',
      'https://b.example'
    ]);
    expect(getServerInfo).toHaveBeenCalledTimes(2);
    expect(observed.latest().entries).toHaveLength(2);
    expect(observed.latest().sourceCount).toBe(1);
  });

  it('publishes a completed branch while another root is still slow', async () => {
    let releaseSlow!: () => void;
    const slow = new Promise<PublicNeighbor[]>((resolve) => {
      releaseSlow = () => resolve([]);
    });
    const listNeighbors = vi.fn(async (origin: string) => {
      if (origin === 'https://slow.example') return slow;
      if (origin === 'https://a.example') return [recommendation('https://b.example')];
      return [recommendation('https://a.example')];
    });
    const discovery = createServerDirectoryDiscovery(
      ['https://a.example', 'https://slow.example'],
      { listNeighbors, getServerInfo: vi.fn(async (origin: string) => profile(origin)) }
    );
    const observed = observe(discovery);
    discovery.start();

    await vi.waitFor(() =>
      expect(observed.latest().entries.some(({ origin }) => origin === 'https://b.example')).toBe(
        true
      )
    );
    expect(observed.latest().isLoading).toBe(true);
    releaseSlow();
    await discovery.whenIdle();
  });

  it('enriches an existing entry without moving it', async () => {
    let releaseSecond!: () => void;
    const second = new Promise<PublicNeighbor[]>((resolve) => {
      releaseSecond = () => resolve([recommendation('https://target.example')]);
    });
    const directories: Record<string, PublicNeighbor[]> = {
      'https://one.example': [recommendation('https://target.example', 'First')],
      'https://target.example': [
        recommendation('https://one.example'),
        recommendation('https://two.example')
      ]
    };
    const listNeighbors = vi.fn(async (origin: string) => {
      if (origin === 'https://two.example') return second;
      return directories[origin] ?? [];
    });
    const discovery = createServerDirectoryDiscovery(
      ['https://one.example', 'https://two.example'],
      { listNeighbors, getServerInfo: vi.fn(async (origin: string) => profile(origin)) }
    );
    const observed = observe(discovery);
    discovery.start();

    await vi.waitFor(() => {
      const target = observed
        .latest()
        .entries.find((entry) => entry.origin === 'https://target.example');
      expect(target?.sourceOrigins).toEqual(['https://one.example']);
    });
    const position = observed
      .latest()
      .entries.findIndex((entry) => entry.origin === 'https://target.example');

    releaseSecond();
    await discovery.whenIdle();

    const target = observed
      .latest()
      .entries.find((entry) => entry.origin === 'https://target.example');
    expect(target?.sourceOrigins).toEqual(['https://one.example', 'https://two.example']);
    expect(observed.latest().entries[position]?.origin).toBe('https://target.example');
  });

  it('keeps failed profiles hidden and distinguishes root and candidate failures', async () => {
    const discovery = createServerDirectoryDiscovery(
      ['https://source.example', 'https://broken-root.example'],
      {
        listNeighbors: vi.fn(async (origin: string) => {
          if (origin === 'https://broken-root.example') throw new Error('offline');
          if (origin === 'https://source.example') {
            return [
              recommendation('https://hidden.example'),
              recommendation('https://broken-candidate.example')
            ];
          }
          if (origin === 'https://broken-candidate.example') throw new Error('offline');
          return [recommendation('https://source.example')];
        }),
        getServerInfo: vi.fn(async (origin: string) => {
          if (origin === 'https://hidden.example') throw new Error('invalid profile');
          return profile(origin);
        })
      }
    );
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(
      observed.latest().entries.some(({ origin }) => origin === 'https://hidden.example')
    ).toBe(false);
    expect(observed.latest()).toMatchObject({ failedSourceCount: 1, failedCandidateCount: 1 });
  });

  it('treats Unimplemented as an empty directory without a failure', async () => {
    const discovery = createServerDirectoryDiscovery(['https://old.example'], {
      listNeighbors: vi.fn(async () => {
        throw new ConnectError('not implemented', Code.Unimplemented);
      }),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);

    await startAndSettle(discovery);

    expect(observed.latest()).toMatchObject({
      entries: [],
      failedSourceCount: 0,
      failedCandidateCount: 0,
      sourceCount: 1
    });
  });

  it('uses one shared six-request scheduler', async () => {
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
    const discovery = createServerDirectoryDiscovery(
      Array.from({ length: 12 }, (_, index) => `https://source-${index}.example`),
      { listNeighbors, getServerInfo: vi.fn(async (origin: string) => profile(origin)) }
    );
    discovery.start();

    await vi.waitFor(() => expect(listNeighbors).toHaveBeenCalledTimes(6));
    expect(maximumActive).toBe(6);
    releaseRequests();
    await discovery.whenIdle();
    expect(listNeighbors).toHaveBeenCalledTimes(12);
    expect(maximumActive).toBe(6);
  });

  it('keeps combined directory and profile work within the shared ceiling', async () => {
    let active = 0;
    let maximumActive = 0;
    let releaseRest!: () => void;
    const restGate = new Promise<void>((resolve) => {
      releaseRest = resolve;
    });
    const listNeighbors = vi.fn(async (origin: string) => {
      active += 1;
      maximumActive = Math.max(maximumActive, active);
      if (origin === 'https://source.example') {
        active -= 1;
        return [recommendation('https://candidate.example')];
      }
      await restGate;
      active -= 1;
      return origin === 'https://candidate.example'
        ? [recommendation('https://source.example')]
        : [];
    });
    const getServerInfo = vi.fn(async (origin: string) => {
      active += 1;
      maximumActive = Math.max(maximumActive, active);
      await restGate;
      active -= 1;
      return profile(origin);
    });
    const discovery = createServerDirectoryDiscovery(
      [
        'https://source.example',
        ...Array.from({ length: 5 }, (_, index) => `https://slow-${index}.example`)
      ],
      { listNeighbors, getServerInfo }
    );
    discovery.start();

    await vi.waitFor(() => expect(getServerInfo).toHaveBeenCalled());
    expect(active).toBe(6);
    expect(maximumActive).toBe(6);

    releaseRest();
    await discovery.whenIdle();
    expect(maximumActive).toBe(6);
  });

  it('rotates ready sources within one candidate batch', async () => {
    const fromA = Array.from({ length: 20 }, (_, index) => `https://a-${index}.example`);
    const fromB = Array.from({ length: 20 }, (_, index) => `https://b-${index}.example`);
    const listNeighbors = vi.fn(async (origin: string) => {
      if (origin === 'https://a.example') return fromA.map((value) => recommendation(value));
      if (origin === 'https://b.example') return fromB.map((value) => recommendation(value));
      return [
        recommendation(origin.startsWith('https://a-') ? 'https://a.example' : 'https://b.example')
      ];
    });
    const discovery = createServerDirectoryDiscovery(
      ['https://a.example', 'https://b.example'],
      { listNeighbors, getServerInfo: vi.fn(async (origin: string) => profile(origin)) }
    );

    await startAndSettle(discovery);

    const candidateCalls = listNeighbors.mock.calls
      .map(([origin]) => origin)
      .filter((origin) => origin !== 'https://a.example' && origin !== 'https://b.example');
    expect(candidateCalls).toHaveLength(SERVER_DIRECTORY_LIMITS.candidateBatchSize);
    expect(candidateCalls).toContain('https://b-0.example');
  });

  it('times out a stalled request after ten seconds', async () => {
    expect(SERVER_DIRECTORY_LIMITS.timeoutMs).toBe(10_000);
    const discovery = createServerDirectoryDiscovery(['https://slow.example'], {
      requestTimeoutMs: 5,
      listNeighbors: vi.fn(
        async (_origin: string, options?: { signal?: AbortSignal }): Promise<PublicNeighbor[]> =>
          new Promise((_, reject) =>
            options?.signal?.addEventListener('abort', () => reject(options.signal?.reason), {
              once: true
            })
          )
      ),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);
    discovery.start();

    await discovery.whenIdle();

    expect(observed.latest().failedSourceCount).toBe(1);
  });

  it('pauses queued work while hidden and resumes it when visible', async () => {
    let releaseRequests!: () => void;
    const requestGate = new Promise<void>((resolve) => {
      releaseRequests = resolve;
    });
    const listNeighbors = vi.fn(async () => {
      await requestGate;
      return [];
    });
    const discovery = createServerDirectoryDiscovery(
      Array.from({ length: 8 }, (_, index) => `https://source-${index}.example`),
      {
        listNeighbors,
        getServerInfo: vi.fn(async (origin: string) => profile(origin))
      }
    );
    const observed = observe(discovery);
    discovery.start();

    await vi.waitFor(() => expect(listNeighbors).toHaveBeenCalledTimes(6));
    discovery.setVisible(false);
    releaseRequests();
    await vi.waitFor(() => expect(observed.latest().activeRequestCount).toBe(0));
    expect(listNeighbors).toHaveBeenCalledTimes(6);
    expect(observed.latest().isPaused).toBe(true);

    discovery.setVisible(true);
    await discovery.whenIdle();
    expect(listNeighbors).toHaveBeenCalledTimes(8);
  });

  it('aborts active work and rejects idle waiters on cancellation', async () => {
    let requestSignal: AbortSignal | undefined;
    const discovery = createServerDirectoryDiscovery(['https://source.example'], {
      listNeighbors: vi.fn(
        async (_origin: string, options?: { signal?: AbortSignal }): Promise<PublicNeighbor[]> => {
          requestSignal = options?.signal;
          return new Promise((_, reject) =>
            options?.signal?.addEventListener('abort', () => reject(options.signal?.reason), {
              once: true
            })
          );
        }
      ),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);
    discovery.start();
    const idle = discovery.whenIdle();
    await vi.waitFor(() => expect(requestSignal).toBeDefined());

    discovery.cancel();

    await expect(idle).rejects.toMatchObject({ name: 'AbortError' });
    expect(requestSignal?.aborted).toBe(true);
    expect(observed.latest().failedSourceCount).toBe(0);
  });

  it('requires explicit non-overlapping batches after the automatic batch', async () => {
    const candidates = Array.from({ length: 30 }, (_, index) => `https://n-${index}.example`);
    const listNeighbors = vi.fn(async (origin: string) => {
      if (origin === 'https://source.example')
        return candidates.map((value) => recommendation(value));
      return [recommendation('https://source.example')];
    });
    const discovery = createServerDirectoryDiscovery(['https://source.example'], {
      listNeighbors,
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);

    await startAndSettle(discovery);
    expect(observed.latest().directoryRequestCount).toBe(13);
    expect(observed.latest().canLoadMore).toBe(true);

    discovery.loadMore();
    discovery.loadMore();
    await discovery.whenIdle();
    expect(observed.latest().directoryRequestCount).toBe(25);
    expect(observed.latest().canLoadMore).toBe(true);
  });

  it('enforces directory, profile, total, and candidate session limits', async () => {
    const candidates = Array.from({ length: 100 }, (_, index) => `https://n-${index}.example`);
    const discovery = createServerDirectoryDiscovery(['https://source.example'], {
      listNeighbors: vi.fn(async (origin: string) =>
        origin === 'https://source.example'
          ? candidates.map((value) => recommendation(value))
          : [recommendation('https://source.example')]
      ),
      getServerInfo: vi.fn(async (origin: string) => profile(origin))
    });
    const observed = observe(discovery);
    await startAndSettle(discovery);

    while (observed.latest().canLoadMore) {
      discovery.loadMore();
      await discovery.whenIdle();
    }

    expect(observed.latest()).toMatchObject({
      directoryRequestCount: SERVER_DIRECTORY_LIMITS.directoryRequests,
      profileRequestCount: SERVER_DIRECTORY_LIMITS.profileRequests,
      totalRequestCount: SERVER_DIRECTORY_LIMITS.totalRequests,
      sessionLimitReached: true
    });
  });
});
