import { Timestamp } from '@bufbuild/protobuf';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import {
  RealtimeProjectionPinnedMessageAction,
  RealtimeProjectionPinnedMessageChange
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { describe, expect, it, vi } from 'vitest';

import type { PinnedMessagesAPI } from '$lib/api-client/pinnedMessages';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { RoomPinsStore } from './pins.svelte';

function pin(id: string, messageId: string, seconds: bigint): PinnedMessage {
  return new PinnedMessage({
    id,
    message: new Message({ id: messageId, roomId: 'R1', body: `body-${messageId}` }),
    pinnedAt: new Timestamp({ seconds })
  });
}

function makeStore(api: PinnedMessagesAPI): RoomPinsStore {
  const connection = { getAPI: () => api } as unknown as ServerConnection;
  return new RoomPinsStore(connection, 'server-1', 'R1');
}

describe('RoomPinsStore', () => {
  it('hydrates, tracks unseen pins, and clears the marker when viewed', async () => {
    const api = {
      list: vi
        .fn()
        .mockResolvedValue({
          items: [pin('P1', 'M1', 10n)],
          activeMessageEventIds: ['M1'],
          totalCount: 1,
          hasMore: false
        }),
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
        .mockResolvedValueOnce({
          items: [],
          activeMessageEventIds: [],
          totalCount: 0,
          hasMore: false
        })
        .mockResolvedValueOnce({
          items: [pin('P2', 'M2', 20n)],
          activeMessageEventIds: ['M2'],
          totalCount: 1,
          hasMore: false
        })
        .mockResolvedValueOnce({
          items: [],
          activeMessageEventIds: [],
          totalCount: 0,
          hasMore: false
        }),
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
      })
    );
    await vi.waitFor(() => expect(store.items[0]?.message?.id).toBe('M2'));
    expect(store.hasUnseen).toBe(true);
    store.applyRealtimeChange(
      new RealtimeProjectionPinnedMessageChange({
        action: RealtimeProjectionPinnedMessageAction.DELETED,
        roomId: 'R1',
        messageEventId: 'M2'
      })
    );
    expect(store.items).toEqual([]);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(3));
    expect(store.hasUnseen).toBe(false);
    release();
  });

  it('reloads after an idempotent create without moving an older pin into the first page', async () => {
    const olderPin = pin('P51', 'M51', 1n);
    const firstPage = [pin('P1', 'M1', 100n)];
    const api = {
      list: vi
        .fn()
        .mockResolvedValueOnce({
          items: firstPage,
          activeMessageEventIds: ['M1', 'M51'],
          totalCount: 51,
          hasMore: true
        })
        .mockResolvedValueOnce({
          items: firstPage,
          activeMessageEventIds: ['M1', 'M51'],
          totalCount: 51,
          hasMore: true
        }),
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
      list: vi.fn().mockResolvedValue({
        items: [pin('P1', 'M1', 10n)],
        activeMessageEventIds: ['M1'],
        totalCount: 1,
        hasMore: false
      }),
      create: vi.fn(),
      remove: vi.fn()
    } as unknown as PinnedMessagesAPI;
    const store = makeStore(api);
    const release = store.retain();
    await vi.waitFor(() => expect(store.statusReady).toBe(true));

    store.applyMessageUpdate('M1', new Message({ id: 'M1', roomId: 'R1', body: 'edited' }));

    expect(store.items[0]?.message?.body).toBe('edited');
    release();
  });
});
