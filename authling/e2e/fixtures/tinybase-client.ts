import { createMergeableStore, type MergeableStore } from 'tinybase/mergeable-store';
import type { Value } from 'tinybase/store';
import { createCustomSynchronizer, type Synchronizer } from 'tinybase/synchronizers';

interface Client {
  store: MergeableStore;
  synchronizer?: Synchronizer;
  socket?: WebSocket;
  endpoint?: string;
  accessToken?: string;
}

const clients = new Map<string, Client>();
const undefinedMarker = '\uFFFC';

const stringify = (value: unknown): string =>
  JSON.stringify(value, (_key, item) => (item === undefined ? undefinedMarker : item));

const decodeLeaf = (stamp: unknown): void => {
  if (Array.isArray(stamp) && stamp[0] === undefinedMarker) stamp[0] = undefined;
};

const decodeValues = (stamp: unknown): void => {
  if (!Array.isArray(stamp) || !stamp[0] || typeof stamp[0] !== 'object') return;
  Object.values(stamp[0]).forEach(decodeLeaf);
};

const decodeTables = (stamp: unknown): void => {
  if (!Array.isArray(stamp) || !stamp[0] || typeof stamp[0] !== 'object') return;
  for (const table of Object.values(stamp[0])) {
    if (!Array.isArray(table) || !table[0] || typeof table[0] !== 'object') continue;
    for (const row of Object.values(table[0])) {
      if (!Array.isArray(row) || !row[0] || typeof row[0] !== 'object') continue;
      Object.values(row[0]).forEach(decodeLeaf);
    }
  }
};

const decodeBody = (message: number, body: unknown, responseTo?: number): unknown => {
  if (message === 3 && Array.isArray(body)) {
    decodeTables(body[0]);
    decodeValues(body[1]);
  } else if (message === 0 && responseTo === 4 && Array.isArray(body)) {
    decodeTables(body[0]);
  } else if (message === 0 && responseTo === 7) {
    decodeValues(body);
  }
  return body;
};

const connect = async (client: Client, endpointURL?: string, accessToken?: string): Promise<void> => {
  const endpoint = new URL(endpointURL ?? '/data/sync', window.location.href);
  endpoint.protocol = endpoint.protocol === 'https:' ? 'wss:' : 'ws:';
  const socket = accessToken
    ? new WebSocket(endpoint, 'authling.account-data.v1')
    : new WebSocket(endpoint);
  await new Promise<void>((resolve, reject) => {
    socket.addEventListener('open', () => resolve(), { once: true });
    socket.addEventListener('error', () => reject(new Error('WebSocket connection failed')), {
      once: true
    });
  });
  if (accessToken) {
    await new Promise<void>((resolve, reject) => {
      const closed = (): void => reject(new Error('WebSocket authentication failed'));
      socket.addEventListener('close', closed, { once: true });
      socket.addEventListener(
        'message',
        (event) => {
          socket.removeEventListener('close', closed);
          const message = JSON.parse(String(event.data)) as { type?: string };
          if (message.type === 'ready') resolve();
          else reject(new Error('Unexpected WebSocket authentication response'));
        },
        { once: true }
      );
      socket.send(JSON.stringify({ type: 'authenticate', access_token: accessToken }));
    });
  }

  let receive: Parameters<Parameters<typeof createCustomSynchronizer>[2]>[0] = () => {};
  let fail: Parameters<Parameters<typeof createCustomSynchronizer>[2]>[1] = () => {};
  const pending = new Map<string, number>();
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
  socket.addEventListener('close', () => fail(new Error('WebSocket closed')));
  const synchronizer = createCustomSynchronizer(
    client.store,
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
  client.socket = socket;
  client.synchronizer = synchronizer;
  client.endpoint = endpointURL;
  client.accessToken = accessToken;
  await synchronizer.startSync();
};

globalThis.authlingTinyBase = {
  async create(name: string, uniqueId: string): Promise<void> {
    clients.set(name, { store: createMergeableStore(uniqueId) });
  },
  setRow(name: string, tableId: string, rowId: string, row: Record<string, string>): void {
    clients.get(name)?.store.setRow(tableId, rowId, row);
  },
  setValue(name: string, valueId: string, value: Value): void {
    clients.get(name)?.store.setValue(valueId, value);
  },
  delRow(name: string, tableId: string, rowId: string): void {
    clients.get(name)?.store.delRow(tableId, rowId);
  },
  getCell(name: string, tableId: string, rowId: string, cellId: string): unknown {
    return clients.get(name)?.store.getCell(tableId, rowId, cellId);
  },
  getValue(name: string, valueId: string): unknown {
    return clients.get(name)?.store.getValue(valueId);
  },
  syncStats(name: string): { sends: number; receives: number } {
    return clients.get(name)?.synchronizer?.getSynchronizerStats() ?? { sends: 0, receives: 0 };
  },
  hasRow(name: string, tableId: string, rowId: string): boolean {
    return clients.get(name)?.store.hasRow(tableId, rowId) ?? false;
  },
  async connect(name: string): Promise<void> {
    const client = clients.get(name);
    if (!client) throw new Error('missing TinyBase client');
    await connect(client);
  },
  async connectWithAccessToken(name: string, endpoint: string, accessToken: string): Promise<void> {
    const client = clients.get(name);
    if (!client) throw new Error('missing TinyBase client');
    await connect(client, endpoint, accessToken);
  },
  async disconnect(name: string): Promise<void> {
    const client = clients.get(name);
    if (!client?.synchronizer) return;
    await client.synchronizer.destroy().catch(() => undefined);
    client.synchronizer = undefined;
    client.socket = undefined;
  },
  async reconnect(name: string): Promise<void> {
    const client = clients.get(name);
    if (!client) throw new Error('missing TinyBase client');
    if (client.synchronizer) {
      await client.synchronizer.destroy().catch(() => undefined);
    }
    await connect(client, client.endpoint, client.accessToken);
  }
};

declare global {
  // This API exists only inside the Playwright compatibility test bundle.
  var authlingTinyBase: {
    create(name: string, uniqueId: string): Promise<void>;
    setRow(name: string, tableId: string, rowId: string, row: Record<string, string>): void;
    setValue(name: string, valueId: string, value: Value): void;
    delRow(name: string, tableId: string, rowId: string): void;
    getCell(name: string, tableId: string, rowId: string, cellId: string): unknown;
    getValue(name: string, valueId: string): unknown;
    syncStats(name: string): { sends: number; receives: number };
    hasRow(name: string, tableId: string, rowId: string): boolean;
    connect(name: string): Promise<void>;
    connectWithAccessToken(name: string, endpoint: string, accessToken: string): Promise<void>;
    disconnect(name: string): Promise<void>;
    reconnect(name: string): Promise<void>;
  };
}
