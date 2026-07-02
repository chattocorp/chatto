import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { createAttachmentAPI, type RoomFileItem } from '$lib/api-client/attachments';
import { RoomFilesStore } from './files.svelte';

const attachmentMocks = vi.hoisted(() => ({
  apis: [] as Array<{
    listRoomAttachments: ReturnType<typeof vi.fn>;
    refreshMessageAttachmentUrls: ReturnType<typeof vi.fn>;
  }>,
  defaultApi: {
    listRoomAttachments: vi.fn(),
    refreshMessageAttachmentUrls: vi.fn()
  }
}));

vi.mock('$lib/api-client/attachments', () => ({
  createAttachmentAPI: vi.fn(() => attachmentMocks.apis.shift() ?? attachmentMocks.defaultApi)
}));

function serverConnection(serverId = 'test-server'): ServerConnection {
  return {
    serverId,
    connectBaseUrl: `https://${serverId}.example.test/api/connect`,
    bearerToken: `${serverId}-token`
  } as ServerConnection;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function emptyPage(): { items: RoomFileItem[]; totalCount: number; hasMore: boolean } {
  return {
    items: [],
    totalCount: 0,
    hasMore: false
  };
}

function fileItem(id: string): RoomFileItem {
  return {
    messageEventId: `message-${id}`,
    threadRootEventId: null,
    createdAt: '2026-01-01T00:00:00Z',
    attachment: {
      id,
      filename: `${id}.png`,
      contentType: 'image/png',
      width: 1,
      height: 1,
      assetUrl: { url: `/assets/${id}`, expiresAt: '2099-01-01T00:00:00Z' },
      thumbnailAssetUrl: null,
      videoProcessing: null
    }
  };
}

describe('RoomFilesStore', () => {
  beforeEach(() => {
    attachmentMocks.apis = [];
    attachmentMocks.defaultApi.listRoomAttachments.mockReset();
    attachmentMocks.defaultApi.refreshMessageAttachmentUrls.mockReset();
    attachmentMocks.defaultApi.listRoomAttachments.mockResolvedValue(emptyPage());
    vi.mocked(createAttachmentAPI).mockClear();
  });

  it('refreshes from attachment events using local event kind', async () => {
    const store = new RoomFilesStore(serverConnection());

    store.setRoom('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.defaultApi.listRoomAttachments).toHaveBeenCalledTimes(1);
    });

    store.ingestServerEvent({
      id: 'evt-1',
      actorId: 'u1',
      createdAt: new Date().toISOString(),
      event: {
        kind: RoomEventKind.AssetProcessingSucceeded,
        assetId: 'asset-1',
        processingRoomId: 'room-1'
      }
    } as EventEnvelope);

    await vi.waitFor(() => {
      expect(attachmentMocks.defaultApi.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
    expect(attachmentMocks.defaultApi.listRoomAttachments).toHaveBeenLastCalledWith(
      expect.objectContaining({ roomId: 'room-1' })
    );
  });

  it('switches attachment APIs and ignores stale loads after a connection change', async () => {
    const firstLoad = deferred<ReturnType<typeof emptyPage>>();
    const firstApi = {
      listRoomAttachments: vi.fn(() => firstLoad.promise),
      refreshMessageAttachmentUrls: vi.fn()
    };
    const secondApi = {
      listRoomAttachments: vi.fn(async () => ({
        ...emptyPage(),
        items: [fileItem('server-2')]
      })),
      refreshMessageAttachmentUrls: vi.fn()
    };
    attachmentMocks.apis.push(firstApi, secondApi);
    const store = new RoomFilesStore(serverConnection('server-1'));

    store.setRoom('room-1');
    await vi.waitFor(() => expect(firstApi.listRoomAttachments).toHaveBeenCalledOnce());

    store.setConnection(serverConnection('server-2'));
    await vi.waitFor(() => expect(secondApi.listRoomAttachments).toHaveBeenCalledOnce());
    expect(store.items.map((item) => item.attachment.id)).toEqual(['server-2']);

    firstLoad.resolve({
      ...emptyPage(),
      items: [fileItem('server-1')]
    });
    await Promise.resolve();

    expect(store.items.map((item) => item.attachment.id)).toEqual(['server-2']);
    expect(createAttachmentAPI).toHaveBeenLastCalledWith({
      serverId: 'server-2',
      baseUrl: 'https://server-2.example.test/api/connect',
      bearerToken: 'server-2-token'
    });
  });

  it('does not let stale loadMore calls clear a newer pagination loading state', async () => {
    const oldLoadMore = deferred<ReturnType<typeof emptyPage>>();
    const newLoadMore = deferred<ReturnType<typeof emptyPage>>();
    const firstApi = {
      listRoomAttachments: vi
        .fn()
        .mockResolvedValueOnce({
          ...emptyPage(),
          items: [fileItem('old-initial')],
          totalCount: 2,
          hasMore: true
        })
        .mockReturnValueOnce(oldLoadMore.promise),
      refreshMessageAttachmentUrls: vi.fn()
    };
    const secondApi = {
      listRoomAttachments: vi
        .fn()
        .mockResolvedValueOnce({
          ...emptyPage(),
          items: [fileItem('new-initial')],
          totalCount: 2,
          hasMore: true
        })
        .mockReturnValueOnce(newLoadMore.promise),
      refreshMessageAttachmentUrls: vi.fn()
    };
    attachmentMocks.apis.push(firstApi, secondApi);
    const store = new RoomFilesStore(serverConnection('server-1'));

    store.setRoom('room-1');
    await vi.waitFor(() => {
      expect(store.items.map((item) => item.attachment.id)).toEqual(['old-initial']);
    });

    const oldLoad = store.loadMore();
    await vi.waitFor(() => expect(firstApi.listRoomAttachments).toHaveBeenCalledTimes(2));
    expect(store.isLoadingMore).toBe(true);

    store.setConnection(serverConnection('server-2'));
    await vi.waitFor(() => {
      expect(store.items.map((item) => item.attachment.id)).toEqual(['new-initial']);
    });

    const newLoad = store.loadMore();
    await vi.waitFor(() => expect(secondApi.listRoomAttachments).toHaveBeenCalledTimes(2));
    expect(store.isLoadingMore).toBe(true);

    oldLoadMore.resolve({
      ...emptyPage(),
      items: [fileItem('old-more')]
    });
    await oldLoad;
    expect(store.isLoadingMore).toBe(true);

    newLoadMore.resolve({
      ...emptyPage(),
      items: [fileItem('new-more')]
    });
    await newLoad;
    expect(store.isLoadingMore).toBe(false);
    expect(store.items.map((item) => item.attachment.id)).toEqual(['new-initial', 'new-more']);
  });
});
