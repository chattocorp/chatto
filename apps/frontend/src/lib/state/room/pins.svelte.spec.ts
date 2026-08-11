import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import {
  RealtimeProjectionPinnedMessageAction,
  RealtimeProjectionPinnedMessageChange
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { describe, expect, it, vi } from 'vitest';

import type { PinnedMessagesAPI } from '$lib/api-client/pinnedMessages';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { serverStorageKey } from '$lib/storage/serverStorage';
import { RoomPinsStore } from './pins.svelte';

const pinMarkers = new WeakMap<PinnedMessage, string>();

function pin(marker: string, messageId: string): PinnedMessage {
  const item = new PinnedMessage({
    message: new Message({ id: messageId, roomId: 'R1', body: `body-${messageId}` })
  });
  pinMarkers.set(item, marker);
  return item;
}

function pinPage(
  items: PinnedMessage[],
  totalCount = items.length,
  hasMore = false,
  latestPinMarker = (items[0] && pinMarkers.get(items[0])) ?? ''
) {
  return { items, totalCount, hasMore, latestPinMarker };
}

function makeStore(
  api: PinnedMessagesAPI,
  serverId = 'server-1',
  viewerId = 'viewer-1'
): RoomPinsStore {
  const connection = { getAPI: () => api } as unknown as ServerConnection;
  return new RoomPinsStore(connection, serverId, viewerId, 'R1');
}

describe('RoomPinsStore', () => {
  it('hydrates, tracks unseen pins, and clears the marker when viewed', async () => {
    const api = {
      list: vi.fn().mockResolvedValue(pinPage([pin('P1', 'M1')])),
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
        .mockResolvedValueOnce(pinPage([pin('P2', 'M2')]))
        .mockResolvedValueOnce(pinPage([], 0, false, 'P2')),
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
        messageEventId: 'M2'
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
    const olderPin = pin('P51', 'M51');
    const firstPage = [pin('P1', 'M1')];
    const api = {
      list: vi
        .fn()
        .mockResolvedValueOnce(pinPage(firstPage, 51, true, 'P1'))
        .mockResolvedValueOnce(pinPage(firstPage, 51, true, 'P1')),
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
      list: vi.fn().mockResolvedValue(pinPage([pin('P1', 'M1')])),
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

  it('drops late mutation responses and purges the viewer marker after access revocation', async () => {
    let resolveCreate: (item: PinnedMessage) => void = () => undefined;
    let resolveRemove: () => void = () => undefined;
    const createResult = new Promise<PinnedMessage>((resolve) => {
      resolveCreate = resolve;
    });
    const removeResult = new Promise<void>((resolve) => {
      resolveRemove = resolve;
    });
    const serverId = 'revoked-mutations';
    const viewerId = 'viewer-private';
    const storageKey = serverStorageKey(serverId, `viewer:${viewerId}:room:R1:pinsSeen`);
    const api = {
      list: vi.fn().mockResolvedValue(pinPage([pin('P1', 'M1')])),
      create: vi.fn().mockReturnValue(createResult),
      remove: vi.fn().mockReturnValue(removeResult)
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api, serverId, viewerId);
    const release = store.retain();
    await vi.waitFor(() => expect(store.items).toHaveLength(1));
    store.markSeen();
    expect(localStorage.getItem(storageKey)).toBe('P1');

    const createPromise = store.create('M2');
    const removePromise = store.remove('M1');
    store.reset({ accessRevoked: true });
    resolveCreate(pin('P2', 'M2'));
    resolveRemove();
    await Promise.all([createPromise, removePromise]);

    expect(store.isPinned('M1')).toBe(false);
    expect(store.isPinned('M2')).toBe(false);
    expect(store.hasUnseen).toBe(false);
    expect(localStorage.getItem(storageKey)).toBeNull();
    release();
  });

  it('does not report an offline unpin as a new pin', async () => {
    const serverId = 'offline-unpin';
    const storageKey = serverStorageKey(serverId, 'viewer:viewer-1:room:R1:pinsSeen');
    localStorage.removeItem(storageKey);
    const firstApi = {
      list: vi.fn().mockResolvedValue(pinPage([pin('P2', 'M2'), pin('P1', 'M1')], 2, false, 'P2')),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const firstStore = makeStore(firstApi, serverId);
    const firstRelease = firstStore.retain();
    await vi.waitFor(() => expect(firstStore.items).toHaveLength(2));
    firstStore.markSeen();
    firstRelease();

    const otherViewerStore = makeStore(firstApi, serverId, 'viewer-2');
    const otherViewerRelease = otherViewerStore.retain();
    await vi.waitFor(() => expect(otherViewerStore.items).toHaveLength(2));
    expect(otherViewerStore.hasUnseen).toBe(true);
    otherViewerRelease();

    const afterUnpinApi = {
      list: vi.fn().mockResolvedValue(pinPage([pin('P1', 'M1')], 1, false, 'P2')),
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
    const firstPage = [pin('P1', 'M1')];
    const secondPage = [pin('P2', 'M2')];
    const api = {
      list: vi
        .fn()
        .mockRejectedValueOnce(new Error('initial failure'))
        .mockResolvedValueOnce(pinPage(firstPage, 2, true))
        .mockRejectedValueOnce(new Error('load-more failure'))
        .mockResolvedValueOnce(pinPage(secondPage, 2, false, 'P1')),
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

  it('allows another page load after invalidation supersedes a pending load', async () => {
    let resolveStalePage: (page: ReturnType<typeof pinPage>) => void = () => undefined;
    const stalePage = new Promise<ReturnType<typeof pinPage>>((resolve) => {
      resolveStalePage = resolve;
    });
    const firstPage = [pin('P1', 'M1')];
    const refreshedPage = [pin('P2', 'M2')];
    const finalPage = [pin('P3', 'M3')];
    const api = {
      list: vi
        .fn()
        .mockResolvedValueOnce(pinPage(firstPage, 2, true))
        .mockReturnValueOnce(stalePage)
        .mockResolvedValueOnce(pinPage(refreshedPage, 2, true))
        .mockResolvedValueOnce(pinPage(finalPage, 2, false, 'P2')),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();
    await vi.waitFor(() => expect(store.items).toEqual(firstPage));

    const staleLoad = store.loadMore();
    await vi.waitFor(() => expect(store.isLoadingMore).toBe(true));
    store.applyRealtimeChange(
      new RealtimeProjectionPinnedMessageChange({
        action: RealtimeProjectionPinnedMessageAction.CREATED,
        roomId: 'R1',
        messageEventId: 'M2'
      }),
      'P2'
    );

    await vi.waitFor(() => expect(store.items).toEqual(refreshedPage));
    expect(store.isLoadingMore).toBe(false);
    await store.loadMore();
    expect(store.items).toEqual([...refreshedPage, ...finalPage]);
    expect(api.list).toHaveBeenCalledTimes(4);

    resolveStalePage(pinPage([pin('STALE', 'M-stale')], 2, false));
    await staleLoad;
    expect(store.items).toEqual([...refreshedPage, ...finalPage]);
    release();
  });
});
