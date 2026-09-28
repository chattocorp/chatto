import { MessageSchema } from '@chatto/api-types/api/v1/message_types_pb';
import { PinnedMessageSchema } from '@chatto/api-types/api/v1/rooms_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinnedMessagesAPI } from './pinnedMessages';
import { create } from '@bufbuild/protobuf';

const listPinnedMessagesMock = vi.hoisted(() => vi.fn());
const timelineUsersForMessagesMock = vi.hoisted(() => vi.fn());

vi.mock('./connect.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./connect.js')>()),
  createChattoClient: () => ({ listPinnedMessages: listPinnedMessagesMock })
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
    const message = create(MessageSchema, { id: 'M1', actorId: 'author' });
    const pinnedMessage = create(PinnedMessageSchema, { message });
    listPinnedMessagesMock.mockResolvedValue({
      pinnedMessages: [pinnedMessage],
      page: { totalCount: 1n, hasMore: false },
      latestPinMarker: 'opaque-marker'
    });
    const config = { serverId: 'server-1', baseUrl: '/api/connect', bearerToken: null };

    const page = await createPinnedMessagesAPI(config).list('R1', 50, 0);

    expect(timelineUsersForMessagesMock).toHaveBeenCalledWith(config, [message], undefined);
    expect(page).toEqual({
      items: [pinnedMessage],
      totalCount: 1,
      hasMore: false,
      latestPinMarker: 'opaque-marker'
    });
  });
});
