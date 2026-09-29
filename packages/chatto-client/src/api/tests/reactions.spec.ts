import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createReactionAPI } from '../reactions.js';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

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

  it('pages the users of a reaction', async () => {
    mocks.listReactionUsers.mockReturnValue({
      userIds: ['u1', 'u2'],
      page: { totalCount: 5n, hasMore: true }
    });
    const input = { roomId: 'room-1', messageEventId: 'event-1', emoji: 'thumbsup' };
    await expect(reactionAPI().listReactionUsers(input, 2)).resolves.toEqual({
      userIds: ['u1', 'u2'],
      totalCount: 5,
      hasMore: true
    });
    expect(receivedRequest(mocks.listReactionUsers)).toMatchObject({
      ...input,
      page: { offset: 2, limit: 50 }
    });

    mocks.listReactionUsers.mockReturnValue({ userIds: [] });
    await expect(reactionAPI().listReactionUsers(input, 0, 10)).resolves.toEqual({
      userIds: [],
      totalCount: 0,
      hasMore: false
    });
  });
});
