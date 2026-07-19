import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Message, MessageAttachment } from '@chatto/api-types/api/v1/message_types_pb';
import {
  RoomMessagePosted,
  RoomTimelineEvent
} from '@chatto/api-types/api/v1/room_timeline_pb';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import { RoomFilesStore, type RoomFileItem } from './files.svelte';

const attachmentMocks = vi.hoisted(() => ({
  listRoomAttachments: vi.fn(),
  refreshAssetUrls: vi.fn()
}));

vi.mock('$lib/api-client/attachments', async (importActual) => {
  const actual = await importActual<typeof import('$lib/api-client/attachments')>();
  return {
    ...actual,
    createAttachmentAPI: vi.fn(() => attachmentMocks)
  };
});

function serverConnection(): ServerConnection {
  return {
    serverId: 'test-server',
    connectBaseUrl: 'https://chat.example.test/api/connect',
    bearerToken: 'test-token'
  } as ServerConnection;
}

function roomFileItem(attachmentId = 'att-1', messageEventId = 'event-1'): RoomFileItem {
  return {
    messageEventId,
    threadRootEventId: null,
    createdAt: '2026-07-03T12:00:00.000Z',
    attachment: {
      id: attachmentId,
      filename: `${attachmentId}.jpg`,
      contentType: 'image/jpeg',
      width: 800,
      height: 600,
      assetUrl: {
        url: `/assets/files/${attachmentId}?stale=1`,
        expiresAt: '2026-07-03T13:00:00.000Z'
      },
      thumbnailAssetUrl: {
        url: `/assets/files/${attachmentId}/image/120x120/cover?stale=1`,
        expiresAt: '2026-07-03T13:00:00.000Z'
      },
      videoProcessing: null
    }
  };
}

function timelineMessage(
  eventId: string,
  attachmentIds: string[],
  createdAt = new Date('2026-07-03T12:00:00.000Z')
): RoomTimelineEvent {
  return new RoomTimelineEvent({
    id: eventId,
    createdAt: Timestamp.fromDate(createdAt),
    event: {
      case: 'messagePosted',
      value: new RoomMessagePosted({
        message: new Message({
          roomId: 'room-1',
          attachments: attachmentIds.map(
            (id) =>
              new MessageAttachment({
                id,
                filename: `${id}.jpg`,
                contentType: 'image/jpeg',
                width: 800,
                height: 600
              })
          )
        })
      })
    }
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe('RoomFilesStore', () => {
  beforeEach(() => {
    attachmentMocks.listRoomAttachments.mockReset();
    attachmentMocks.refreshAssetUrls.mockReset();
    attachmentMocks.listRoomAttachments.mockResolvedValue({
      items: [],
      totalCount: 0,
      hasMore: false
    });
    attachmentMocks.refreshAssetUrls.mockResolvedValue(new Map());
  });

  it('does not fall back to stale file URLs after refreshed URLs are cleared', () => {
    const store = new RoomFilesStore(serverConnection(), 'room-1');
    const item = roomFileItem();
    store.items = [item];
    store.refreshedAttachmentUrls.set('att-1', {
      assetUrl: null,
      thumbnailAssetUrl: null,
      videoThumbnailAssetUrl: null,
      variantAssetUrls: new Map()
    });

    expect(store.assetUrlFor(item)).toBeNull();
    expect(store.thumbnailAssetUrlFor(item)).toBeNull();
    expect(store.nextAssetUrlRefreshAt).toBeNull();
  });

  it('starts empty and loads only when hydrated', async () => {
    const store = new RoomFilesStore(serverConnection(), 'room-1');

    expect(store.items).toEqual([]);
    expect(attachmentMocks.listRoomAttachments).not.toHaveBeenCalled();

    await store.hydrate();

    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledOnce();
  });

  it('hydrates a room only once', async () => {
    const store = new RoomFilesStore(serverConnection(), 'room-1');

    await store.hydrate();
    await store.hydrate();

    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledOnce();
  });

  it('adds attachments from new realtime messages without refetching', async () => {
    const store = new RoomFilesStore(serverConnection(), 'room-1');
    await store.hydrate();

    store.applyTimelineEvent(timelineMessage('event-1', ['att-1']), 'event-1');

    expect(store.items.map((item) => item.attachment.id)).toEqual(['att-1']);
    expect(store.totalCount).toBe(1);
    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledOnce();
  });

  it('replaces and removes attachments for loaded messages', async () => {
    attachmentMocks.listRoomAttachments.mockResolvedValue({
      items: [roomFileItem()],
      totalCount: 1,
      hasMore: false
    });
    const store = new RoomFilesStore(serverConnection(), 'room-1');
    await store.hydrate();

    store.applyTimelineEvent(timelineMessage('event-1', ['att-2']), 'edit-1');
    expect(store.items.map((item) => item.attachment.id)).toEqual(['att-2']);

    store.applyTimelineEvent(timelineMessage('event-1', []), 'edit-2');
    expect(store.items).toEqual([]);
    expect(store.totalCount).toBe(0);
  });

  it('keeps realtime updates out of rooms that have never been hydrated', () => {
    const store = new RoomFilesStore(serverConnection(), 'room-1');

    store.applyTimelineEvent(timelineMessage('event-1', ['att-1']), 'event-1');

    expect(store.items).toEqual([]);
    expect(attachmentMocks.listRoomAttachments).not.toHaveBeenCalled();
  });

  it('refetches after an update races the initial hydration', async () => {
    const pending = deferred<{ items: RoomFileItem[]; totalCount: number; hasMore: boolean }>();
    attachmentMocks.listRoomAttachments
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce({ items: [roomFileItem()], totalCount: 1, hasMore: false });
    const store = new RoomFilesStore(serverConnection(), 'room-1');

    const hydration = store.hydrate();
    await vi.waitFor(() => expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledOnce());
    store.applyTimelineEvent(timelineMessage('event-1', ['att-1']), 'event-1');
    pending.resolve({ items: [], totalCount: 0, hasMore: false });
    await hydration;

    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    expect(store.items.map((item) => item.attachment.id)).toEqual(['att-1']);
  });

  it('allows pagination after a reset fences a pending page request', async () => {
    const pendingPage = deferred<{
      items: RoomFileItem[];
      totalCount: number;
      hasMore: boolean;
    }>();
    attachmentMocks.listRoomAttachments
      .mockResolvedValueOnce({ items: [roomFileItem()], totalCount: 2, hasMore: true })
      .mockReturnValueOnce(pendingPage.promise)
      .mockResolvedValueOnce({
        items: [roomFileItem('att-2', 'event-2')],
        totalCount: 2,
        hasMore: false
      });
    const store = new RoomFilesStore(serverConnection(), 'room-1');
    await store.hydrate();

    const staleLoad = store.loadMore();
    await vi.waitFor(() => expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2));
    store.reset();
    pendingPage.resolve({ items: [], totalCount: 2, hasMore: true });
    await staleLoad;
    await store.hydrate();

    expect(store.items.map((item) => item.attachment.id)).toEqual(['att-2']);
  });

  it('coalesces asset IDs queued during a URL refresh', async () => {
    const firstRefresh = deferred<Map<string, RefreshedAttachmentUrls>>();
    const secondRefresh = deferred<Map<string, RefreshedAttachmentUrls>>();
    attachmentMocks.refreshAssetUrls
      .mockReturnValueOnce(firstRefresh.promise)
      .mockReturnValueOnce(secondRefresh.promise);
    const store = new RoomFilesStore(serverConnection(), 'room-1');
    await store.hydrate();
    const firstItem = roomFileItem();
    const secondItem = roomFileItem('att-2', 'event-2');
    store.items = [firstItem, secondItem];

    const refreshFirst = store.refreshUrlsForItem(firstItem);
    await vi.waitFor(() => expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledOnce());
    const refreshSecond = store.refreshUrlsForItem(secondItem);
    firstRefresh.resolve(new Map());
    await vi.waitFor(() => expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledTimes(2));
    expect(attachmentMocks.refreshAssetUrls.mock.calls[1]?.[1]).toEqual(['att-2']);
    secondRefresh.resolve(new Map());

    await Promise.all([refreshFirst, refreshSecond]);
  });
});
