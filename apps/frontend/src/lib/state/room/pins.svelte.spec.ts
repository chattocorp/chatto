import { Timestamp } from '@bufbuild/protobuf';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import {
  RealtimeProjectionPinnedMessageAction,
  RealtimeProjectionPinnedMessageChange
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { PinnedMessagesAPI } from '$lib/api-client/pinnedMessages';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { serverStorageKey } from '$lib/storage/serverStorage';
import { RoomPinsStore } from './pins.svelte';

function pin(id: string, messageId: string, seconds: bigint): PinnedMessage {
  return new PinnedMessage({
    id,
    message: new Message({ id: messageId, roomId: 'R1', body: `body-${messageId}` }),
    pinnedAt: new Timestamp({ seconds })
  });
}

function pinPage(
  items: PinnedMessage[],
  totalCount = items.length,
  hasMore = false,
  latestPinEventId = items[0]?.id ?? ''
) {
  return { items, totalCount, hasMore, latestPinEventId };
}

function makeStore(api: PinnedMessagesAPI, serverId = 'server-1'): RoomPinsStore {
  const connection = { getAPI: () => api } as unknown as ServerConnection;
  return new RoomPinsStore(connection, serverId, 'R1');
}

afterEach(() => vi.useRealTimers());

describe('RoomPinsStore', () => {
  it('hydrates, tracks unseen pins, and clears the marker when viewed', async () => {
    const api = {
      list: vi
        .fn()
        .mockResolvedValue(pinPage([pin('P1', 'M1', 10n)])),
      batchGet: vi.fn(),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();
    await vi.waitFor(() => expect(store.items).toHaveLength(1));
    expect(store.hasUnseen).toBe(true);
    store.markSeen();
    expect(store.hasUnseen).toBe(false);
    release();
  });

  it('refreshes on a live pin and removes live unpins without retaining message copies', async () => {
    const api = {
      list: vi
        .fn()
        .mockResolvedValueOnce(pinPage([]))
        .mockResolvedValueOnce(pinPage([pin('P2', 'M2', 20n)]))
        .mockResolvedValueOnce(pinPage([], 0, false, 'P2')),
      batchGet: vi.fn(),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    store.applyRealtimeChange(
      new RealtimeProjectionPinnedMessageChange({
        action: RealtimeProjectionPinnedMessageAction.CREATED,
        roomId: 'R1',
        messageEventId: 'M2',
        pinnedAt: new Timestamp({ seconds: 20n })
      }),
      'P2'
    );
    await vi.waitFor(() => expect(store.items[0]?.message?.id).toBe('M2'));
    expect(store.hasUnseen).toBe(true);
    store.applyRealtimeChange(
      new RealtimeProjectionPinnedMessageChange({
        action: RealtimeProjectionPinnedMessageAction.DELETED,
        roomId: 'R1',
        messageEventId: 'M2'
      }),
      'U2'
    );
    expect(store.items).toEqual([]);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(3));
    expect(store.hasUnseen).toBe(true);
    store.markSeen();
    expect(store.hasUnseen).toBe(false);
    release();
  });

  it('reloads after an idempotent create without moving an older pin into the first page', async () => {
    const olderPin = pin('P51', 'M51', 1n);
    const firstPage = [pin('P1', 'M1', 100n)];
    const api = {
      list: vi
        .fn()
        .mockResolvedValueOnce(pinPage(firstPage, 51, true, 'P1'))
        .mockResolvedValueOnce(pinPage(firstPage, 51, true, 'P1')),
      batchGet: vi.fn(),
      create: vi.fn().mockResolvedValue(olderPin),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();
    await vi.waitFor(() => expect(store.items).toEqual(firstPage));

    await store.create('M51');

    expect(store.isPinned('M51')).toBe(true);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expect(store.items).toEqual(firstPage);
    release();
  });

  it('updates the cached resource when a pinned message changes', async () => {
    const api = {
      list: vi.fn().mockResolvedValue(pinPage([pin('P1', 'M1', 10n)])),
      batchGet: vi.fn(),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();
    await vi.waitFor(() => expect(store.items).toHaveLength(1));

    store.applyMessageUpdate('M1', new Message({ id: 'M1', roomId: 'R1', body: 'edited' }));

    expect(store.items[0]?.message?.body).toBe('edited');
    release();
  });

  it('batches authoritative statuses for currently rendered messages', async () => {
    const api = {
      list: vi.fn().mockRejectedValue(new Error('list unavailable')),
      batchGet: vi.fn().mockResolvedValue([pin('P2', 'M2', 20n)]),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);

    store.ensureStatus('M1');
    store.ensureStatus('M2');
    store.ensureStatus('M2');

    await vi.waitFor(() => expect(api.batchGet).toHaveBeenCalledWith('R1', ['M1', 'M2']));
    expect(store.hasPinStatus('M1')).toBe(true);
    expect(store.isPinned('M1')).toBe(false);
    expect(store.isPinned('M2')).toBe(true);
  });

  it('retries an in-flight status batch invalidated by a realtime pin change', async () => {
    let resolveFirstBatch: (items: PinnedMessage[]) => void = () => undefined;
    const firstBatch = new Promise<PinnedMessage[]>((resolve) => {
      resolveFirstBatch = resolve;
    });
    const api = {
      list: vi.fn(),
      batchGet: vi.fn().mockReturnValueOnce(firstBatch).mockResolvedValueOnce([]),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);

    store.ensureStatus('M1');
    await vi.waitFor(() => expect(api.batchGet).toHaveBeenCalledTimes(1));
    store.applyRealtimeChange(
      new RealtimeProjectionPinnedMessageChange({
        action: RealtimeProjectionPinnedMessageAction.CREATED,
        roomId: 'R1',
        messageEventId: 'M2'
      }),
      'P2'
    );
    resolveFirstBatch([]);

    await vi.waitFor(() => expect(api.batchGet).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(store.hasPinStatus('M1')).toBe(true));
    expect(store.isPinned('M1')).toBe(false);
    expect(store.isPinned('M2')).toBe(true);
  });

  it('drops a late status result after room access is revoked', async () => {
    let resolveBatch: (items: PinnedMessage[]) => void = () => undefined;
    const batch = new Promise<PinnedMessage[]>((resolve) => {
      resolveBatch = resolve;
    });
    const api = {
      list: vi.fn(),
      batchGet: vi.fn().mockReturnValueOnce(batch).mockResolvedValueOnce([]),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);

    for (let index = 1; index <= 101; index++) store.ensureStatus(`M${index}`);
    await vi.waitFor(() => expect(api.batchGet).toHaveBeenCalledTimes(1));
    store.reset({ accessRevoked: true });
    store.reset({ rehydrateRetained: true });
    resolveBatch([pin('P1', 'M1', 10n)]);
    await Promise.resolve();
    await Promise.resolve();

    expect(api.batchGet).toHaveBeenCalledTimes(1);
    expect(store.hasPinStatus('M1')).toBe(false);
    store.ensureStatus('M1');
    await Promise.resolve();
    expect(api.batchGet).toHaveBeenCalledTimes(1);

    store.restoreAfterAccessGrant();
    store.ensureStatus('M1');
    await vi.waitFor(() => expect(api.batchGet).toHaveBeenCalledTimes(2));
  });

  it('retries a rejected status batch after an ordinary realtime invalidation', async () => {
    vi.useFakeTimers();
    let rejectBatch: (error: Error) => void = () => undefined;
    const batch = new Promise<PinnedMessage[]>((_, reject) => {
      rejectBatch = reject;
    });
    const api = {
      list: vi.fn(),
      batchGet: vi.fn().mockReturnValueOnce(batch).mockResolvedValueOnce([]),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);

    store.ensureStatus('M1');
    await vi.advanceTimersByTimeAsync(0);
    store.applyRealtimeChange(
      new RealtimeProjectionPinnedMessageChange({
        action: RealtimeProjectionPinnedMessageAction.CREATED,
        roomId: 'R1',
        messageEventId: 'M2'
      }),
      'P2'
    );
    rejectBatch(new Error('temporary'));
    await vi.advanceTimersByTimeAsync(0);
    expect(api.batchGet).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1_000);
    expect(api.batchGet).toHaveBeenCalledTimes(2);
    expect(store.hasPinStatus('M1')).toBe(true);
  });

  it('retries a transient status lookup failure with capped backoff', async () => {
    vi.useFakeTimers();
    const api = {
      list: vi.fn(),
      batchGet: vi.fn().mockRejectedValueOnce(new Error('temporary')).mockResolvedValueOnce([]),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);

    store.ensureStatus('M1');
    await vi.advanceTimersByTimeAsync(0);
    expect(api.batchGet).toHaveBeenCalledTimes(1);
    store.ensureStatus('M2');
    await vi.advanceTimersByTimeAsync(999);
    expect(api.batchGet).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(api.batchGet).toHaveBeenLastCalledWith('R1', ['M1', 'M2']);
    expect(api.batchGet).toHaveBeenCalledTimes(2);
    expect(store.hasPinStatus('M1')).toBe(true);
    expect(store.isPinned('M1')).toBe(false);
  });

  it('cancels status retries when the final room retain is released', async () => {
    vi.useFakeTimers();
    const api = {
      list: vi.fn().mockResolvedValue(pinPage([])),
      batchGet: vi.fn().mockRejectedValue(new Error('temporary')),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();

    store.ensureStatus('M1');
    await vi.advanceTimersByTimeAsync(0);
    expect(api.batchGet).toHaveBeenCalledTimes(1);
    release();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(api.batchGet).toHaveBeenCalledTimes(1);
  });

  it('does not report an offline unpin as a new pin', async () => {
    const serverId = 'offline-unpin';
    const storageKey = serverStorageKey(serverId, 'room:R1:pinsSeen');
    localStorage.removeItem(storageKey);
    const firstApi = {
      list: vi.fn().mockResolvedValue(
        pinPage([pin('P2', 'M2', 20n), pin('P1', 'M1', 10n)], 2, false, 'P2')
      ),
      batchGet: vi.fn(),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const firstStore = makeStore(firstApi, serverId);
    const firstRelease = firstStore.retain();
    await vi.waitFor(() => expect(firstStore.items).toHaveLength(2));
    firstStore.markSeen();
    firstRelease();

    const afterUnpinApi = {
      list: vi.fn().mockResolvedValue(pinPage([pin('P1', 'M1', 10n)], 1, false, 'P2')),
      batchGet: vi.fn(),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const afterUnpinStore = makeStore(afterUnpinApi, serverId);
    const afterUnpinRelease = afterUnpinStore.retain();
    await vi.waitFor(() => expect(afterUnpinStore.items).toHaveLength(1));

    expect(afterUnpinStore.hasUnseen).toBe(false);
    afterUnpinRelease();
    localStorage.removeItem(storageKey);
  });

  it('retries initial and load-more failures without discarding loaded pins', async () => {
    const firstPage = [pin('P1', 'M1', 10n)];
    const secondPage = [pin('P2', 'M2', 5n)];
    const api = {
      list: vi
        .fn()
        .mockRejectedValueOnce(new Error('initial failure'))
        .mockResolvedValueOnce(pinPage(firstPage, 2, true))
        .mockRejectedValueOnce(new Error('load-more failure'))
        .mockResolvedValueOnce(pinPage(secondPage, 2, false, 'P1')),
      batchGet: vi.fn(),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();
    await vi.waitFor(() => expect(store.error).toBe(true));

    store.retry();
    await vi.waitFor(() => expect(store.items).toEqual(firstPage));
    await store.loadMore();
    expect(store.loadMoreError).toBe(true);
    expect(store.items).toEqual(firstPage);

    await store.loadMore();
    expect(store.items).toEqual([...firstPage, ...secondPage]);
    expect(store.loadMoreError).toBe(false);
    release();
  });
});
