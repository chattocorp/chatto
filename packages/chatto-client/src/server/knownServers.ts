import { signal } from '../reactivity/index.js';
import type { NeighborhoodServerProfile } from '../api/server.js';
import { parseServerUrl } from '../util/serverUrl.js';

/** Public metadata for an explicitly discovered server, without a registration or session. */
export interface KnownServer {
  origin: string;
  profile: NeighborhoodServerProfile;
}

const storageKey = 'chatto:known-servers';

/**
 * Remembers explicitly discovered servers. This store never connects, signs in,
 * or starts discovery itself. Hosts choose when to query these discovery sources.
 */
export class KnownServers {
  readonly #servers = signal<readonly KnownServer[]>([]);
  readonly #storage: Storage | undefined;

  get servers(): readonly KnownServer[] {
    return this.#servers.get();
  }

  constructor(storage?: Storage) {
    this.#storage = storage;
    try {
      const saved: unknown = JSON.parse(storage?.getItem(storageKey) ?? '[]');
      if (!Array.isArray(saved)) return;
      const servers = new Map<string, KnownServer>();
      for (const entry of saved) {
        try {
          const server = readServer(entry);
          servers.set(server.origin, server);
        } catch {
          // Ignore malformed records without discarding the other saved servers.
        }
      }
      this.#servers.set([...servers.values()]);
    } catch {
      // Storage can be unavailable or contain invalid JSON. Discovery still works in memory.
    }
  }

  /** Remember only public discovery fields; URLs are canonical origins and duplicates update in place. */
  remember(origin: string, profile: NeighborhoodServerProfile): void {
    const server = readServer({ origin, profile });
    const servers = new Map(this.servers.map((entry) => [entry.origin, entry]));
    servers.set(server.origin, server);
    this.#save([...servers.values()]);
  }

  /** Stop remembering an explicit discovery source without changing any registration. */
  forget(origin: string): void {
    const canonical = parseServerUrl(origin).origin;
    this.#save(this.servers.filter((server) => server.origin !== canonical));
  }

  #save(servers: readonly KnownServer[]): void {
    this.#servers.set(servers);
    try {
      this.#storage?.setItem(storageKey, JSON.stringify(servers));
    } catch {
      // Keep the current session usable when device storage is blocked or full.
    }
  }
}

/** Validate persisted input and copy an explicit allowlist, never credentials or auth configuration. */
function readServer(value: unknown): KnownServer {
  if (
    !value ||
    typeof value !== 'object' ||
    !('origin' in value) ||
    typeof value.origin !== 'string' ||
    !('profile' in value)
  ) {
    throw new Error('Invalid known server');
  }
  const profile = value.profile;
  if (
    !profile ||
    typeof profile !== 'object' ||
    !('name' in profile) ||
    typeof profile.name !== 'string' ||
    !('version' in profile) ||
    typeof profile.version !== 'string'
  ) {
    throw new Error('Invalid known server profile');
  }
  return {
    origin: parseServerUrl(value.origin).origin,
    profile: {
      name: profile.name,
      version: profile.version,
      description:
        'description' in profile && typeof profile.description === 'string'
          ? profile.description
          : null,
      iconUrl: 'iconUrl' in profile && typeof profile.iconUrl === 'string' ? profile.iconUrl : null,
      bannerUrl:
        'bannerUrl' in profile && typeof profile.bannerUrl === 'string' ? profile.bannerUrl : null
    }
  };
}
