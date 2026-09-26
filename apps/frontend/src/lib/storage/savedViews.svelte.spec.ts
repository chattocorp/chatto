import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { savedViewFixture } from '$lib/test-utils/savedView';
import {
  clearAllSavedViews,
  clearSavedView,
  invalidateSavedViewWrites,
  loadSavedView,
  saveView,
  snapshotBoundaryTime,
  type SavedView
} from './savedViews';

function openStorage(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open('chatto-saved-views', 2);
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

/** Count the manifest and resource rows stored for one server and user. */
async function storedRows(serverId: string, userId: string): Promise<number> {
  const db = await openStorage();
  try {
    const scope = `${serverId}\u0000${userId}`;
    const transaction = db.transaction(['manifests', 'resources'], 'readonly');
    const count = (request: IDBRequest<number>) =>
      new Promise<number>((resolve, reject) => {
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
      });
    const [manifests, resources] = await Promise.all([
      count(transaction.objectStore('manifests').count(scope)),
      count(transaction.objectStore('resources').index('scope').count(scope))
    ]);
    return manifests + resources;
  } finally {
    db.close();
  }
}

function view(serverId: string, userId: string, savedAt = Date.now()): SavedView {
  return savedViewFixture({
    serverId,
    userId,
    serverName: 'Example',
    savedAt,
    rooms: [
      {
        id: 'room',
        name: 'Room',
        messages: [
          {
            id: 'message',
            createdAt: '2026-09-23T00:00:00Z',
            author: 'Member',
            body: 'Saved text'
          }
        ]
      }
    ]
  });
}

describe('device saved views', () => {
  beforeEach(async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    await clearAllSavedViews();
    vi.setSystemTime(Date.now() + 1);
  });
  afterEach(() => vi.useRealTimers());

  it('loads only the exact server and user and clears that identity', async () => {
    await saveView(view('one', 'alice'));
    await saveView(view('one', 'bob'));
    await saveView(view('two', 'alice'));
    expect((await loadSavedView('one', 'alice'))?.rooms[0].events[0].event).toMatchObject({
      body: 'Saved text'
    });
    expect(await loadSavedView('two', 'bob')).toBeNull();
    await clearSavedView('one', 'alice');
    expect(await loadSavedView('one', 'alice')).toBeNull();
    expect(await loadSavedView('one', 'bob')).not.toBeNull();
    expect(await loadSavedView('two', 'alice')).not.toBeNull();
  });

  it('expires a view seven days after its last sync', async () => {
    await saveView(view('old', 'alice'));
    vi.setSystemTime(Date.now() + 8 * 24 * 60 * 60 * 1000);
    expect(await loadSavedView('old', 'alice')).toBeNull();
    expect(await storedRows('old', 'alice')).toBe(0);
  });

  it('rejects and deletes incomplete legacy and corrupt snapshots', async () => {
    const invalid = view('one', 'alice');
    await saveView(view('two', 'alice'));
    await saveView({ ...invalid, version: 1 } as unknown as SavedView);
    expect(await loadSavedView('one', 'alice')).toBeNull();
    expect(await storedRows('one', 'alice')).toBe(0);
    invalid.rooms[0].resource = '{broken';
    await saveView(invalid);
    expect(await loadSavedView('one', 'alice')).toBeNull();
    expect(await storedRows('one', 'alice')).toBe(0);
    invalid.rooms[0].resource = '{}';
    await saveView(invalid);
    expect(await loadSavedView('one', 'alice')).toBeNull();
    expect(await storedRows('one', 'alice')).toBe(0);
    const event = invalid.rooms[0].events[0] as unknown as { createdAt: unknown };
    invalid.rooms[0].resource = view('one', 'alice').rooms[0].resource;
    event.createdAt = 42;
    await saveView(invalid);
    expect(await loadSavedView('one', 'alice')).toBeNull();
    expect(await storedRows('one', 'alice')).toBe(0);
    expect(await loadSavedView('two', 'alice')).not.toBeNull();
  });

  it('keeps a snapshot that another tab replaced after a rejected read', async () => {
    await saveView({ ...view('one', 'alice'), version: 1 } as unknown as SavedView);
    const other = await openStorage();
    const manifest = await new Promise<Record<string, unknown>>((resolve, reject) => {
      const request = other.transaction('manifests').objectStore('manifests').get('one\u0000alice');
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
    const transaction = IDBDatabase.prototype.transaction;
    // Queue another tab's replacement before the cleanup transaction starts.
    const spy = vi.spyOn(IDBDatabase.prototype, 'transaction').mockImplementation(function (
      this: IDBDatabase,
      ...args: Parameters<IDBDatabase['transaction']>
    ) {
      if (this !== other && args[1] === 'readwrite') {
        spy.mockRestore();
        transaction
          .call(other, 'manifests', 'readwrite')
          .objectStore('manifests')
          .put({ ...manifest, savedAt: Number(manifest.savedAt) + 1 });
      }
      return transaction.apply(this, args);
    });
    try {
      expect(await loadSavedView('one', 'alice')).toBeNull();
    } finally {
      spy.mockRestore();
      other.close();
    }
    expect(await storedRows('one', 'alice')).toBeGreaterThan(1);
  });

  it('discards a disk read started before a private cache boundary', async () => {
    await saveView(view('one', 'alice'));
    const pending = loadSavedView('one', 'alice');
    invalidateSavedViewWrites();

    expect(await pending).toBeNull();
  });

  it('does not replace a newer view with an older tab snapshot', async () => {
    const recent = view('one', 'alice', Date.now());
    const event = recent.rooms[0].events[0].event;
    if (event.kind === 'messagePosted') event.body = 'Current text';
    await saveView(recent);
    await saveView(view('one', 'alice', recent.savedAt - 1_000));
    expect((await loadSavedView('one', 'alice'))?.rooms[0].events[0].event).toMatchObject({
      body: 'Current text'
    });
  });

  it('evicts an oversized timeline while retaining layout and the other resource records', async () => {
    const oversized = view('large', 'alice');
    const event = oversized.rooms[0].events[0].event;
    if (event.kind === 'messagePosted') event.body = 'x'.repeat(11 * 1024 * 1024);
    await saveView(oversized);
    const saved = await loadSavedView('large', 'alice');
    expect(saved?.rooms).toHaveLength(1);
    expect(saved?.rooms[0].timeline).toBeUndefined();
    expect(saved?.rooms[0].events).toEqual([]);
  });

  it('persists every loaded room without a recent-room count or message-row limit', async () => {
    const snapshot = view('many', 'alice');
    snapshot.rooms = Array.from({ length: 15 }, (_, index) => {
      const room = view('many', 'alice').rooms[0];
      room.id = `room-${index}`;
      const resource = JSON.parse(room.resource);
      resource.room.id = room.id;
      room.resource = JSON.stringify(resource);
      room.events = Array.from({ length: 75 }, (_, eventIndex) => ({
        ...room.events[0],
        id: `event-${eventIndex}`
      }));
      return room;
    });
    await saveView(snapshot);
    const saved = await loadSavedView('many', 'alice');
    expect(saved?.rooms).toHaveLength(15);
    expect(saved?.rooms.every((room) => room.events.length === 75)).toBe(true);
    expect(saved?.checkpoint).toBe(snapshot.checkpoint);
  });

  it('rejects stale writers after a purge even when their capture timestamp is newer', async () => {
    const before = view('one', 'alice');
    await saveView(before);
    vi.setSystemTime(Date.now() + 1);
    await clearSavedView('one', 'alice');
    vi.setSystemTime(Date.now() + 1);
    await saveView({ ...before, savedAt: Date.now() });
    expect(await loadSavedView('one', 'alice')).toBeNull();
    await saveView({
      ...before,
      checkpoint: 'reconciled-after-purge',
      checkpointAt: Date.now(),
      savedAt: Date.now()
    });
    expect((await loadSavedView('one', 'alice'))?.checkpoint).toBe('reconciled-after-purge');
  });

  it('accepts the post-purge checkpoint while storage work is delayed', async () => {
    const before = view('delayed-purge', 'alice');
    await saveView(before);
    const purge = clearSavedView('delayed-purge', 'alice');
    // The triggering event can complete in the same millisecond as the purge request.
    const checkpointAt = snapshotBoundaryTime();
    const replacement = saveView({ ...before, checkpoint: 'after-purge', checkpointAt });
    vi.setSystemTime(Date.now() + 1000);
    await Promise.all([purge, replacement]);
    expect((await loadSavedView('delayed-purge', 'alice'))?.checkpoint).toBe('after-purge');
  });

  it('does not weaken a privacy cutoff when the device clock moves backwards', async () => {
    const before = view('clock-change', 'alice');
    vi.setSystemTime(Date.now() + 100);
    await clearSavedView('clock-change', 'alice');
    vi.setSystemTime(Date.now() - 200);
    await clearSavedView('clock-change', 'alice');
    await saveView(before);
    expect(await loadSavedView('clock-change', 'alice')).toBeNull();
  });

  it('retains pagination boundaries, partial membership, and independent thread windows', async () => {
    const snapshot = view('one', 'alice');
    const room = snapshot.rooms[0];
    room.timeline = { startCursor: 'older', endCursor: 'newer', hasNewer: true };
    room.members = { ids: ['member'], totalCount: 20, complete: false, presence: [['member', 1]] };
    room.threads = [
      {
        rootId: 'message',
        events: room.events,
        hasReachedStart: true,
        timeline: { hasNewer: false }
      }
    ];
    await saveView(snapshot);
    const saved = await loadSavedView('one', 'alice');
    expect(saved?.rooms[0].timeline).toEqual(room.timeline);
    expect(saved?.rooms[0].members).toEqual(room.members);
    expect(saved?.rooms[0].threads).toEqual(room.threads);
  });

  it('coalesces queued writes without cancelling a room snapshot when its route closes', async () => {
    const snapshots = Array.from({ length: 12 }, (_, index) => ({
      ...view('one', 'alice'),
      savedAt: Date.now() + index,
      checkpoint: `checkpoint-${index}`
    }));
    await Promise.all(snapshots.map(saveView));
    expect((await loadSavedView('one', 'alice'))?.checkpoint).toBe('checkpoint-11');
  });

  it('rejects and deletes a checkpoint set with a missing resource row', async () => {
    await saveView(view('one', 'alice'));
    const db = await openStorage();
    try {
      await new Promise<void>((resolve, reject) => {
        const transaction = db.transaction('resources', 'readwrite');
        transaction.objectStore('resources').delete('one\u0000alice\u0000timeline:room');
        transaction.oncomplete = () => resolve();
        transaction.onerror = () => reject(transaction.error);
      });
    } finally {
      db.close();
    }
    expect(await loadSavedView('one', 'alice')).toBeNull();
    expect(await storedRows('one', 'alice')).toBe(0);
  });

  it('deletes a database that normal reads cannot open and every other origin database', async () => {
    await saveView(view('one', 'alice'));
    // A newer frontend version left a database version that this code cannot open.
    await new Promise<void>((resolve, reject) => {
      const request = indexedDB.open('chatto-saved-views', 3);
      request.onsuccess = () => {
        request.result.close();
        resolve();
      };
      request.onerror = () => reject(request.error);
    });
    await new Promise<void>((resolve, reject) => {
      const request = indexedDB.open('unrelated-cache');
      request.onsuccess = () => {
        request.result.close();
        resolve();
      };
      request.onerror = () => reject(request.error);
    });
    expect(await loadSavedView('one', 'alice')).toBeNull();

    await clearAllSavedViews({ allDatabases: true });

    const names = (await indexedDB.databases()).map((database) => database.name);
    expect(names).not.toContain('unrelated-cache');
    const stale = view('one', 'alice');
    stale.checkpointAt = Date.now() - 1;
    await saveView(stale);
    expect(await loadSavedView('one', 'alice')).toBeNull();
    await saveView(view('one', 'alice', Date.now() + 1));
    expect(await loadSavedView('one', 'alice')).not.toBeNull();
  });

  it('keeps the previous complete checkpoint when a write fails after deleting old rows', async () => {
    const original = view('one', 'alice');
    await saveView(original);
    const put = IDBObjectStore.prototype.put;
    const fail = vi.spyOn(IDBObjectStore.prototype, 'put').mockImplementation(function (
      this: IDBObjectStore,
      value,
      key
    ) {
      if (this.name === 'resources')
        throw new DOMException('Storage unavailable', 'QuotaExceededError');
      return key === undefined ? put.call(this, value) : put.call(this, value, key);
    });
    try {
      await saveView({ ...original, savedAt: Date.now() + 1, checkpoint: 'incomplete' });
    } finally {
      fail.mockRestore();
    }
    expect((await loadSavedView('one', 'alice'))?.checkpoint).toBe(original.checkpoint);
  });
});
