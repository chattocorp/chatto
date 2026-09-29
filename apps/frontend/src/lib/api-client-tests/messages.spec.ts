import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createMessageAPI } from '$lib/api-client/messages';
import { AssetUploadService } from '@chatto/api-types/api/v1/asset_uploads_connect';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';
import { CreateMessageResponse, UpdateMessageResponse } from '@chatto/api-types/api/v1/messages_pb';
import {
  AssetUpload,
  AssetUploadStatus,
  CompleteUploadResponse,
  CreateUploadResponse,
  UploadChunkResponse
} from '@chatto/api-types/api/v1/asset_uploads_pb';
import { Asset } from '@chatto/api-types/api/v1/attachments_pb';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';

const messages = mockService(MessageService);
const users = mockService(UserService);
const uploads = mockService(AssetUploadService);

function messageAPI() {
  return createMessageAPI(
    fakeServer((router) =>
      router
        .service(MessageService, messages)
        .service(UserService, users)
        .service(AssetUploadService, uploads)
    )
  );
}

describe('createMessageAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    users.batchGetUsers.mockReturnValue({ users: [] });
  });

  it('trims descriptions before create and edit requests reach schema validation', async () => {
    messages.createMessage.mockReturnValue(new CreateMessageResponse());
    messages.setAttachmentDescription.mockReturnValue({});
    const api = messageAPI();
    const description = '界'.repeat(1000);
    await api.createMessage({
      roomId: 'room-1',
      body: 'hello',
      attachmentDescriptions: [{ assetId: 'asset-1', description: `  ${description}\n` }]
    });
    expect(receivedRequest(messages.createMessage)).toMatchObject({
      attachmentDescriptions: [{ assetId: 'asset-1', description }]
    });
    await api.setAttachmentDescription('room-1', 'event-1', 'asset-1', `  ${description}\n`);
    expect(receivedRequest(messages.setAttachmentDescription)).toMatchObject({ description });
    await api.setAttachmentDescription('room-1', 'event-1', 'asset-1', ' \n ');
    expect(messages.setAttachmentDescription.mock.lastCall?.[0]).toMatchObject({ description: '' });
  });

  it('posts a message and maps the renderable event response', async () => {
    messages.createMessage.mockReturnValue(
      new CreateMessageResponse({
        message: new Message({
          id: 'evt-1',
          actorId: 'user-1',
          createdAt: Timestamp.fromDate(new Date('2026-06-20T10:00:00Z')),
          roomId: 'room-1',
          body: 'hello',
          thread: { viewerState: { isFollowing: true } }
        })
      })
    );
    users.batchGetUsers.mockReturnValue({
      users: [
        {
          user: {
            id: 'user-1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false
          }
        }
      ]
    });

    const api = messageAPI();

    const result = await api.createMessage({
      roomId: 'room-1',
      body: 'hello',
      threadRootEventId: 'root-1',
      inReplyTo: 'reply-1',
      alsoSendToChannel: true,
      linkPreviewToken: 'cht_LPpreviewtoken'
    });

    expect(receivedRequest(messages.createMessage)).toMatchObject({
      roomId: 'room-1',
      body: 'hello',
      threadRootEventId: 'root-1',
      inReplyTo: 'reply-1',
      alsoSendToChannel: true,
      linkPreviewToken: 'cht_LPpreviewtoken'
    });
    expect(receivedRequest(users.batchGetUsers)).toMatchObject({ userIds: ['user-1'] });
    expect(result).toMatchObject({
      event: {
        id: 'evt-1',
        actor: { id: 'user-1', displayName: 'Alice' },
        event: { kind: 'messagePosted', body: 'hello' }
      }
    });
  });

  it('uploads browser files through AssetUploadService and posts attachment asset IDs', async () => {
    uploads.createUpload.mockReturnValue(
      new CreateUploadResponse({
        upload: new AssetUpload({
          uploadId: 'upload-note',
          roomId: 'room-1',
          status: AssetUploadStatus.OPEN,
          committedOffset: 0n,
          size: 5n,
          maxChunkSize: 1024,
          sha256: '2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824'
        })
      })
    );
    uploads.uploadChunk.mockReturnValue(
      new UploadChunkResponse({
        upload: new AssetUpload({
          uploadId: 'upload-note',
          roomId: 'room-1',
          status: AssetUploadStatus.OPEN,
          committedOffset: 5n,
          size: 5n,
          maxChunkSize: 1024
        })
      })
    );
    uploads.completeUpload.mockReturnValue(
      new CompleteUploadResponse({
        upload: new AssetUpload({
          uploadId: 'upload-note',
          status: AssetUploadStatus.COMPLETED,
          committedOffset: 5n,
          size: 5n,
          assetId: 'asset-note'
        }),
        asset: new Asset({
          id: 'asset-note',
          filename: 'note.txt',
          contentType: 'text/plain'
        })
      })
    );
    messages.createMessage.mockReturnValue(
      new CreateMessageResponse({
        message: new Message({
          id: 'evt-attachment',
          actorId: 'user-1',
          roomId: 'room-1',
          body: 'with file'
        })
      })
    );

    const file = new File(['hello'], 'note.txt', { type: 'text/plain' });
    const api = messageAPI();

    await api.createMessage({
      roomId: 'room-1',
      body: 'with file',
      attachments: [file],
      threadRootEventId: 'root-1',
      alsoSendToChannel: true
    });

    expect(receivedRequest(uploads.createUpload)).toMatchObject({
      roomId: 'room-1',
      filename: 'note.txt',
      contentType: 'text/plain',
      size: 5n,
      sha256: '2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824'
    });
    expect(receivedRequest(uploads.uploadChunk)).toMatchObject({
      uploadId: 'upload-note',
      offset: 0n,
      chunkSha256: '2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824'
    });
    expect(Array.from(uploads.uploadChunk.mock.calls[0][0].content)).toEqual([
      104, 101, 108, 108, 111
    ]);
    expect(receivedRequest(uploads.completeUpload)).toMatchObject({ uploadId: 'upload-note' });
    const request = messages.createMessage.mock.calls[0][0];
    expect(request.attachmentAssetIds).toEqual(['asset-note']);
    expect(request.threadRootEventId).toBe('root-1');
    expect(request.alsoSendToChannel).toBe(true);
  });

  it('propagates Connect errors', async () => {
    messages.createMessage.mockImplementation(() => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });

    const api = messageAPI();

    await expect(api.createMessage({ roomId: 'room-1', body: 'hello' })).rejects.toMatchObject({
      code: Code.Unauthenticated,
      rawMessage: 'authentication required'
    });
  });

  it('updates a message through MessageService', async () => {
    messages.updateMessage.mockReturnValue(
      new UpdateMessageResponse({
        message: new Message({
          id: 'event-1',
          actorId: 'user-1',
          createdAt: Timestamp.fromDate(new Date('2026-06-20T10:00:00Z')),
          roomId: 'room-1',
          body: 'edited'
        })
      })
    );
    users.batchGetUsers.mockReturnValue({
      users: [
        {
          user: {
            id: 'user-1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false
          }
        }
      ]
    });

    const api = messageAPI();

    await expect(
      api.updateMessage({
        roomId: 'room-1',
        eventId: 'event-1',
        body: 'edited',
        alsoSendToChannel: false
      })
    ).resolves.toMatchObject({
      updated: true,
      event: {
        id: 'event-1',
        actor: { id: 'user-1', displayName: 'Alice' },
        event: { kind: 'messagePosted', body: 'edited' }
      }
    });

    expect(receivedRequest(messages.updateMessage)).toMatchObject({
      roomId: 'room-1',
      eventId: 'event-1',
      body: 'edited',
      alsoSendToChannel: false,
      updateMask: { paths: ['body', 'also_send_to_channel'] }
    });
    expect(receivedRequest(users.batchGetUsers)).toMatchObject({ userIds: ['user-1'] });
  });

  it('can patch message echo state without sending a body', async () => {
    messages.updateMessage.mockReturnValue(new UpdateMessageResponse());

    const api = messageAPI();

    await expect(
      api.updateMessage({
        roomId: 'room-1',
        eventId: 'event-1',
        alsoSendToChannel: true
      })
    ).resolves.toEqual({ updated: true, event: null });

    expect(receivedRequest(messages.updateMessage)).toMatchObject({
      roomId: 'room-1',
      eventId: 'event-1',
      alsoSendToChannel: true,
      updateMask: { paths: ['also_send_to_channel'] }
    });
  });

  it('deletes message content through MessageService', async () => {
    messages.deleteMessage.mockReturnValue({});
    messages.deleteAttachment.mockReturnValue({});
    messages.deleteLinkPreview.mockReturnValue({});

    const api = messageAPI();

    await expect(api.deleteMessage('room-1', 'event-1')).resolves.toBe(true);
    await expect(api.deleteAttachment('room-1', 'event-1', 'attachment-1')).resolves.toBe(true);
    await expect(
      api.deleteLinkPreview('room-1', 'event-1', 'https://example.test/article')
    ).resolves.toBe(true);

    expect(receivedRequest(messages.deleteMessage)).toMatchObject({
      roomId: 'room-1',
      eventId: 'event-1'
    });
    expect(receivedRequest(messages.deleteAttachment)).toMatchObject({
      roomId: 'room-1',
      eventId: 'event-1',
      attachmentId: 'attachment-1'
    });
    expect(receivedRequest(messages.deleteLinkPreview)).toMatchObject({
      roomId: 'room-1',
      eventId: 'event-1',
      url: 'https://example.test/article'
    });
  });
});
