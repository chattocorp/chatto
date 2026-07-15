import { Code, ConnectError } from '@connectrpc/connect';
import { createClientSyncAPI, ClientSyncTimeFormat } from '$lib/api-client/clientSync';
import { getLocale, setLocale, type Locale } from '$lib/i18n/runtime';
import { selectableLocales } from '$lib/i18n/locales';
import { TimeFormat } from '$lib/render/types';
import { SvelteMap } from 'svelte/reactivity';
import { Codecs, globalSlot } from '$lib/storage/slot';
import { serverConnectionManager } from './server/serverConnection.svelte';
import { serverRegistry, type RegisteredServer } from './server/registry.svelte';
import {
  canonicalServerOrigin,
  generateServerId,
  type PendingHomeMove,
  portableMetadataUpdate,
  recordPendingHomeMove,
  wasRemovedSinceLastSync
} from './server/serverIdentity';

type CachedPersonalSettings = {
  initialized: boolean;
  locale: Locale;
  timezone: string | null;
  timeFormat: TimeFormat;
};

const anonymousCacheSlot = globalSlot(
  'personal-settings',
  {
    initialized: false,
    locale: getLocale(),
    timezone: null,
    timeFormat: TimeFormat.Auto
  } satisfies CachedPersonalSettings,
  Codecs.json<CachedPersonalSettings>()
);

const accountCachesSlot = globalSlot(
  'personal-settings-by-account',
  {} as Record<string, CachedPersonalSettings>,
  Codecs.json<Record<string, CachedPersonalSettings>>()
);

const homeAccountBindingsSlot = globalSlot(
  'personal-home-account-bindings',
  {} as Record<string, string>,
  Codecs.json<Record<string, string>>()
);

const directorySnapshotsSlot = globalSlot(
  'personal-directory-snapshots',
  {} as Record<string, string[]>,
  Codecs.json<Record<string, string[]>>()
);

const pendingHomeMovesSlot = globalSlot(
  'personal-pending-home-moves',
  {} as Record<string, PendingHomeMove>,
  Codecs.json<Record<string, PendingHomeMove>>()
);

export type ClientSyncStatus = 'local' | 'loading' | 'synced' | 'unavailable' | 'error';

class ClientSyncState {
  #cache = $state<CachedPersonalSettings>(anonymousCacheSlot.get());
  #accountCaches = accountCachesSlot.get();
  #homeAccountBindings = homeAccountBindingsSlot.get();
  #activeAccountKey: string | null = null;
  #directorySnapshots: Record<string, string[]> = directorySnapshotsSlot.get();
  #pendingHomeMoves = pendingHomeMovesSlot.get();
  status = $state<ClientSyncStatus>('local');
  loadedHomeServerId = $state<string | null>(null);
  #loadGeneration = 0;
  #directoryQueue: Promise<void> = Promise.resolve();
  #homeMoveQueue: Promise<void> = Promise.resolve();
  #homeMoveRetryTimer: ReturnType<typeof setTimeout> | null = null;
  #explicitHomeServerId: string | null = null;
  #movingHomeServerId: string | null = null;

  get locale(): Locale {
    return this.#cache.locale;
  }

  get timezone(): string | null {
    return this.#cache.timezone;
  }

  get timeFormat(): TimeFormat {
    return this.#cache.timeFormat;
  }

  get isInitialized(): boolean {
    return this.#cache.initialized;
  }

  async setLocale(locale: Locale): Promise<void> {
    this.#persist({ locale, initialized: true });
    await setLocale(locale);
    await this.#savePreferences({ locale }, ['locale']);
  }

  async setDisplaySettings(timezone: string | null, timeFormat: TimeFormat): Promise<void> {
    this.#persist({ timezone, timeFormat, initialized: true });
    await this.#savePreferences(
      {
        timezone: timezone ?? undefined,
        timeFormat: renderTimeFormatToClientSync(timeFormat)
      },
      ['timezone', 'time_format']
    );
  }

  async selectHomeServer(homeServerId: string): Promise<boolean> {
    if (
      !serverRegistry.getServer(homeServerId) ||
      !serverRegistry.isAuthenticated(homeServerId) ||
      !serverRegistry.isClientSyncCapable(homeServerId)
    ) {
      return false;
    }
    const previousHomeServerId = serverRegistry.homeServerId;
    this.#explicitHomeServerId = homeServerId;
    this.#movingHomeServerId =
      previousHomeServerId && previousHomeServerId !== homeServerId ? homeServerId : null;
    if (!serverRegistry.setHomeServer(homeServerId)) {
      this.#explicitHomeServerId = null;
      this.#movingHomeServerId = null;
      return false;
    }
    try {
      if (previousHomeServerId && previousHomeServerId !== homeServerId) {
        await this.#moveToHomeServer(homeServerId, previousHomeServerId);
      } else {
        await this.load(homeServerId);
      }
      return true;
    } finally {
      if (this.#explicitHomeServerId === homeServerId) this.#explicitHomeServerId = null;
      if (this.#movingHomeServerId === homeServerId) this.#movingHomeServerId = null;
    }
  }

  async load(homeServerId: string): Promise<void> {
    // A deliberate move writes the current client state to the new home. The
    // root sync effect can observe the registry change concurrently; do not let
    // that automatic load replace the state being transferred.
    if (this.#movingHomeServerId === homeServerId) return;
    const generation = ++this.#loadGeneration;
    const accountKey = this.#activateHomeAccount(homeServerId);
    if (!accountKey) {
      this.#rejectHomeAccount();
      return;
    }
    const api = this.#api(homeServerId);
    if (!api) {
      this.#setLocalStatus();
      return;
    }

    this.status = 'loading';
    try {
      const [remotePreferences, remoteDirectory] = await Promise.all([
        api.getPreferences(),
        api.listKnownServers()
      ]);
      if (generation !== this.#loadGeneration) return;

      const hasRemotePreferences =
        remotePreferences.locale !== undefined ||
        remotePreferences.timezone !== undefined ||
        remotePreferences.timeFormat !== undefined;

      if (hasRemotePreferences) {
        const locale = isSelectableLocale(remotePreferences.locale)
          ? remotePreferences.locale
          : this.#cache.locale;
        this.#persist({
          initialized: true,
          locale,
          timezone: remotePreferences.timezone ?? null,
          timeFormat: clientSyncTimeFormatToRender(remotePreferences.timeFormat)
        });
        if (locale !== getLocale()) await setLocale(locale);
      } else {
        this.#adoptLegacyHomeSettings(homeServerId);
        await api.updatePreferences(
          {
            locale: this.#cache.locale,
            timezone: this.#cache.timezone ?? undefined,
            timeFormat: renderTimeFormatToClientSync(this.#cache.timeFormat)
          },
          ['locale', 'timezone', 'time_format']
        );
      }

      const remoteHome = this.#mergeRemoteServers(
        remoteDirectory.servers,
        remoteDirectory.homeServerId,
        accountKey
      );
      if (
        this.#explicitHomeServerId === null &&
        remoteHome &&
        serverRegistry.isAuthenticated(remoteHome) &&
        serverRegistry.isClientSyncCapable(remoteHome)
      ) {
        serverRegistry.setHomeServer(remoteHome);
      }

      this.loadedHomeServerId = serverRegistry.homeServerId;
      this.status = 'synced';
      await this.reconcileDirectory();
    } catch (error) {
      if (generation !== this.#loadGeneration) return;
      this.loadedHomeServerId = null;
      if (
        error instanceof ConnectError &&
        (error.code === Code.Unimplemented || error.code === Code.NotFound)
      ) {
        this.status = 'unavailable';
        return;
      }
      console.error('[client-sync] failed to load home-server data', error);
      this.status = 'error';
    }
  }

  async #moveToHomeServer(homeServerId: string, previousHomeServerId: string): Promise<void> {
    const generation = ++this.#loadGeneration;
    const transferredPreferences = { ...this.#cache };
    const accountKey = this.#activateHomeAccount(homeServerId);
    if (!accountKey) {
      this.#rejectHomeAccount();
      return;
    }
    this.#replaceCache(transferredPreferences);
    const api = this.#api(homeServerId);
    if (!api) {
      this.loadedHomeServerId = null;
      this.#setLocalStatus();
      return;
    }

    this.status = 'loading';
    try {
      this.#adoptLegacyHomeSettings(homeServerId);
      await api.updatePreferences(
        {
          locale: this.#cache.locale,
          timezone: this.#cache.timezone ?? undefined,
          timeFormat: renderTimeFormatToClientSync(this.#cache.timeFormat)
        },
        ['locale', 'timezone', 'time_format']
      );
      if (generation !== this.#loadGeneration) return;

      const remoteDirectory = await api.listKnownServers();
      if (generation !== this.#loadGeneration) return;
      this.#mergeRemoteServers(remoteDirectory.servers, remoteDirectory.homeServerId, accountKey);

      this.loadedHomeServerId = homeServerId;
      this.status = 'synced';
      await this.reconcileDirectory();
      this.#recordPendingHomeMove(previousHomeServerId, homeServerId);
      await this.retryPendingHomeMoves();
    } catch (error) {
      if (generation !== this.#loadGeneration) return;
      this.loadedHomeServerId = null;
      if (
        error instanceof ConnectError &&
        (error.code === Code.Unimplemented || error.code === Code.NotFound)
      ) {
        this.status = 'unavailable';
        return;
      }
      console.error('[client-sync] failed to move home-server data', error);
      this.status = 'error';
    }
  }

  reconcileDirectory(): Promise<void> {
    const homeServerId = this.loadedHomeServerId;
    const accountKey = homeServerId ? this.#homeAccountKey(homeServerId, false) : null;
    if (!homeServerId || !accountKey || this.status !== 'synced') return Promise.resolve();
    this.#directoryQueue = this.#directoryQueue
      .catch(() => {})
      .then(() => this.#reconcileDirectoryNow(homeServerId, accountKey));
    return this.#directoryQueue;
  }

  reset(): void {
    this.#loadGeneration++;
    this.loadedHomeServerId = null;
    this.#activeAccountKey = null;
    this.#replaceCache(anonymousCacheSlot.get(), false);
    if (this.#cache.locale !== getLocale()) void setLocale(this.#cache.locale);
    this.#setLocalStatus();
  }

  forgetDeviceAccounts(): void {
    this.#accountCaches = {};
    this.#homeAccountBindings = {};
    this.#directorySnapshots = {};
    this.#pendingHomeMoves = {};
    accountCachesSlot.set(this.#accountCaches);
    homeAccountBindingsSlot.set(this.#homeAccountBindings);
    directorySnapshotsSlot.set(this.#directorySnapshots);
    pendingHomeMovesSlot.set(this.#pendingHomeMoves);
    if (this.#homeMoveRetryTimer) clearTimeout(this.#homeMoveRetryTimer);
    this.#homeMoveRetryTimer = null;
    this.reset();
  }

  retryPendingHomeMoves(): Promise<void> {
    this.#homeMoveQueue = this.#homeMoveQueue
      .catch(() => {})
      .then(async () => {
        for (const [previousOrigin, move] of Object.entries(this.#pendingHomeMoves)) {
          const previous = serverRegistry.servers.find(
            (server) => serverOrigin(server.url) === previousOrigin
          );
          const next = serverRegistry.servers.find(
            (server) => serverOrigin(server.url) === move.newOrigin
          );
          const previousUserID = previous ? this.#authenticatedUserID(previous.id) : null;
          if (
            !previous ||
            !next ||
            !serverRegistry.isAuthenticated(previous.id) ||
            previousUserID !== move.previousUserId ||
            this.#homeAccountBindings[previousOrigin] !== move.previousUserId
          ) {
            continue;
          }
          try {
            await this.#pointPreviousHomeAt(previous.id, next.id);
            delete this.#pendingHomeMoves[previousOrigin];
            pendingHomeMovesSlot.set(this.#pendingHomeMoves);
          } catch (error) {
            console.error('[client-sync] failed to propagate home move; will retry', error);
            this.#scheduleHomeMoveRetry();
          }
        }
      });
    return this.#homeMoveQueue;
  }

  #setLocalStatus(): void {
    this.status = serverRegistry.homeServerId ? 'unavailable' : 'local';
  }

  #api(homeServerId: string) {
    const home = serverRegistry.getServer(homeServerId);
    const store = serverRegistry.tryGetStore(homeServerId);
    if (!home || !store?.isAuthenticated) return null;
    const connection = serverConnectionManager.getClient(homeServerId);
    return createClientSyncAPI({
      serverId: homeServerId,
      baseUrl: connection.connectBaseUrl,
      bearerToken: connection.bearerToken
    });
  }

  async #savePreferences(
    preferences: Parameters<ReturnType<typeof createClientSyncAPI>['updatePreferences']>[0],
    paths: Array<'locale' | 'timezone' | 'time_format'>
  ): Promise<void> {
    const homeServerId = this.loadedHomeServerId;
    const api = homeServerId ? this.#api(homeServerId) : null;
    if (!api) return;
    try {
      await api.updatePreferences(preferences, paths);
      this.status = 'synced';
    } catch (error) {
      console.error('[client-sync] failed to save preferences', error);
      this.status = 'error';
      throw error;
    }
  }

  async #reconcileDirectoryNow(homeServerId: string, accountKey: string): Promise<void> {
    const api = this.#api(homeServerId);
    if (!api) return;
    const remote = await api.listKnownServers();
    const remoteByURL = new SvelteMap(
      remote.servers.map((server) => [serverOrigin(server.url), server])
    );
    const remoteIDs = remote.servers.map((server) => server.id);
    let desiredRemoteHomeID: string | undefined;
    const localServers = [...serverRegistry.servers].sort((a, b) =>
      a.id === serverRegistry.homeServerId ? -1 : b.id === serverRegistry.homeServerId ? 1 : 0
    );
    const localURLs = new Set(localServers.map((server) => serverOrigin(server.url)));

    for (const local of localServers) {
      const origin = serverOrigin(local.url);
      const existing = remoteByURL.get(origin);
      const portable = portableServer(local, existing?.id ?? generateServerId(origin, remoteIDs));
      if (!existing) {
        await api.createKnownServer(portable);
        remoteIDs.push(portable.id);
      } else if (
        existing.url !== portable.url ||
        existing.name !== portable.name ||
        existing.iconUrl !== portable.iconUrl
      ) {
        await api.updateKnownServer(portable);
      }
      if (local.id === serverRegistry.homeServerId) {
        desiredRemoteHomeID = existing?.id ?? portable.id;
      }
      remoteByURL.delete(origin);
    }

    const previousURLs = new Set(this.#directorySnapshots[accountKey] ?? []);
    for (const removed of remoteByURL.values()) {
      if (
        removed.id !== remote.homeServerId &&
        wasRemovedSinceLastSync(serverOrigin(removed.url), previousURLs, localURLs)
      ) {
        await api.deleteKnownServer(removed.id);
      }
    }

    if (desiredRemoteHomeID && remote.homeServerId !== desiredRemoteHomeID) {
      await api.setHomeServer(desiredRemoteHomeID);
    }

    this.#directorySnapshots[accountKey] = serverRegistry.servers.map((server) =>
      serverOrigin(server.url)
    );
    directorySnapshotsSlot.set(this.#directorySnapshots);
  }

  #mergeRemoteServers(
    servers: Array<{ id: string; url: string; name: string; iconUrl?: string }>,
    remoteHomeServerId: string | undefined,
    accountKey: string
  ): string | undefined {
    const remoteURLs = new Set(servers.map((server) => serverOrigin(server.url)));
    const previousURLs = new Set(this.#directorySnapshots[accountKey] ?? []);
    for (const local of [...serverRegistry.servers]) {
      const origin = serverOrigin(local.url);
      if (
        local.id !== serverRegistry.homeServerId &&
        wasRemovedSinceLastSync(origin, previousURLs, remoteURLs)
      ) {
        serverRegistry.removeServer(local.id);
      }
    }

    const remoteToLocal = new SvelteMap<string, string>();
    for (const remote of servers) {
      const origin = serverOrigin(remote.url);
      const existing = serverRegistry.servers.find((server) => serverOrigin(server.url) === origin);
      if (existing) {
        serverRegistry.updateServer(existing.id, portableMetadataUpdate(remote));
        remoteToLocal.set(remote.id, existing.id);
        continue;
      }
      const localID = generateServerId(
        origin,
        serverRegistry.servers.map((server) => server.id)
      );
      serverRegistry.addServer({
        id: localID,
        url: origin,
        name: remote.name,
        iconUrl: remote.iconUrl ?? null,
        token: null,
        userId: null,
        userLogin: null,
        userDisplayName: null,
        userAvatarUrl: null,
        reauthRequiredAt: Date.now(),
        addedAt: Date.now()
      });
      remoteToLocal.set(remote.id, localID);
    }
    return remoteHomeServerId ? remoteToLocal.get(remoteHomeServerId) : undefined;
  }

  #adoptLegacyHomeSettings(homeServerId: string): void {
    if (this.#cache.initialized) return;
    const legacy = serverRegistry.tryGetStore(homeServerId)?.currentUser.user?.settings;
    this.#persist({
      initialized: true,
      locale: getLocale(),
      timezone: legacy?.timezone ?? null,
      timeFormat: legacy?.timeFormat ?? TimeFormat.Auto
    });
  }

  #persist(update: Partial<CachedPersonalSettings>): void {
    Object.assign(this.#cache, update);
    if (this.#activeAccountKey) {
      this.#accountCaches[this.#activeAccountKey] = { ...this.#cache };
      accountCachesSlot.set(this.#accountCaches);
    } else {
      anonymousCacheSlot.set(this.#cache);
    }
  }

  #replaceCache(settings: CachedPersonalSettings, persist = true): void {
    Object.assign(this.#cache, settings);
    if (persist) this.#persist({});
  }

  #activateHomeAccount(homeServerId: string): string | null {
    const accountKey = this.#homeAccountKey(homeServerId, true);
    if (!accountKey) return null;
    this.#activeAccountKey = accountKey;
    this.#replaceCache(this.#accountCaches[accountKey] ?? anonymousCacheSlot.get());
    return accountKey;
  }

  #homeAccountKey(homeServerId: string, bind: boolean): string | null {
    const home = serverRegistry.getServer(homeServerId);
    const store = serverRegistry.tryGetStore(homeServerId);
    const userID = store?.currentUser.user?.id ?? home?.userId;
    if (!home || !userID) return null;
    const origin = serverOrigin(home.url);
    const boundUserID = this.#homeAccountBindings[origin];
    if (boundUserID && boundUserID !== userID) return null;
    if (!boundUserID && bind) {
      this.#homeAccountBindings[origin] = userID;
      homeAccountBindingsSlot.set(this.#homeAccountBindings);
    }
    return `${origin}\n${userID}`;
  }

  #rejectHomeAccount(): void {
    this.loadedHomeServerId = null;
    this.#activeAccountKey = null;
    this.#replaceCache(anonymousCacheSlot.get(), false);
    this.status = 'error';
  }

  async #pointPreviousHomeAt(previousHomeServerId: string, newHomeServerId: string): Promise<void> {
    const previousAPI = this.#api(previousHomeServerId);
    const newHome = serverRegistry.getServer(newHomeServerId);
    if (!previousAPI || !newHome) return;
    const directory = await previousAPI.listKnownServers();
    const newOrigin = serverOrigin(newHome.url);
    let entry = directory.servers.find((server) => serverOrigin(server.url) === newOrigin);
    if (!entry) {
      const ids = directory.servers.map((server) => server.id);
      entry = portableServer(newHome, generateServerId(newOrigin, ids));
      await previousAPI.createKnownServer(entry);
    }
    if (directory.homeServerId !== entry.id) await previousAPI.setHomeServer(entry.id);
  }

  #recordPendingHomeMove(previousHomeServerId: string, newHomeServerId: string): void {
    const previous = serverRegistry.getServer(previousHomeServerId);
    const next = serverRegistry.getServer(newHomeServerId);
    const previousUserId = this.#authenticatedUserID(previousHomeServerId);
    if (!previous || !next || !previousUserId) return;
    const previousOrigin = serverOrigin(previous.url);
    const newOrigin = serverOrigin(next.url);

    this.#pendingHomeMoves = recordPendingHomeMove(
      this.#pendingHomeMoves,
      previousOrigin,
      newOrigin,
      previousUserId
    );
    pendingHomeMovesSlot.set(this.#pendingHomeMoves);
  }

  #authenticatedUserID(serverId: string): string | null {
    const server = serverRegistry.getServer(serverId);
    return serverRegistry.tryGetStore(serverId)?.currentUser.user?.id ?? server?.userId ?? null;
  }

  #scheduleHomeMoveRetry(): void {
    if (this.#homeMoveRetryTimer) return;
    this.#homeMoveRetryTimer = setTimeout(() => {
      this.#homeMoveRetryTimer = null;
      void this.retryPendingHomeMoves();
    }, 30_000);
  }
}

function portableServer(server: RegisteredServer, id = server.id) {
  return {
    id,
    url: serverOrigin(server.url),
    name: server.name,
    iconUrl: server.iconUrl ?? undefined
  };
}

function serverOrigin(url: string): string {
  return canonicalServerOrigin(url);
}

function isSelectableLocale(value: string | undefined): value is Locale {
  return value !== undefined && selectableLocales.includes(value as Locale);
}

function renderTimeFormatToClientSync(value: TimeFormat): ClientSyncTimeFormat {
  if (value === TimeFormat.TwelveHour) return ClientSyncTimeFormat.TIME_FORMAT_12_HOUR;
  if (value === TimeFormat.TwentyFourHour) return ClientSyncTimeFormat.TIME_FORMAT_24_HOUR;
  return ClientSyncTimeFormat.TIME_FORMAT_UNSPECIFIED;
}

function clientSyncTimeFormatToRender(value: ClientSyncTimeFormat | undefined): TimeFormat {
  if (value === ClientSyncTimeFormat.TIME_FORMAT_12_HOUR) return TimeFormat.TwelveHour;
  if (value === ClientSyncTimeFormat.TIME_FORMAT_24_HOUR) return TimeFormat.TwentyFourHour;
  return TimeFormat.Auto;
}

export const clientSync = new ClientSyncState();

/** Wire home-server loading and server-directory reconciliation to the root shell. */
export function useClientSync(): void {
  const candidateFingerprint = $derived(
    JSON.stringify(
      serverRegistry.servers.map((server) => [
        server.id,
        serverRegistry.isAuthenticated(server.id),
        serverRegistry.isClientSyncCapable(server.id)
      ])
    )
  );

  $effect(() => {
    void candidateFingerprint;
    const homeServerId = serverRegistry.homeServerId;
    const homeStore = homeServerId ? serverRegistry.tryGetStore(homeServerId) : undefined;
    if (
      homeServerId &&
      homeStore &&
      !homeStore.serverInfo.loading &&
      homeStore.serverInfo.error === null &&
      !serverRegistry.isClientSyncCapable(homeServerId)
    ) {
      serverRegistry.clearHomeServer();
    }
    serverRegistry.chooseAutomaticHomeServer();
  });

  $effect(() => {
    const homeServerId = serverRegistry.homeServerId;
    const authenticated = homeServerId ? serverRegistry.isAuthenticated(homeServerId) : false;
    const capable = homeServerId ? serverRegistry.isClientSyncCapable(homeServerId) : false;
    if (!homeServerId || !authenticated || !capable) {
      clientSync.reset();
      return;
    }
    void clientSync.load(homeServerId);
  });

  const directoryFingerprint = $derived(
    JSON.stringify(
      serverRegistry.servers.map(({ id, url, name, iconUrl }) => ({ id, url, name, iconUrl }))
    )
  );
  $effect(() => {
    void directoryFingerprint;
    void candidateFingerprint;
    void serverRegistry.homeServerId;
    void clientSync.reconcileDirectory();
    void clientSync.retryPendingHomeMoves();
  });
}
