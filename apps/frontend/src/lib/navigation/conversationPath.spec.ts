import { describe, expect, it, vi } from 'vitest';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import {
  conversationPath,
  conversationUserId,
  userConversationRoomId,
  type ConversationStore
} from './conversationPath';

vi.mock('$lib/navigation', () => ({ serverIdToSegment: () => '-' }));

function store(ids: string[], kind = RoomKind.DM): ConversationStore {
  return {
    accountId: 'viewer',
    projection: { rooms: new Map([['room', { room: { kind }, memberUserIds: ids }]]) }
  };
}

describe('conversation destinations', () => {
  it('uses participant IDs without requiring a profile, including deleted users', () => {
    const data = store(['viewer', 'deleted']);
    expect(conversationUserId(data, 'room')).toBe('deleted');
    expect(conversationPath('server', 'room', { store: data })).toBe('/chat/-/user/deleted');
    expect(userConversationRoomId(data, 'deleted')).toBe('room');
  });

  it('uses the viewer ID for a self-conversation', () => {
    expect(conversationUserId(store(['viewer']), 'room')).toBe('viewer');
  });

  it.each([['viewer', 'a', 'b'], ['a', 'b'], []])('does not select a user for %j', (...ids) => {
    expect(conversationPath('server', 'room', { store: store(ids) })).toBe('/chat/-/room');
  });

  it('preserves thread and message destinations', () => {
    expect(
      conversationPath('server', 'room', {
        threadId: 'thread',
        messageId: 'message',
        store: store(['viewer', 'peer'])
      })
    ).toBe('/chat/-/user/peer/thread/m/message');
    expect(
      conversationPath('server', 'room', { messageId: 'message', store: store(['viewer', 'peer']) })
    ).toBe('/chat/-/user/peer/m/message');
    expect(
      conversationPath('server', 'room', {
        threadId: 'thread',
        messageId: 'message',
        store: store([], RoomKind.CHANNEL)
      })
    ).toBe('/chat/-/room/thread/m/message');
  });

  it('waits for participant data and never infers a user from a room ID', () => {
    expect(conversationUserId(store([]), 'missing')).toBeNull();
    expect(userConversationRoomId(store([]), 'peer')).toBeNull();
  });
});
