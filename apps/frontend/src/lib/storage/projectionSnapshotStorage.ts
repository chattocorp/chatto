// SPDX-License-Identifier: Apache-2.0

import { decodePresentation } from './decodeSavedView';
import {
  SAVED_RESOURCE_SCHEMA_VERSION,
  SAVED_VIEW_VERSION,
  snapshotStorageGeneration,
  type SavedRoom,
  type SavedView
} from './savedViews';

const DB_NAME = 'chatto-saved-views';
const STORE_NAME = 'manifests';
const RESOURCE_STORE = 'resources';
const INVALIDATIONS_STORE = 'invalidations';
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;
// The application shell uses at most 12 MB of the 20 MB offline budget.
const MAX_TOTAL_BYTES = 8_000_000;

type SavedRecord = SavedView & { key: string };

type ResourceRecord = {
  key: string;
  scope: string;
  schemaVersion: typeof SAVED_RESOURCE_SCHEMA_VERSION;
  checkpoint: string;
  data: unknown;
};

type Manifest = Omit<SavedRecord, 'rooms' | 'presentation'> & { resources: string[] };

function keyFor(serverId: string, userId: string): string {
  return `${serverId}\u0000${userId}`;
}

function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null);
  return new Promise((resolve) => {
    const request = indexedDB.open(DB_NAME, 2);
    request.onupgradeneeded = () => {
      // V2 presentation caches have no applied checkpoint and cannot seed replay.
      if (request.result.objectStoreNames.contains('views'))
        request.result.deleteObjectStore('views');
      request.result.createObjectStore(STORE_NAME, { keyPath: 'key' });
      request.result
        .createObjectStore(RESOURCE_STORE, { keyPath: 'key' })
        .createIndex('scope', 'scope');
      request.result.createObjectStore(INVALIDATIONS_STORE);
    };
    let settled = false;
    request.onsuccess = () => {
      const db = request.result;
      // A late open after a blocked result has no owner to close it.
      if (settled) {
        db.close();
        return;
      }
      settled = true;
      // Let another tab delete or upgrade the database without waiting for this read.
      db.onversionchange = () => db.close();
      resolve(db);
    };
    request.onerror = () => {
      settled = true;
      resolve(null);
    };
    request.onblocked = () => {
      settled = true;
      resolve(null);
    };
  });
}

/**
 * Delete one database. Resolve when the deletion ends or another connection
 * blocks it; a blocked deletion completes when that connection closes.
 */
function deleteDatabase(name: string): Promise<void> {
  return new Promise((resolve) => {
    const request = indexedDB.deleteDatabase(name);
    request.onsuccess = () => resolve();
    request.onerror = () => resolve();
    request.onblocked = () => resolve();
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
  try {
    if (!value || typeof value !== 'object') return false;
    const record = value as Partial<SavedRecord>;
    const valid =
      record.version === SAVED_VIEW_VERSION &&
      typeof record.checkpoint === 'string' &&
      record.checkpoint.length > 0 &&
      Number.isFinite(record.checkpointAt) &&
      typeof record.key === 'string' &&
      typeof record.serverId === 'string' &&
      typeof record.userId === 'string' &&
      typeof record.serverName === 'string' &&
      (record.viewerName === undefined || typeof record.viewerName === 'string') &&
      Number.isFinite(record.savedAt) &&
      !!record.presentation &&
      (record.presentation.searchStatus === undefined ||
        (Number.isInteger(record.presentation.searchStatus.state) &&
          (record.presentation.searchStatus.retryAfterMs === null ||
            Number.isFinite(record.presentation.searchStatus.retryAfterMs)))) &&
      (record.presentation.serverVersion === undefined ||
        typeof record.presentation.serverVersion === 'string') &&
      (record.presentation.activeCalls === undefined ||
        (Array.isArray(record.presentation.activeCalls) &&
          record.presentation.activeCalls.every((value) => typeof value === 'string'))) &&
      typeof record.presentation.server === 'string' &&
      (record.presentation.viewer === undefined ||
        typeof record.presentation.viewer === 'string') &&
      (record.presentation.runtime === undefined ||
        typeof record.presentation.runtime === 'string') &&
      (record.presentation.motd === undefined || typeof record.presentation.motd === 'string') &&
      Array.isArray(record.presentation.roomGroups) &&
      record.presentation.roomGroups.every((value) => typeof value === 'string') &&
      Array.isArray(record.presentation.users) &&
      record.presentation.users.every((value) => typeof value === 'string') &&
      Array.isArray(record.rooms) &&
      record.rooms.every(
        (room) =>
          typeof room.id === 'string' &&
          typeof room.name === 'string' &&
          typeof room.resource === 'string' &&
          (room.hasReachedStart === undefined || typeof room.hasReachedStart === 'boolean') &&
          Array.isArray(room.events) &&
          (room.timeline === undefined ||
            (typeof room.timeline.hasNewer === 'boolean' &&
              (room.timeline.startCursor === undefined ||
                typeof room.timeline.startCursor === 'string') &&
              (room.timeline.endCursor === undefined ||
                typeof room.timeline.endCursor === 'string'))) &&
          (room.members === undefined ||
            (Array.isArray(room.members.ids) &&
              room.members.ids.every((id) => typeof id === 'string') &&
              Number.isFinite(room.members.totalCount) &&
              typeof room.members.complete === 'boolean' &&
              Array.isArray(room.members.presence) &&
              room.members.presence.every(
                (entry) =>
                  Array.isArray(entry) &&
                  entry.length === 2 &&
                  typeof entry[0] === 'string' &&
                  Number.isInteger(entry[1])
              ))) &&
          (room.kind === undefined || typeof room.kind === 'number') &&
          (room.universal === undefined || typeof room.universal === 'boolean')
      );
    if (!valid) return false;
    decodePresentation(record as SavedView);
    return true;
  } catch {
    return false;
  }
}

/** Split one consistent projection into independently versioned resource records. */
function resourceRecords(view: SavedView): ResourceRecord[] {
  const scope = keyFor(view.serverId, view.userId);
  const record = (id: string, data: unknown): ResourceRecord => ({
    key: `${scope}\u0000${id}`,
    scope,
    schemaVersion: SAVED_RESOURCE_SCHEMA_VERSION,
    checkpoint: view.checkpoint,
    data
  });
  return [
    record(
      'layout',
      view.rooms.map(
        ({
          events: _events,
          members: _members,
          timeline: _timeline,
          hasReachedStart: _start,
          threads: _threads,
          ...room
        }) => room
      )
    ),
    record('shared', view.presentation),
    ...view.rooms.flatMap((room) => [
      ...(room.timeline
        ? [
            record(`timeline:${room.id}`, {
              events: room.events,
              timeline: room.timeline,
              hasReachedStart: room.hasReachedStart
            })
          ]
        : []),
      ...(room.members ? [record(`members:${room.id}`, room.members)] : []),
      ...(room.threads ?? []).map((thread) => record(`thread:${room.id}:${thread.rootId}`, thread))
    ])
  ];
}

/** A manifest lists every required row. Missing/mismatched rows invalidate the checkpoint. */
function assemble(manifest: Manifest, records: ResourceRecord[]): SavedRecord | null {
  const rows = new Map(records.map((row) => [row.key, row]));
  if (
    !Array.isArray(manifest.resources) ||
    manifest.resources.length !== records.length ||
    new Set(manifest.resources).size !== records.length ||
    manifest.resources.some((key) => {
      const row = rows.get(key);
      return (
        !row ||
        row.scope !== manifest.key ||
        row.schemaVersion !== SAVED_RESOURCE_SCHEMA_VERSION ||
        row.checkpoint !== manifest.checkpoint
      );
    })
  )
    return null;
  const data = (id: string) => rows.get(`${manifest.key}\u0000${id}`)?.data;
  const layout = data('layout');
  if (!Array.isArray(layout) || !data('shared')) return null;
  const rooms = layout.map((room) => ({
    ...room,
    events: [],
    ...(data(`timeline:${room.id}`) as Partial<SavedRoom> | undefined),
    members: data(`members:${room.id}`),
    threads: records
      .filter((row) => row.key.startsWith(`${manifest.key}\u0000thread:${room.id}:`))
      .map((row) => row.data)
  }));
  const value = { ...manifest, rooms, presentation: data('shared') };
  return validRecord(value) ? value : null;
}

async function deleteResources(store: IDBObjectStore, scope: string): Promise<void> {
  const keys = await requestResult(store.index('scope').getAllKeys(scope));
  for (const key of keys) store.delete(key);
}

/**
 * Validate a stored set for the exact server and user. Returns null for any
 * incompatible, incomplete, or corrupt set.
 */
function decodeSnapshot(
  manifest: Manifest,
  records: ResourceRecord[],
  serverId: string,
  userId: string,
  schemas: typeof import('./presentationSnapshot')
): SavedView | null {
  try {
    const value = assemble(manifest, records);
    if (!value || value.serverId !== serverId || value.userId !== userId) return null;
    const { timelineSnapshotSchema, notificationSnapshotSchema } = schemas;
    for (const room of value.rooms) room.events = timelineSnapshotSchema.parse(room.events);
    for (const room of value.rooms)
      for (const thread of room.threads ?? []) {
        if (
          typeof thread.rootId !== 'string' ||
          !thread.rootId ||
          typeof thread.hasReachedStart !== 'boolean' ||
          typeof thread.timeline?.hasNewer !== 'boolean'
        )
          return null;
        if (
          thread.timeline.startCursor !== undefined &&
          typeof thread.timeline.startCursor !== 'string'
        )
          return null;
        if (
          thread.timeline.endCursor !== undefined &&
          typeof thread.timeline.endCursor !== 'string'
        )
          return null;
        thread.events = timelineSnapshotSchema.parse(thread.events);
      }
    if (value.presentation.notifications) {
      value.presentation.notifications = notificationSnapshotSchema.parse(
        value.presentation.notifications
      );
    }
    const { key: _key, ...view } = value;
    return view;
  } catch {
    return null;
  }
}

/**
 * Delete the set that a read rejected. This is not a privacy boundary, so it
 * records no invalidation cutoff. The set stays if another tab replaced its
 * manifest after the read.
 */
async function discardSnapshot(
  db: IDBDatabase,
  scope: string,
  manifest: Manifest | undefined
): Promise<void> {
  const transaction = db.transaction([STORE_NAME, RESOURCE_STORE], 'readwrite');
  const store = transaction.objectStore(STORE_NAME);
  const current = await requestResult<Manifest | undefined>(store.get(scope));
  if (
    !Object.is(current?.savedAt, manifest?.savedAt) ||
    !Object.is(current?.checkpoint, manifest?.checkpoint)
  )
    return;
  store.delete(scope);
  await deleteResources(transaction.objectStore(RESOURCE_STORE), scope);
  await transactionDone(transaction);
}

/**
 * Read only a saved view for the exact local server and user. A read deletes
 * an expired, incompatible, incomplete, or corrupt set, so the next startup
 * does not read and reject it again.
 */
export async function loadSavedView(
  serverId: string,
  userId: string | null,
  generation: number
): Promise<SavedView | null> {
  if (!userId) return null;
  const db = await openDatabase();
  if (!db) return null;
  try {
    const transaction = db.transaction([STORE_NAME, RESOURCE_STORE], 'readonly');
    const scope = keyFor(serverId, userId);
    const [manifest, records] = await Promise.all([
      requestResult<Manifest | undefined>(transaction.objectStore(STORE_NAME).get(scope)),
      requestResult<ResourceRecord[]>(
        transaction.objectStore(RESOURCE_STORE).index('scope').getAll(scope)
      )
    ]);
    if (generation !== snapshotStorageGeneration.value) return null;
    if (!manifest) {
      // Resource rows without a manifest can never assemble.
      if (records.length > 0) await discardSnapshot(db, scope, undefined);
      return null;
    }
    // Check the age first. An expired set, or one without a valid save time,
    // does not need the costly validation.
    if (!(Date.now() - manifest.savedAt < MAX_AGE_MS)) {
      await discardSnapshot(db, scope, manifest);
      return null;
    }
    // The validator is needed only when a disk record exists. Keep its schema
    // library out of first-visit and login route bundles. A failed chunk load
    // throws here and keeps the set, because the set itself was not rejected.
    const schemas = await import('./presentationSnapshot');
    const view = decodeSnapshot(manifest, records, serverId, userId, schemas);
    if (generation !== snapshotStorageGeneration.value) return null;
    if (!view) await discardSnapshot(db, scope, manifest);
    return view;
  } catch {
    return null;
  } finally {
    db.close();
  }
}

type PendingWrite = { view: SavedView; generation: number; complete: (() => void)[] };
const pendingWrites = new Map<string, PendingWrite>();
const writingScopes = new Set<string>();

/** Coalesce writes while one transaction is running; navigation never cancels a save. */
export function saveView(view: SavedView, generation: number): Promise<void> {
  const scope = keyFor(view.serverId, view.userId);
  return new Promise((resolve) => {
    const pending = pendingWrites.get(scope);
    pendingWrites.set(scope, {
      view:
        pending && pending.generation === generation && pending.view.savedAt > view.savedAt
          ? pending.view
          : view,
      generation,
      complete: [...(pending?.complete ?? []), resolve]
    });
    if (!writingScopes.has(scope)) void drainWrites(scope);
  });
}

async function drainWrites(scope: string): Promise<void> {
  writingScopes.add(scope);
  try {
    let next: PendingWrite | undefined;
    while ((next = pendingWrites.get(scope))) {
      pendingWrites.delete(scope);
      try {
        if (next.generation === snapshotStorageGeneration.value)
          await writeView(next.view, next.generation);
      } catch {
        // Serialization and storage failures must not interrupt live state or
        // prevent the next valid snapshot from being written.
      } finally {
        for (const complete of next.complete) complete();
      }
    }
  } finally {
    writingScopes.delete(scope);
  }
}

/** Commit resource data and its replay frontier together. Storage is best effort. */
async function writeView(view: SavedView, generation: number): Promise<void> {
  const scope = keyFor(view.serverId, view.userId);
  let records = resourceRecords(view);
  // The byte budget evicts optional windows, never an arbitrary room count.
  // An omitted owner will perform a normal fresh read when opened.
  let bytes = JSON.stringify(records).length * 2;
  const optional = records
    .slice(2)
    .map((row) => ({ row, bytes: JSON.stringify(row).length * 2 }))
    .sort((a, b) => b.bytes - a.bytes);
  for (const candidate of optional) {
    if (bytes <= MAX_TOTAL_BYTES) break;
    records = records.filter((row) => row !== candidate.row);
    bytes -= candidate.bytes;
  }
  if (bytes > MAX_TOTAL_BYTES) return;
  const db = await openDatabase();
  if (!db) return;
  let transaction: IDBTransaction | undefined;
  try {
    if (generation !== snapshotStorageGeneration.value) return;
    transaction = db.transaction([STORE_NAME, RESOURCE_STORE, INVALIDATIONS_STORE], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    const resources = transaction.objectStore(RESOURCE_STORE);
    const invalidations = transaction.objectStore(INVALIDATIONS_STORE);
    // A later capture time must not let a stale tab undo a privacy purge. Only
    // a reconciliation barrier accepted after the purge can establish a new copy.
    const cutoffs = await Promise.all(
      ['all', `server:${view.serverId}`, scope].map((key) =>
        requestResult<number | undefined>(invalidations.get(key))
      )
    );
    if (cutoffs.some((cutoff) => cutoff !== undefined && view.checkpointAt <= cutoff)) {
      transaction.abort();
      return;
    }
    const current = await requestResult<Manifest[]>(store.getAll());
    if (
      current.some(
        (entry) => entry.key === keyFor(view.serverId, view.userId) && entry.savedAt > view.savedAt
      )
    )
      return;
    const entries = current
      .filter(
        (entry) =>
          entry.key !== keyFor(view.serverId, view.userId) &&
          Date.now() - entry.savedAt < MAX_AGE_MS
      )
      .sort((a, b) => b.savedAt - a.savedAt);
    const { rooms: _rooms, presentation: _presentation, ...header } = view;
    const manifest: Manifest = { ...header, key: scope, resources: records.map((row) => row.key) };
    const selected = new Set([scope]);
    let total = JSON.stringify(records).length * 2;
    for (const entry of entries) {
      const previousRecords = await requestResult(resources.index('scope').getAll(entry.key));
      const bytes = JSON.stringify(previousRecords).length * 2;
      if (total + bytes > MAX_TOTAL_BYTES) continue;
      selected.add(entry.key);
      total += bytes;
    }
    for (const entry of current) {
      if (selected.has(entry.key)) continue;
      store.delete(entry.key);
      await deleteResources(resources, entry.key);
    }
    await deleteResources(resources, scope);
    for (const record of records) resources.put(record);
    store.put(manifest);
    await transactionDone(transaction);
  } catch {
    // A synchronous put/serialization failure must roll back earlier deletes
    // too. Catching the exception alone would let the partial transaction commit.
    try {
      transaction?.abort();
    } catch {
      /* Already completed or aborted. */
    }
  } finally {
    db.close();
  }
}

/** Clock adjustments must not weaken an earlier privacy boundary. */
async function advanceInvalidation(
  store: IDBObjectStore,
  key: string,
  cutoff: number
): Promise<void> {
  const previous = await requestResult<number | undefined>(store.get(key));
  store.put(Math.max(previous ?? 0, cutoff), key);
}

/** Remove saved private content when a local or verified server boundary occurs. */
export async function clearSavedView(
  serverId: string,
  userId: string | undefined,
  cutoff: number
): Promise<void> {
  const db = await openDatabase();
  if (!db) return;
  try {
    const transaction = db.transaction(
      [STORE_NAME, RESOURCE_STORE, INVALIDATIONS_STORE],
      'readwrite'
    );
    const store = transaction.objectStore(STORE_NAME);
    const resources = transaction.objectStore(RESOURCE_STORE);
    await advanceInvalidation(
      transaction.objectStore(INVALIDATIONS_STORE),
      userId ? keyFor(serverId, userId) : `server:${serverId}`,
      cutoff
    );
    if (userId) {
      store.delete(keyFor(serverId, userId));
      await deleteResources(resources, keyFor(serverId, userId));
    } else {
      const keys = await requestResult(store.getAllKeys());
      for (const key of keys) {
        if (typeof key === 'string' && key.startsWith(`${serverId}\u0000`)) {
          store.delete(key);
          await deleteResources(resources, key);
        }
      }
    }
    await transactionDone(transaction);
  } catch {
    // A failed purge is retried on the next explicit boundary or cache open.
  } finally {
    db.close();
  }
}

/**
 * Remove every saved private view on this browser profile. This deletes the
 * database, so it also removes a database that this code cannot open, for
 * example one from a newer frontend version. With `allDatabases`, it deletes
 * every IndexedDB database of this origin. A new database then records the
 * device-wide cutoff, so a stale tab cannot write older data again.
 */
export async function clearAllSavedViews(
  cutoff: number,
  { allDatabases = false }: { allDatabases?: boolean } = {}
): Promise<void> {
  if (typeof indexedDB === 'undefined') return;
  const names = new Set([DB_NAME]);
  if (allDatabases) {
    try {
      for (const { name } of await indexedDB.databases()) if (name) names.add(name);
    } catch {
      // Without a database list, delete only the known database.
    }
  }
  await Promise.all([...names].map(deleteDatabase));
  const db = await openDatabase();
  if (!db) return;
  try {
    const transaction = db.transaction(
      [STORE_NAME, RESOURCE_STORE, INVALIDATIONS_STORE],
      'readwrite'
    );
    transaction.objectStore(STORE_NAME).clear();
    transaction.objectStore(RESOURCE_STORE).clear();
    await advanceInvalidation(transaction.objectStore(INVALIDATIONS_STORE), 'all', cutoff);
    await transactionDone(transaction);
  } catch {
    // See clearSavedView.
  } finally {
    db.close();
  }
}
