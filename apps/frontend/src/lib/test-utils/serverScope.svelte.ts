import type { ConnectAPIConfig } from '$lib/api-client/connect';
import { fakeServer, type FakeServerRoutes } from './fakeServer';
import type { CurrentUser } from '$lib/api-client/viewer';
import { CurrentUserState } from '$lib/auth/currentUser.svelte';
import { NO_SERVER_PERMISSIONS, type ServerPermissions } from '$lib/state/server/permissions';
import type { ServerScope } from '$lib/state/server/scope.svelte';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { ServerStateStore } from '$lib/state/server/store.svelte';

/** Options for {@link createTestServerScope}. Every option has a working default. */
export type TestServerScopeOptions = {
  /** Server ID of the scope. Default: `'server-1'`. */
  serverId?: string;
  /** Query scope of the connection. Default: the server ID with a `-session` suffix. */
  queryScope?: string;
  /**
   * Accepted and verified account, or null for no loaded account. Default: user
   * `viewer-1`. To model a pending verification, call
   * `currentUser.invalidateVerification()`; `currentUser.accept()` verifies again.
   */
  viewer?: Partial<CurrentUser> | null;
  /** Permission flags over `NO_SERVER_PERMISSIONS`. The result is loaded unless `loaded` is false. */
  permissions?: Partial<ServerPermissions>;
  /** What `serverInfo.isSupportedVersion` reports. Default: true. */
  isSupportedVersion?: boolean;
  /**
   * What `connection.getAPI` returns for every factory. Without it, `getAPI`
   * runs the real factory with a stub config, so `vi.mock` of an API module works.
   */
  api?: object;
  /**
   * Services of an in-memory fake server. With them, `getAPI` runs the real API
   * factories and clients against these handlers. See {@link fakeServer}.
   */
  routes?: FakeServerRoutes;
  /**
   * Extra `store.serverInfo` members, such as `livekitUrl`. Getters are kept.
   * `isSupportedVersion` reads the fixture, unless the `store` option replaces
   * the whole `serverInfo`.
   */
  serverInfo?: object;
  /** Extra store members, such as `navigation` or `projection`. Getters are kept. */
  store?: object;
  /** Extra connection members. Getters are kept. */
  connection?: object;
};

/**
 * A typed fake of the `/chat/[serverId]` server scope for component specs.
 * Tests change its `$state` fields to drive the component under test.
 *
 * The server ID and query scope are fixed, as in the app: a change of server or
 * session remounts the route subtree with a new scope. To model one, unmount
 * the component and render it again with a new fixture.
 */
export class TestServerScope {
  readonly serverId: string;
  /** Result of `scope.isCurrent()`. */
  current = $state(true);
  permissions = $state<ServerPermissions>(NO_SERVER_PERMISSIONS);
  /** Result of `serverInfo.isSupportedVersion`. */
  isSupportedVersion = $state(true);
  /** Projection viewer ID. Default: the accepted account's ID. */
  projectionViewerId = $state<string | null | undefined>(undefined);
  /** Query scope of the connection. */
  readonly queryScope: string;
  /** A real account state, so `update()` and its same-account rule behave as in the app. */
  readonly currentUser = new CurrentUserState();
  readonly scope: ServerScope;

  constructor(options: TestServerScopeOptions = {}) {
    this.serverId = options.serverId ?? 'server-1';
    this.queryScope = options.queryScope ?? `${this.serverId}-session`;
    this.permissions = { ...NO_SERVER_PERMISSIONS, loaded: true, ...options.permissions };
    this.isSupportedVersion = options.isSupportedVersion ?? true;
    this.currentUser.loading = false;
    if (options.viewer !== null) {
      this.currentUser.accept({
        id: 'viewer-1',
        login: 'viewer',
        displayName: 'Viewer',
        settings: null,
        ...options.viewer
      } as CurrentUser);
    }

    this.scope = buildScope(this, options);
  }
}

/** Build the typed scope objects whose getters read the fixture's current state. */
function buildScope(t: TestServerScope, options: TestServerScopeOptions): ServerScope {
  const store = buildStore(t, options.serverInfo, options.store);
  const connection = withMembers(
    {
      get serverId() {
        return t.serverId;
      },
      get queryScope() {
        return t.queryScope;
      },
      isConnected: true,
      showConnectionLostBanner: false,
      get connectBaseUrl() {
        return `https://${t.serverId}.example.test/api/connect`;
      },
      bearerToken: null,
      get apiConfig(): ConnectAPIConfig {
        const config: ConnectAPIConfig = {
          serverId: t.serverId,
          queryScope: t.queryScope,
          baseUrl: `https://${t.serverId}.example.test/api/connect`,
          bearerToken: null
        };
        return options.routes ? fakeServer(options.routes, config) : config;
      },
      getAPI<T>(factory: (config: ConnectAPIConfig) => T): T {
        return (options.api as T | undefined) ?? factory(this.apiConfig);
      },
      invalidatePrivateData() {},
      forceReconnect() {}
    },
    options.connection
  );
  return {
    get serverId() {
      return t.serverId;
    },
    connection: connection as unknown as ServerConnection,
    store,
    isCurrent: () => t.current
  };
}

/** Build the scope's store. Its getters read the fixture's current state. */
function buildStore(
  t: TestServerScope,
  serverInfo: object | undefined,
  extra: object | undefined
): ServerStateStore {
  const store = withMembers(
    {
      serverId: t.serverId,
      currentUser: t.currentUser,
      get accountId() {
        return t.currentUser.user?.id ?? null;
      },
      get viewerId() {
        return t.currentUser.user?.id ?? null;
      },
      get viewerUser() {
        return t.currentUser.user;
      },
      get projectionViewerId() {
        return t.projectionViewerId === undefined
          ? (t.currentUser.user?.id ?? null)
          : t.projectionViewerId;
      },
      get isAuthenticated() {
        // Like a cookie session: the loaded account must also be verified.
        const user = t.currentUser.user;
        return user != null && t.currentUser.verifiedUserId === user.id;
      },
      get permissions() {
        return t.permissions;
      },
      serverInfo: withMembers(withMembers({}, serverInfo), {
        get isSupportedVersion() {
          return t.isSupportedVersion;
        }
      })
    },
    extra
  );
  return store as unknown as ServerStateStore;
}

/** Copy `extra`'s own members onto `base`, keeping getters and setters. */
function withMembers<T extends object>(base: T, extra: object | undefined): T {
  if (extra) Object.defineProperties(base, Object.getOwnPropertyDescriptors(extra));
  return base;
}

let latest: TestServerScope | null = null;

/** Create a fake server scope. `serverScopeModule` serves the most recent one. */
export function createTestServerScope(options?: TestServerScopeOptions): TestServerScope {
  latest = new TestServerScope(options);
  return latest;
}

/**
 * A replacement for `$lib/state/server/scope.svelte` in `vi.mock`:
 *
 * ```ts
 * vi.mock('$lib/state/server/scope.svelte', async () =>
 *   (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
 * );
 * ```
 */
export const serverScopeModule = {
  useServerScope(): ServerScope {
    if (!latest) throw new Error('Call createTestServerScope() before rendering');
    return latest.scope;
  },
  provideServerScope(): void {}
};
