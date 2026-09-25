import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { savedViewFixture } from '$lib/test-utils/savedView';
import {
  clearAllSavedViews,
  clearSavedView,
  invalidateSavedViewWrites,
  loadSavedView,
  saveView,
  type SavedView
} from './savedViews';

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
  });

  it('rejects incomplete legacy and corrupt snapshots', async () => {
    const invalid = view('one', 'alice');
    await saveView({ ...invalid, version: 1 } as unknown as SavedView);
    expect(await loadSavedView('one', 'alice')).toBeNull();
    invalid.rooms[0].resource = '{broken';
    await saveView(invalid);
    expect(await loadSavedView('one', 'alice')).toBeNull();
    invalid.rooms[0].resource = '{}';
    await saveView(invalid);
    expect(await loadSavedView('one', 'alice')).toBeNull();
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

  it('rejects a checkpoint set with a missing resource row', async () => {
    await saveView(view('one', 'alice'));
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('chatto-saved-views', 2);
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
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
