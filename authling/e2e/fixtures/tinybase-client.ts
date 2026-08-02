import { createMergeableStore, type MergeableStore } from 'tinybase/mergeable-store';
import { createCustomSynchronizer, type Synchronizer } from 'tinybase/synchronizers';

interface Client {
  store: MergeableStore;
  synchronizer?: Synchronizer;
  socket?: WebSocket;
}

const clients = new Map<string, Client>();
const undefinedMarker = { __authling_tinybase_undefined: true };

const stringify = (value: unknown): string =>
  JSON.stringify(value, (_key, item) => (item === undefined ? undefinedMarker : item));

const parse = (value: string): unknown =>
  JSON.parse(value, (_key, item) =>
    item &&
    typeof item === 'object' &&
    Object.keys(item).length === 1 &&
    item.__authling_tinybase_undefined === true
      ? undefined
      : item
  );

const connect = async (client: Client): Promise<void> => {
  const endpoint = new URL('/data/sync', window.location.href);
  endpoint.protocol = endpoint.protocol === 'https:' ? 'wss:' : 'ws:';
  const socket = new WebSocket(endpoint);
  await new Promise<void>((resolve, reject) => {
    socket.addEventListener('open', () => resolve(), { once: true });
    socket.addEventListener('error', () => reject(new Error('WebSocket connection failed')), {
      once: true
    });
  });

  let receive: Parameters<Parameters<typeof createCustomSynchronizer>[2]>[0] = () => {};
  let fail: Parameters<Parameters<typeof createCustomSynchronizer>[2]>[1] = () => {};
  socket.addEventListener('message', (event) => {
    const [requestId, message, body] = parse(String(event.data)) as [string | null, number, unknown];
    receive('authling', requestId, message, body);
  });
  socket.addEventListener('close', () => fail(new Error('WebSocket closed')));
  const synchronizer = createCustomSynchronizer(
    client.store,
    (_toClientId, requestId, message, body) =>
      socket.send(stringify([requestId, message, body])),
    (registeredReceive, registeredFail) => {
      receive = registeredReceive;
      fail = registeredFail;
    },
    () => socket.close(),
    2
  );
  client.socket = socket;
  client.synchronizer = synchronizer;
  await synchronizer.startSync();
};

globalThis.authlingTinyBase = {
  async create(name: string, uniqueId: string): Promise<void> {
    clients.set(name, { store: createMergeableStore(uniqueId) });
  },
  setRow(name: string, tableId: string, rowId: string, row: Record<string, string>): void {
    clients.get(name)?.store.setRow(tableId, rowId, row);
  },
  setValue(name: string, valueId: string, value: string): void {
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
  hasRow(name: string, tableId: string, rowId: string): boolean {
    return clients.get(name)?.store.hasRow(tableId, rowId) ?? false;
  },
  async connect(name: string): Promise<void> {
    const client = clients.get(name);
    if (!client) throw new Error('missing TinyBase client');
    await connect(client);
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
    await connect(client);
  }
};

declare global {
  // This API exists only inside the Playwright compatibility test bundle.
  var authlingTinyBase: {
    create(name: string, uniqueId: string): Promise<void>;
    setRow(name: string, tableId: string, rowId: string, row: Record<string, string>): void;
    setValue(name: string, valueId: string, value: string): void;
    delRow(name: string, tableId: string, rowId: string): void;
    getCell(name: string, tableId: string, rowId: string, cellId: string): unknown;
    getValue(name: string, valueId: string): unknown;
    hasRow(name: string, tableId: string, rowId: string): boolean;
    connect(name: string): Promise<void>;
    disconnect(name: string): Promise<void>;
    reconnect(name: string): Promise<void>;
  };
}
