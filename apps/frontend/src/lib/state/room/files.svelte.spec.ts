import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import { RoomFilesStore, type RoomFileItem } from './files.svelte';

const attachmentMocks = vi.hoisted(() => ({
  listRoomAttachments: vi.fn(),
  refreshAssetUrls: vi.fn()
}));

vi.mock('$lib/api-client/attachments', () => ({
  createAttachmentAPI: vi.fn(() => attachmentMocks)
}));

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
      filename: 'image.jpg',
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
    const store = new RoomFilesStore(serverConnection());
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

  it('does not load files until the panel is activated', async () => {
    const store = new RoomFilesStore(serverConnection());
    store.selectRoom('room-1');

    expect(attachmentMocks.listRoomAttachments).not.toHaveBeenCalled();

    store.activate();

    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });
  });

  it('refreshes invalidated files immediately while active', async () => {
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });

    store.invalidate('room-1');

    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
  });

  it('defers invalidated file refreshes while dormant until the panel reopens', async () => {
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });

    store.deactivate();
    store.invalidate('room-1');
    await Promise.resolve();
    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);

    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
  });

  it('ignores invalidations for another room', async () => {
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });

    store.invalidate('room-2');
    await Promise.resolve();

    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
  });

  it('reloads a previously visited room after another room was selected', async () => {
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });

    store.deactivate();
    store.selectRoom('room-2');
    store.selectRoom('room-1');
    store.activate();

    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
  });

  it('ignores a list response that arrives after the panel closes', async () => {
    const pending = deferred<{
      items: RoomFileItem[];
      totalCount: number;
      hasMore: boolean;
    }>();
    attachmentMocks.listRoomAttachments
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValue({ items: [], totalCount: 0, hasMore: false });
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });

    store.deactivate();
    pending.resolve({ items: [roomFileItem()], totalCount: 1, hasMore: false });
    await Promise.resolve();
    await Promise.resolve();

    expect(store.items).toEqual([]);
    store.activate();
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
  });

  it('coalesces realtime invalidations while a replacement is in flight', async () => {
    const pending = deferred<{ items: RoomFileItem[]; totalCount: number; hasMore: boolean }>();
    attachmentMocks.listRoomAttachments
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValue({ items: [], totalCount: 0, hasMore: false });
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });

    store.invalidate('room-1');
    store.invalidate('room-1');
    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);

    pending.resolve({ items: [], totalCount: 0, hasMore: false });
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
  });

  it('allows pagination after the panel closes during a pending page request', async () => {
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
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => expect(store.hasMore).toBe(true));

    const staleLoad = store.loadMore();
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
    store.deactivate();
    expect(store.isLoadingMore).toBe(false);
    store.activate();

    pendingPage.resolve({ items: [], totalCount: 2, hasMore: true });
    await staleLoad;
    await store.loadMore();

    expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(3);
    expect(store.items.map((item) => item.attachment.id)).toEqual(['att-1', 'att-2']);
  });

  it('refreshes asset IDs queued while another URL refresh is pending', async () => {
    const firstRefresh = deferred<Map<string, RefreshedAttachmentUrls>>();
    const secondRefresh = deferred<Map<string, RefreshedAttachmentUrls>>();
    attachmentMocks.refreshAssetUrls
      .mockReturnValueOnce(firstRefresh.promise)
      .mockReturnValueOnce(secondRefresh.promise);
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });
    const firstItem = roomFileItem();
    const secondItem = roomFileItem('att-2', 'event-2');
    store.items = [firstItem, secondItem];

    const refreshFirst = store.refreshUrlsForItem(firstItem);
    await vi.waitFor(() => {
      expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledTimes(1);
    });
    const refreshSecond = store.refreshUrlsForItem(secondItem);
    firstRefresh.resolve(new Map());
    await vi.waitFor(() => {
      expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledTimes(2);
    });
    expect(attachmentMocks.refreshAssetUrls.mock.calls[1]?.[1]).toEqual(['att-2']);
    secondRefresh.resolve(new Map());

    await Promise.all([refreshFirst, refreshSecond]);
  });

  it('ignores refreshed URLs that arrive after the panel closes', async () => {
    const pending = deferred<Map<string, RefreshedAttachmentUrls>>();
    attachmentMocks.refreshAssetUrls.mockReturnValueOnce(pending.promise);
    const store = new RoomFilesStore(serverConnection());
    store.activate('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
    });
    const item = roomFileItem();
    store.items = [item];

    const refresh = store.refreshUrlsForItem(item);
    store.deactivate();
    pending.resolve(
      new Map([
        [
          'att-1',
          {
            assetUrl: { url: '/fresh', expiresAt: '2099-01-01T00:00:00Z' },
            thumbnailAssetUrl: null,
            videoThumbnailAssetUrl: null,
            variantAssetUrls: new Map()
          }
        ]
      ])
    );
    await refresh;

    expect(store.refreshedAttachmentUrls.size).toBe(0);
  });
});
