import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { createMessageAPI } from '$lib/api-client/messages';
import {
  MentionConfirmationChallenge,
  CreateMessageResponse,
  UpdateMessageResponse
} from '@chatto/api-types/api/v1/messages_pb';
import {
  RoomTimelineEvent,
  RoomTimelineIncludes,
  RoomTimelineMessagePosted
} from '@chatto/api-types/api/v1/room_timeline_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  createMessage: vi.fn(),
  updateMessage: vi.fn(),
  deleteMessage: vi.fn(),
  deleteAttachment: vi.fn(),
  deleteLinkPreview: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createMessageAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();

    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.createMessage.mockReset();
    mocks.updateMessage.mockReset();
    mocks.deleteMessage.mockReset();
    mocks.deleteAttachment.mockReset();
    mocks.deleteLinkPreview.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      createMessage: mocks.createMessage,
      updateMessage: mocks.updateMessage,
      deleteMessage: mocks.deleteMessage,
      deleteAttachment: mocks.deleteAttachment,
      deleteLinkPreview: mocks.deleteLinkPreview
    });
  });

  it('posts a message with bearer auth and maps the renderable event response', async () => {
    mocks.createMessage.mockResolvedValue(
      new CreateMessageResponse({
        result: {
          case: 'event',
          value: new RoomTimelineEvent({
            id: 'evt-1',
            actorId: 'user-1',
            createdAt: Timestamp.fromDate(new Date('2026-06-20T10:00:00Z')),
            event: {
              case: 'messagePosted',
              value: new RoomTimelineMessagePosted({
                roomId: 'room-1',
                body: 'hello',
                viewerIsFollowingThread: true
              })
            }
          })
        },
        includes: new RoomTimelineIncludes({
          users: {
            'user-1': new User({
              id: 'user-1',
              login: 'alice',
              displayName: 'Alice'
            })
          }
        })
      })
    );

    const api = createMessageAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

    const result = await api.createMessage({
      roomId: 'room-1',
      body: 'hello',
      threadRootEventId: 'root-1',
      inReplyTo: 'reply-1',
      alsoSendToChannel: true,
      mentionConfirmationToken: 'confirm-token',
      linkPreview: {
        url: 'https://example.test',
        title: 'Example',
        description: null,
        siteName: 'Example Site',
        imageAssetId: 'asset-1',
        embedType: null,
        embedId: null
      }
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.createMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        roomId: 'room-1',
        body: 'hello',
        threadRootEventId: 'root-1',
        inReplyTo: 'reply-1',
        alsoSendToChannel: true,
        mentionConfirmationToken: 'confirm-token',
        linkPreview: expect.objectContaining({
          url: 'https://example.test',
          imageAssetId: 'asset-1'
        })
      }),
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(result).toMatchObject({
      kind: 'event',
      event: {
        id: 'evt-1',
        actor: { id: 'user-1', displayName: 'Alice' },
        event: { kind: 'messagePosted', body: 'hello' }
      }
    });
  });

  it('returns large mention confirmation challenges without treating them as errors', async () => {
    mocks.createMessage.mockResolvedValue(
      new CreateMessageResponse({
        result: {
          case: 'mentionConfirmation',
          value: new MentionConfirmationChallenge({
            recipientCount: 12,
            token: 'confirm-token'
          })
        }
      })
    );

    const api = createMessageAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(api.createMessage({ roomId: 'room-1', body: '@all hello' })).resolves.toEqual({
      kind: 'mentionConfirmation',
      recipientCount: 12,
      token: 'confirm-token'
    });
    expect(mocks.createMessage).toHaveBeenCalledWith(expect.anything(), { headers: undefined });
  });

  it('maps browser files to protobuf attachment uploads', async () => {
    mocks.createMessage.mockResolvedValue(
      new CreateMessageResponse({
        result: {
          case: 'event',
          value: new RoomTimelineEvent({
            id: 'evt-attachment',
            actorId: 'user-1',
            event: {
              case: 'messagePosted',
              value: new RoomTimelineMessagePosted({
                roomId: 'room-1',
                body: 'with file'
              })
            }
          })
        }
      })
    );

    const file = new File(['hello'], 'note.txt', { type: 'text/plain' });
    const api = createMessageAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await api.createMessage({
      roomId: 'room-1',
      body: 'with file',
      attachments: [file]
    });

    const request = mocks.createMessage.mock.calls[0][0];
    expect(request.attachments).toHaveLength(1);
    expect(request.attachments[0]).toMatchObject({
      filename: 'note.txt',
      contentType: 'text/plain'
    });
    expect(Array.from(request.attachments[0].content)).toEqual([104, 101, 108, 108, 111]);
  });

  it('marks the server authentication stale on unauthenticated Connect errors', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.createMessage.mockRejectedValue(err);

    const api = createMessageAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'expired-token'
    });

    await expect(api.createMessage({ roomId: 'room-1', body: 'hello' })).rejects.toBe(err);
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });

  it('updates a message through MessageService', async () => {
    mocks.updateMessage.mockResolvedValue(
      new UpdateMessageResponse({
        updated: true,
        event: new RoomTimelineEvent({
          id: 'event-1',
          actorId: 'user-1',
          createdAt: Timestamp.fromDate(new Date('2026-06-20T10:00:00Z')),
          event: {
            case: 'messagePosted',
            value: new RoomTimelineMessagePosted({
              roomId: 'room-1',
              body: 'edited'
            })
          }
        }),
        includes: new RoomTimelineIncludes({
          users: {
            'user-1': new User({
              id: 'user-1',
              login: 'alice',
              displayName: 'Alice'
            })
          }
        })
      })
    );

    const api = createMessageAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

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

    expect(mocks.updateMessage).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        eventId: 'event-1',
        body: 'edited',
        alsoSendToChannel: false
      },
      { headers: { Authorization: 'Bearer remote-token' } }
    );
  });

  it('can patch message echo state without sending a body', async () => {
    mocks.updateMessage.mockResolvedValue(new UpdateMessageResponse({ updated: true }));

    const api = createMessageAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(
      api.updateMessage({
        roomId: 'room-1',
        eventId: 'event-1',
        alsoSendToChannel: true
      })
    ).resolves.toEqual({ updated: true, event: null });

    expect(mocks.updateMessage).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        eventId: 'event-1',
        alsoSendToChannel: true
      },
      { headers: undefined }
    );
  });

  it('deletes message content through MessageService', async () => {
    mocks.deleteMessage.mockResolvedValue({ deleted: true });
    mocks.deleteAttachment.mockResolvedValue({ deleted: true });
    mocks.deleteLinkPreview.mockResolvedValue({ deleted: true });

    const api = createMessageAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(api.deleteMessage('room-1', 'event-1')).resolves.toBe(true);
    await expect(api.deleteAttachment('room-1', 'event-1', 'attachment-1')).resolves.toBe(true);
    await expect(
      api.deleteLinkPreview('room-1', 'event-1', 'https://example.test/article')
    ).resolves.toBe(true);

    expect(mocks.deleteMessage).toHaveBeenCalledWith(
      { roomId: 'room-1', eventId: 'event-1' },
      { headers: undefined }
    );
    expect(mocks.deleteAttachment).toHaveBeenCalledWith(
      { roomId: 'room-1', eventId: 'event-1', attachmentId: 'attachment-1' },
      { headers: undefined }
    );
    expect(mocks.deleteLinkPreview).toHaveBeenCalledWith(
      { roomId: 'room-1', eventId: 'event-1', url: 'https://example.test/article' },
      { headers: undefined }
    );
  });

});
