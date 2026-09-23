// SPDX-License-Identifier: Apache-2.0

/** Bounded, presentation-only private content saved for offline reading. */
export type SavedMessage = {
  id: string;
  createdAt: string;
  author: string;
  authorId?: string;
  body: string;
};

export type SavedRoom = {
  id: string;
  name: string;
  kind?: number;
  universal?: boolean;
  messages: SavedMessage[];
};

export type SavedView = {
  version: 1;
  serverId: string;
  userId: string;
  viewerName?: string;
  serverName: string;
  savedAt: number;
  rooms: SavedRoom[];
};

const DB_NAME = 'chatto-saved-views';
const STORE_NAME = 'views';
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;
// The application shell uses at most 12 MB of the 20 MB offline budget.
const MAX_TOTAL_BYTES = 8_000_000;
let purgeGeneration = 0;

/** Fence writes captured before a verified private-content change. */
export function invalidateSavedViewWrites(): void {
  purgeGeneration++;
}

type SavedRecord = SavedView & { key: string };

function keyFor(serverId: string, userId: string): string {
  return `${serverId}\u0000${userId}`;
}

function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null);
  return new Promise((resolve) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => {
      request.result.createObjectStore(STORE_NAME, { keyPath: 'key' });
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => resolve(null);
    request.onblocked = () => resolve(null);
  });
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error);
    transaction.onabort = () => reject(transaction.error);
  });
}

function validRecord(value: unknown): value is SavedRecord {
  if (!value || typeof value !== 'object') return false;
  const record = value as Partial<SavedRecord>;
  return record.version === 1 && typeof record.key === 'string' &&
    typeof record.serverId === 'string' && typeof record.userId === 'string' &&
    typeof record.serverName === 'string' &&
    (record.viewerName === undefined || typeof record.viewerName === 'string') &&
    Number.isFinite(record.savedAt) &&
    Array.isArray(record.rooms) && record.rooms.every((room) =>
      typeof room.id === 'string' && typeof room.name === 'string' &&
      (room.kind === undefined || typeof room.kind === 'number') &&
      (room.universal === undefined || typeof room.universal === 'boolean') &&
      Array.isArray(room.messages) && room.messages.every((message) =>
        typeof message.id === 'string' && typeof message.createdAt === 'string' &&
        typeof message.author === 'string' && typeof message.body === 'string' &&
        (message.authorId === undefined || typeof message.authorId === 'string')
      )
    );
}

/** Read only a saved view for the exact local server and user. */
export async function loadSavedView(serverId: string, userId: string | null): Promise<SavedView | null> {
  if (!userId) return null;
  const db = await openDatabase();
  if (!db) return null;
  try {
    const transaction = db.transaction(STORE_NAME, 'readonly');
    const value: unknown = await requestResult(transaction.objectStore(STORE_NAME).get(keyFor(serverId, userId)));
    if (!validRecord(value) || value.serverId !== serverId || value.userId !== userId) return null;
    if (Date.now() - value.savedAt >= MAX_AGE_MS) {
      await clearSavedView(serverId, userId);
      return null;
    }
    const { key: _key, ...view } = value;
    return view;
  } catch {
    return null;
  } finally {
    db.close();
  }
}

/** Save a bounded text snapshot. Storage failure leaves live chat unaffected. */
export async function saveView(view: SavedView): Promise<void> {
  const generation = purgeGeneration;
  if (JSON.stringify(view).length * 2 > MAX_TOTAL_BYTES) return;
  const db = await openDatabase();
  if (!db) return;
  try {
    if (generation !== purgeGeneration) return;
    const transaction = db.transaction(STORE_NAME, 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    const current = (await requestResult(store.getAll()) as unknown[]).filter(validRecord);
    if (current.some((entry) => entry.key === keyFor(view.serverId, view.userId) &&
      entry.savedAt > view.savedAt)) return;
    const entries = current
      .filter((entry) => entry.key !== keyFor(view.serverId, view.userId) &&
        Date.now() - entry.savedAt < MAX_AGE_MS)
      .sort((a, b) => b.savedAt - a.savedAt);
    const selected: SavedRecord[] = [{ ...view, key: keyFor(view.serverId, view.userId) }];
    let total = JSON.stringify(selected[0]).length * 2;
    for (const entry of entries) {
      const bytes = JSON.stringify(entry).length * 2;
      if (total + bytes > MAX_TOTAL_BYTES) continue;
      selected.push(entry);
      total += bytes;
    }
    store.clear();
    for (const entry of selected) store.put(entry);
    await transactionDone(transaction);
  } catch {
    // Private browsing, quota pressure, and blocked storage must not break chat.
  } finally {
    db.close();
  }
}

/** Remove saved private content when a local or verified server boundary occurs. */
export async function clearSavedView(serverId: string, userId?: string): Promise<void> {
  purgeGeneration++;
  const db = await openDatabase();
  if (!db) return;
  try {
    const transaction = db.transaction(STORE_NAME, 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    if (userId) store.delete(keyFor(serverId, userId));
    else {
      const keys = await requestResult(store.getAllKeys());
      for (const key of keys) {
        if (typeof key === 'string' && key.startsWith(`${serverId}\u0000`)) store.delete(key);
      }
    }
    await transactionDone(transaction);
  } catch {
    // A failed purge is retried on the next explicit boundary or cache open.
  } finally {
    db.close();
  }
}

/** Remove every saved private view on this browser profile. */
export async function clearAllSavedViews(): Promise<void> {
  purgeGeneration++;
  const db = await openDatabase();
  if (!db) return;
  try {
    const transaction = db.transaction(STORE_NAME, 'readwrite');
    transaction.objectStore(STORE_NAME).clear();
    await transactionDone(transaction);
  } catch {
    // See clearSavedView.
  } finally {
    db.close();
  }
}
