import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
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

function roomFileItem(): RoomFileItem {
  return {
    messageEventId: 'event-1',
    threadRootEventId: null,
    createdAt: '2026-07-03T12:00:00.000Z',
    attachment: {
      id: 'att-1',
      filename: 'image.jpg',
      contentType: 'image/jpeg',
      width: 800,
      height: 600,
      assetUrl: {
        url: '/assets/files/att-1?stale=1',
        expiresAt: '2026-07-03T13:00:00.000Z'
      },
      thumbnailAssetUrl: {
        url: '/assets/files/att-1/image/120x120/cover?stale=1',
        expiresAt: '2026-07-03T13:00:00.000Z'
      },
      videoProcessing: null
    }
  };
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
  });

  it('refreshes from attachment events using local event kind', async () => {
    const store = new RoomFilesStore(serverConnection());

    store.setRoom('room-1');
    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(1);
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
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledTimes(2);
    });
    expect(attachmentMocks.listRoomAttachments).toHaveBeenLastCalledWith(
      expect.objectContaining({ roomId: 'room-1' })
    );
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
});
