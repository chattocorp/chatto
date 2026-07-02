import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { RoomFilesStore } from './files.svelte';

const attachmentMocks = vi.hoisted(() => ({
  listRoomAttachments: vi.fn(),
  refreshMessageAttachmentUrls: vi.fn()
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

describe('RoomFilesStore', () => {
  beforeEach(() => {
    attachmentMocks.listRoomAttachments.mockReset();
    attachmentMocks.refreshMessageAttachmentUrls.mockReset();
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
});
