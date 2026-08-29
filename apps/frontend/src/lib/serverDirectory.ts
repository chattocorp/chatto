import { Code, ConnectError } from '@connectrpc/connect';
import {
  getPublicNeighborOrigins,
  getPublicServerInfo,
  type PublicServerInfo
} from '$lib/api-client/server';

const DISCOVERY_TIMEOUT_MS = 10_000;
const PROFILE_CONCURRENCY = 6;
const MAX_NEIGHBORS_PER_SOURCE = 100;

export type ServerDirectoryEntry = {
  origin: string;
  profile: PublicServerInfo | null;
};

export type ServerDirectorySnapshot = {
  entries: ServerDirectoryEntry[];
  failedSourceCount: number;
  sourceCount: number;
};

type DirectoryLoadOptions = {
  signal?: AbortSignal;
  listNeighbors?: typeof getPublicNeighborOrigins;
  getServerInfo?: typeof getPublicServerInfo;
};

/** Convert an advertised URL to a canonical HTTP(S) origin. */
export function canonicalServerOrigin(value: string): string | null {
  try {
    const url = new URL(value);
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || url.username || url.password) {
      return null;
    }
    return url.origin;
  } catch {
    return null;
  }
}

/**
 * Load the direct Neighbor directories of registered servers and hydrate each
 * distinct advertised origin with its public profile. Result order follows the
 * source responses but is not a product ordering contract.
 */
export async function loadServerDirectory(
  registeredOrigins: readonly string[],
  options: DirectoryLoadOptions = {}
): Promise<ServerDirectorySnapshot> {
  const listNeighbors = options.listNeighbors ?? getPublicNeighborOrigins;
  const getServerInfo = options.getServerInfo ?? getPublicServerInfo;
  const sourceResults = await Promise.allSettled(
    registeredOrigins.map((origin) =>
      listNeighbors(origin, { signal: discoverySignal(options.signal) }).catch((error) => {
        if (error instanceof ConnectError && error.code === Code.Unimplemented) return [];
        throw error;
      })
    )
  );
  throwIfAborted(options.signal);

  const advertisedOrigins: string[] = [];
  const seen = new Set<string>();
  let failedSourceCount = 0;

  for (const result of sourceResults) {
    if (result.status === 'rejected') {
      failedSourceCount += 1;
      continue;
    }
    for (const advertised of result.value.slice(0, MAX_NEIGHBORS_PER_SOURCE)) {
      const origin = canonicalServerOrigin(advertised);
      if (!origin || seen.has(origin)) continue;
      seen.add(origin);
      advertisedOrigins.push(origin);
    }
  }

  const profiles = await mapWithConcurrency(advertisedOrigins, PROFILE_CONCURRENCY, (origin) =>
    getServerInfo(origin, { signal: discoverySignal(options.signal) }).catch(() => null)
  );
  throwIfAborted(options.signal);

  return {
    entries: advertisedOrigins.map((origin, index) => ({
      origin,
      profile: profiles[index] ?? null
    })),
    failedSourceCount,
    sourceCount: registeredOrigins.length
  };
}

function discoverySignal(parent?: AbortSignal): AbortSignal {
  const timeout = AbortSignal.timeout(DISCOVERY_TIMEOUT_MS);
  return parent ? AbortSignal.any([parent, timeout]) : timeout;
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw signal.reason;
}

async function mapWithConcurrency<T, R>(
  values: readonly T[],
  concurrency: number,
  operation: (value: T) => Promise<R>
): Promise<R[]> {
  const results = new Array<R>(values.length);
  let nextIndex = 0;

  async function worker() {
    while (nextIndex < values.length) {
      const index = nextIndex++;
      results[index] = await operation(values[index]!);
    }
  }

  await Promise.all(
    Array.from({ length: Math.min(concurrency, values.length) }, () => worker())
  );
  return results;
}
