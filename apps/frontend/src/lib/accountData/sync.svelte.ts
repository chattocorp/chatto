import { createMergeableStore, type MergeableStore } from 'tinybase/mergeable-store';
import { createLocalPersister, type LocalPersister } from 'tinybase/persisters/persister-browser';
import { createCustomSynchronizer, type Synchronizer } from 'tinybase/synchronizers';
import { SvelteMap, SvelteSet, SvelteURL } from 'svelte/reactivity';
import { getClientConfiguration } from '$lib/clientConfig';
import { authorizeAccountData, type AccountDataAuthorization } from './authorization';
import { clearPersistedAccountDataSession } from './persistedAuthorization';
import { serverRegistry, type ServerRegistration } from '$lib/state/server/registry.svelte';
import { AuthlingSession, type AuthlingSessionStatus } from '$lib/authling/session.svelte';

const TABLE_ID = 'chattoServers';
const DEVICE_ID_KEY = 'chatto:account-data:device-id';
const STORE_KEY = 'chatto:account-data:tinybase';
const UNDEFINED_MARKER = '\uFFFC';
const MAX_SYNCED_SERVERS = 100;

export type AccountDataSyncStatus = AuthlingSessionStatus;

type PublicServerRow = {
  id: string;
  url: string;
  name: string;
  iconUrl: string;
  addedAt: number;
};

/** Owns the browser's durable TinyBase peer and its Authling connection. */
export class AccountDataSync {
  readonly session = new AuthlingSession();
  get status(): AccountDataSyncStatus {
    return this.session.status;
  }
  get providerLabel(): string | null {
    return this.session.providerLabel;
  }
  get accountId(): string | null {
    return this.session.accountId;
  }
  get error(): string | null {
    return this.session.error;
  }
  #store: MergeableStore | null = null;
  #persister: LocalPersister | null = null;
  #synchronizer: Synchronizer | null = null;
  #socket: WebSocket | null = null;
  #initialized: Promise<void> | null = null;
  #applying = false;
  #catalogSubscribed = false;
  #lifecycleGeneration = 0;

  initialize(): Promise<void> {
    this.#initialized ??= this.#initialize(this.#lifecycleGeneration);
    return this.#initialized;
  }

  async connect(): Promise<void> {
    const generation = this.#lifecycleGeneration;
    await this.initialize();
    if (!this.#isCurrent(generation)) return;
    if (this.status === 'connecting' || this.status === 'connected') return;
    this.session.beginConnecting();
    try {
      const configuration = await getClientConfiguration();
      if (!this.#isCurrent(generation)) return;
      const persisted = configuration.authling
        ? this.session.restore(configuration.authling)
        : null;
      if (persisted) {
        try {
          await this.#connectWithAuthorization(persisted, generation);
          return;
        } catch {
          if (!this.#isCurrent(generation)) return;
          this.session.clearGrant();
          await this.#disconnectTransport();
          await this.#destroyLocalPeer();
          if (!this.#isCurrent(generation)) return;
          clearPersistedAccountDataSession();
          serverRegistry.detachSyncedRegistrations();
        }
      }
      serverRegistry.detachSyncedRegistrations();
      clearPersistedAccountDataSession();
      const authorization = await authorizeAccountData();
      if (!this.#isCurrent(generation)) return;
      await this.#createLocalPeer(generation);
      if (!this.#isCurrent(generation)) return;
      await this.#connectWithAuthorization(authorization, generation);
    } catch (error) {
      if (!this.#isCurrent(generation)) return;
      await this.#disconnectTransport();
      this.session.fail(error);
    }
  }

  /** Clear this frontend's Authling grant and cache without deleting synchronized server data. */
  async signOut(): Promise<void> {
    this.#lifecycleGeneration++;
    await this.#disconnectTransport();
    await this.#destroyLocalPeer();
    clearPersistedAccountDataSession();
    serverRegistry.detachSyncedRegistrations();
    this.session.reset();
    this.#initialized = null;
  }

  async #initialize(generation: number): Promise<void> {
    if (typeof window === 'undefined') return;
    if (!this.#catalogSubscribed) {
      this.#catalogSubscribed = true;
      serverRegistry.subscribeCatalog((change) => {
        if (change === 'public') {
          this.#writeRegistryToStore();
          return;
        }
        this.session.clearGrant();
        void this.#disconnectTransport();
        this.session.transportDisconnected();
      });
    }

    const configuration = await getClientConfiguration();
    if (!this.#isCurrent(generation)) return;
    const authorization = configuration.authling
      ? this.session.restore(configuration.authling)
      : null;
    if (!authorization) {
      clearPersistedAccountDataSession();
      serverRegistry.detachSyncedRegistrations();
      return;
    }
    await this.#createLocalPeer(generation);
    if (!this.#isCurrent(generation)) return;
    this.session.beginConnecting();
    try {
      await this.#connectWithAuthorization(authorization, generation);
    } catch {
      if (!this.#isCurrent(generation)) return;
      await this.#disconnectTransport();
      this.session.transportDisconnected();
    }
  }

  async #createLocalPeer(generation: number): Promise<void> {
    if (this.#store) return;
    const store = createMergeableStore(this.#deviceId());
    this.#store = store;
    this.#persister = createLocalPersister(store, STORE_KEY);
    await this.#persister.startAutoLoad(() => this.#registryContent());
    if (!this.#isCurrent(generation)) return;
    await this.#persister.startAutoSave();
    if (!this.#isCurrent(generation)) return;
    this.#applyStoreToRegistry();
    store.addTableListener(TABLE_ID, () => this.#applyStoreToRegistry());
  }

  async #destroyLocalPeer(): Promise<void> {
    const persister = this.#persister;
    this.#persister = null;
    if (persister) await persister.destroy().catch(() => undefined);
    this.#store = null;
  }

  async #connectWithAuthorization(
    authorization: AccountDataAuthorization,
    generation: number
  ): Promise<void> {
    if (!this.#store) throw new Error('Account-data storage is not ready.');
    await this.#disconnectTransport();
    if (!this.#isCurrent(generation)) return;
    const endpoint = new SvelteURL('/data/sync', authorization.issuer);
    endpoint.protocol = endpoint.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(endpoint, 'authling.account-data.v1');
    this.#socket = socket;
    await waitForOpen(socket);
    if (!this.#isCurrent(generation)) {
      socket.close();
      return;
    }
    await authenticateSocket(socket, authorization.accessToken);
    if (!this.#isCurrent(generation)) {
      socket.close();
      return;
    }

    let receive: Parameters<Parameters<typeof createCustomSynchronizer>[2]>[0] = () => {};
    let fail: Parameters<Parameters<typeof createCustomSynchronizer>[2]>[1] = () => {};
    const pending = new SvelteMap<string, number>();
    socket.addEventListener('message', (event) => {
      const [requestId, message, body] = JSON.parse(String(event.data)) as [
        string | null,
        number,
        unknown
      ];
      const responseTo = requestId === null ? undefined : pending.get(requestId);
      if (message === 0 && requestId !== null) pending.delete(requestId);
      receive('authling', requestId, message, decodeBody(message, body, responseTo));
    });
    socket.addEventListener('close', () => {
      fail(new Error('WebSocket closed'));
      if (this.#socket === socket) {
        this.#socket = null;
        this.#synchronizer = null;
        this.session.transportDisconnected();
      }
    });

    const synchronizer = createCustomSynchronizer(
      this.#store,
      (_toClientId, requestId, message, body) => {
        if (message !== 0 && requestId !== null) pending.set(requestId, message);
        socket.send(stringify([requestId, message, body]));
      },
      (registeredReceive, registeredFail) => {
        receive = registeredReceive;
        fail = registeredFail;
      },
      () => socket.close(),
      2
    );
    this.#synchronizer = synchronizer;
    await synchronizer.startSync();
    if (!this.#isCurrent(generation)) {
      await synchronizer.destroy().catch(() => undefined);
      return;
    }
    this.session.establish(authorization);
  }

  #isCurrent(generation: number): boolean {
    return generation === this.#lifecycleGeneration;
  }

  async #disconnectTransport(): Promise<void> {
    const synchronizer = this.#synchronizer;
    this.#synchronizer = null;
    if (synchronizer) await synchronizer.destroy().catch(() => undefined);
    this.#socket?.close();
    this.#socket = null;
  }

  #registryContent(): [{ [TABLE_ID]: Record<string, PublicServerRow> }, Record<string, never>] {
    return [
      {
        [TABLE_ID]: Object.fromEntries(
          serverRegistry.registrations.map((server) => [server.id, publicServerRow(server)])
        )
      },
      {}
    ];
  }

  #writeRegistryToStore(): void {
    if (!this.#store || this.#applying) return;
    this.#applying = true;
    try {
      const wanted = new SvelteSet(serverRegistry.registrations.map((server) => server.id));
      for (const server of serverRegistry.registrations) {
        this.#store.setRow(TABLE_ID, server.id, publicServerRow(server));
      }
      for (const rowId of this.#store.getRowIds(TABLE_ID)) {
        if (!wanted.has(rowId)) this.#store.delRow(TABLE_ID, rowId);
      }
    } finally {
      this.#applying = false;
    }
  }

  #applyStoreToRegistry(): void {
    if (!this.#store || this.#applying) return;
    this.#applying = true;
    try {
      const rows = this.#store.getTable(TABLE_ID);
      const entries = Object.entries(rows);
      if (entries.length > MAX_SYNCED_SERVERS) return;
      const remoteIds = new SvelteSet(entries.map(([id]) => id));
      for (const [id, raw] of entries) {
        const row = parsePublicServerRow(id, raw);
        if (!row) continue;
        const existing = serverRegistry.getServer(id);
        if (existing) {
          if (new SvelteURL(existing.url).origin !== row.url) continue;
          serverRegistry.updateRegistration(id, {
            name: row.name,
            iconUrl: row.iconUrl || null,
            addedAt: row.addedAt
          });
          continue;
        }
        serverRegistry.addServer({
          id,
          url: row.url,
          name: row.name,
          iconUrl: row.iconUrl || null,
          addedAt: row.addedAt,
          source: 'synced'
        });
      }
      for (const server of [...serverRegistry.registrations]) {
        if (!remoteIds.has(server.id)) serverRegistry.removeServer(server.id);
      }
    } finally {
      this.#applying = false;
    }
  }

  #deviceId(): string {
    let id = localStorage.getItem(DEVICE_ID_KEY);
    if (!id) {
      id = crypto.randomUUID();
      localStorage.setItem(DEVICE_ID_KEY, id);
    }
    return id;
  }
}

/** Serialize only the public catalogue fields permitted to cross the Authling boundary. */
export function publicServerRow(server: ServerRegistration): PublicServerRow {
  return {
    id: server.id,
    url: server.url,
    name: server.name,
    iconUrl: server.iconUrl ?? '',
    addedAt: server.addedAt
  };
}

function parsePublicServerRow(id: string, row: Record<string, unknown>): PublicServerRow | null {
  if (
    !/^[A-Za-z0-9-]{1,128}$/.test(id) ||
    row.id !== id ||
    typeof row.url !== 'string' ||
    row.url.length > 2048 ||
    typeof row.name !== 'string' ||
    row.name.length === 0 ||
    row.name.length > 200 ||
    typeof row.iconUrl !== 'string' ||
    row.iconUrl.length > 2048 ||
    typeof row.addedAt !== 'number' ||
    !Number.isFinite(row.addedAt) ||
    row.addedAt < 0
  ) {
    return null;
  }
  try {
    const url = new URL(row.url);
    if (url.protocol !== 'https:' && url.hostname !== 'localhost') return null;
    let iconUrl = '';
    if (row.iconUrl) {
      const iconURL = new URL(row.iconUrl, url);
      if (iconURL.protocol !== 'https:' && iconURL.hostname !== 'localhost') return null;
      iconUrl = iconURL.toString();
    }
    return { id, url: url.origin, name: row.name, iconUrl, addedAt: row.addedAt };
  } catch {
    return null;
  }
}

function waitForOpen(socket: WebSocket): Promise<void> {
  return new Promise((resolve, reject) => {
    const cleanup = () => {
      socket.removeEventListener('open', opened);
      socket.removeEventListener('error', failed);
      socket.removeEventListener('close', closed);
    };
    const opened = () => {
      cleanup();
      resolve();
    };
    const failed = () => {
      cleanup();
      reject(new Error('WebSocket connection failed.'));
    };
    const closed = () => {
      cleanup();
      reject(new Error('WebSocket closed before connecting.'));
    };
    socket.addEventListener('open', opened, { once: true });
    socket.addEventListener('error', failed, { once: true });
    socket.addEventListener('close', closed, { once: true });
  });
}

function authenticateSocket(socket: WebSocket, accessToken: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const closed = () => reject(new Error('WebSocket authentication failed.'));
    socket.addEventListener('close', closed, { once: true });
    socket.addEventListener(
      'message',
      (event) => {
        socket.removeEventListener('close', closed);
        const message = JSON.parse(String(event.data)) as { type?: string };
        if (message.type === 'ready') resolve();
        else reject(new Error('Unexpected WebSocket authentication response.'));
      },
      { once: true }
    );
    socket.send(JSON.stringify({ type: 'authenticate', access_token: accessToken }));
  });
}

const stringify = (value: unknown): string =>
  JSON.stringify(value, (_key, item) => (item === undefined ? UNDEFINED_MARKER : item));

function decodeLeaf(stamp: unknown): void {
  if (Array.isArray(stamp) && stamp[0] === UNDEFINED_MARKER) stamp[0] = undefined;
}

function decodeValues(stamp: unknown): void {
  if (!Array.isArray(stamp) || !stamp[0] || typeof stamp[0] !== 'object') return;
  Object.values(stamp[0]).forEach(decodeLeaf);
}

function decodeTables(stamp: unknown): void {
  if (!Array.isArray(stamp) || !stamp[0] || typeof stamp[0] !== 'object') return;
  for (const table of Object.values(stamp[0])) {
    if (!Array.isArray(table) || !table[0] || typeof table[0] !== 'object') continue;
    for (const row of Object.values(table[0])) {
      if (!Array.isArray(row) || !row[0] || typeof row[0] !== 'object') continue;
      Object.values(row[0]).forEach(decodeLeaf);
    }
  }
}

function decodeBody(message: number, body: unknown, responseTo?: number): unknown {
  if (message === 3 && Array.isArray(body)) {
    decodeTables(body[0]);
    decodeValues(body[1]);
  } else if (message === 0 && responseTo === 4 && Array.isArray(body)) {
    decodeTables(body[0]);
  } else if (message === 0 && responseTo === 7) {
    decodeValues(body);
  }
  return body;
}

export const accountDataSync = new AccountDataSync();
