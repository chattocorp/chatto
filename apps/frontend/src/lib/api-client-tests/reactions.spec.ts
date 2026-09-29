import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createReactionAPI } from '$lib/api-client/reactions';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const mocks = mockService(MessageService);

function reactionAPI() {
  return createReactionAPI(fakeServer((router) => router.service(MessageService, mocks)));
}

describe('createReactionAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('adds a reaction', async () => {
    mocks.addReaction.mockReturnValue({
      added: true,
      reaction: {
        emoji: 'thumbsup',
        count: 2,
        hasReacted: true,
        previewUserIds: ['u1', 'u2']
      }
    });

    const result = await reactionAPI().addReaction({
      roomId: 'room-1',
      messageEventId: 'event-1',
      emoji: 'thumbsup'
    });

    expect(receivedRequest(mocks.addReaction)).toMatchObject({
      roomId: 'room-1',
      messageEventId: 'event-1',
      emoji: 'thumbsup'
    });
    expect(result).toEqual({
      added: true,
      reaction: {
        emoji: 'thumbsup',
        count: 2,
        hasReacted: true,
        previewUserIds: ['u1', 'u2']
      }
    });
  });

  it('removes a reaction and maps a missing summary to null', async () => {
    mocks.removeReaction.mockReturnValue({ removed: false });

    const result = await reactionAPI().removeReaction({
      roomId: 'room-1',
      messageEventId: 'event-1',
      emoji: 'thumbsup'
    });

    expect(receivedRequest(mocks.removeReaction)).toMatchObject({
      roomId: 'room-1',
      messageEventId: 'event-1',
      emoji: 'thumbsup'
    });
    expect(result).toEqual({ removed: false, reaction: null });
  });

  it('propagates Connect errors', async () => {
    mocks.addReaction.mockImplementation(() => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });

    await expect(
      reactionAPI().addReaction({ roomId: 'room-1', messageEventId: 'event-1', emoji: 'thumbsup' })
    ).rejects.toMatchObject({ code: Code.Unauthenticated, rawMessage: 'authentication required' });
  });
});
