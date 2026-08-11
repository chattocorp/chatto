import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinnedMessagesAPI } from './pinnedMessages';

const listPinnedMessagesMock = vi.hoisted(() => vi.fn());
const timelineUsersForMessagesMock = vi.hoisted(() => vi.fn());

vi.mock('./connect.js', () => ({
  authHeaders: () => new Headers(),
  createChattoClient: () => ({ listPinnedMessages: listPinnedMessagesMock }),
  handleAuthError: (_config: unknown, error: unknown) => {
    throw error;
  }
}));

vi.mock('./roomTimeline.js', () => ({
  timelineUsersForMessages: timelineUsersForMessagesMock
}));

describe('pinned messages API', () => {
  beforeEach(() => {
    listPinnedMessagesMock.mockReset();
    timelineUsersForMessagesMock.mockReset().mockResolvedValue({});
  });

  it('hydrates message-related users through the shared user cache', async () => {
    const message = new Message({ id: 'M1', actorId: 'author' });
    const pinnedMessage = new PinnedMessage({ message });
    listPinnedMessagesMock.mockResolvedValue({
      pinnedMessages: [pinnedMessage],
      page: { totalCount: 1n, hasMore: false },
      latestPinMarker: 'opaque-marker'
    });
    const config = { serverId: 'server-1', baseUrl: '/api/connect', bearerToken: null };

    const page = await createPinnedMessagesAPI(config).list('R1', 50, 0);

    expect(timelineUsersForMessagesMock).toHaveBeenCalledWith(config, [message]);
    expect(page).toEqual({
      items: [pinnedMessage],
      totalCount: 1,
      hasMore: false,
      latestPinMarker: 'opaque-marker'
    });
  });
});
