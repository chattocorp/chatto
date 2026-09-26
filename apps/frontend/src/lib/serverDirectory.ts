import { Code, ConnectError } from '@connectrpc/connect';
import {
  getPublicServerInfo,
  listNeighborhoodServers,
  type NeighborhoodServerProfile,
  type PublicServerInfo
} from '$lib/api-client/server';

/** Request limits for public discovery requests from this client. */
export const SERVER_DIRECTORY_LIMITS = {
  concurrency: 6,
  timeoutMs: 10_000
} as const;

export type ServerProfileEntry = {
  origin: string;
  profile: PublicServerInfo | null;
};

/** One merged Server Directory result. */
export type ServerDirectoryEntry = {
  origin: string;
  profile: NeighborhoodServerProfile;
  /**
   * Origin of the registered server that supplied the profile. Its logo and
   * banner URLs identify copies on that server.
   */
  imageOrigin: string;
  /** Canonical origins of the servers whose recommendations are shown. */
  sourceOrigins: string[];
};

/** Merged Neighborhoods of the registered servers. */
export type ServerDirectory = {
  entries: ServerDirectoryEntry[];
  /** Number of registered servers that the client asked. */
  sourceCount: number;
  /** Number of registered servers whose Neighborhood did not load. */
  failedSourceCount: number;
};

type DirectoryLoadOptions = {
  signal?: AbortSignal;
  listNeighborhood?: typeof listNeighborhoodServers;
};

type ProfileLoadOptions = {
  signal?: AbortSignal;
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

/** Convert a hostname or HTTP(S) URL entered by a person to its origin. */
export function serverOriginFromInput(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) && !/^https?:\/\//i.test(trimmed)) return null;
  return canonicalServerOrigin(/^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`);
}

/** Load public profiles without making one failed server hide the others. */
export async function loadServerProfiles(
  origins: readonly string[],
  options: ProfileLoadOptions = {}
): Promise<ServerProfileEntry[]> {
  const getServerInfo = options.getServerInfo ?? getPublicServerInfo;
  const profiles = await mapWithConcurrency(
    origins,
    SERVER_DIRECTORY_LIMITS.concurrency,
    (origin) => getServerInfo(origin, { signal: discoverySignal(options.signal) }).catch(() => null)
  );
  throwIfAborted(options.signal);

  return origins.map((origin, index) => ({ origin, profile: profiles[index] ?? null }));
}

/**
 * Load and merge the cached Neighborhood of each registered server. The client
 * contacts only registered servers. A registered server already knows the
 * user's network address, so the directory reveals it to no other server.
 * An older server without a Neighborhood contributes no results and does not
 * count as failed. Results keep the order in which they first appear.
 */
export async function loadServerDirectory(
  registeredOrigins: readonly string[],
  options: DirectoryLoadOptions = {}
): Promise<ServerDirectory> {
  const listNeighborhood = options.listNeighborhood ?? listNeighborhoodServers;
  const sources = [
    ...new Set(
      registeredOrigins.flatMap((value) => {
        const origin = canonicalServerOrigin(value);
        return origin ? [origin] : [];
      })
    )
  ];
  const responses = await mapWithConcurrency(
    sources,
    SERVER_DIRECTORY_LIMITS.concurrency,
    async (source) => {
      try {
        return await listNeighborhood(source, { signal: discoverySignal(options.signal) });
      } catch (error) {
        throwIfAborted(options.signal);
        if (error instanceof ConnectError && error.code === Code.Unimplemented) return [];
        return null;
      }
    }
  );
  throwIfAborted(options.signal);

  const entries = new Map<string, ServerDirectoryEntry>();
  let failedSourceCount = 0;
  sources.forEach((source, index) => {
    const servers = responses[index];
    if (!servers) {
      failedSourceCount += 1;
      return;
    }
    for (const server of servers) {
      const origin = canonicalServerOrigin(server.origin);
      if (!origin || origin === source) continue;
      let entry = entries.get(origin);
      if (!entry) {
        entry = { origin, profile: server.profile, imageOrigin: source, sourceOrigins: [] };
        entries.set(origin, entry);
      }
      const recommenders = [
        ...(server.directNeighbor ? [source] : []),
        ...server.recommendedByOrigins.flatMap((value) => {
          const recommender = canonicalServerOrigin(value);
          return recommender ? [recommender] : [];
        })
      ];
      for (const recommender of recommenders) {
        if (recommender !== origin && !entry.sourceOrigins.includes(recommender)) {
          entry.sourceOrigins.push(recommender);
        }
      }
    }
  });

  return {
    entries: [...entries.values()].filter((entry) => entry.sourceOrigins.length > 0),
    sourceCount: sources.length,
    failedSourceCount
  };
}

function discoverySignal(
  parent?: AbortSignal,
  timeoutMs: number = SERVER_DIRECTORY_LIMITS.timeoutMs
): AbortSignal {
  const timeout = AbortSignal.timeout(timeoutMs);
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

  await Promise.all(Array.from({ length: Math.min(concurrency, values.length) }, () => worker()));
  return results;
}
