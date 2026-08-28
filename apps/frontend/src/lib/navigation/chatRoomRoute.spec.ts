import { describe, expect, it } from 'vitest';
import { chatRoomIdFromRoute } from './chatRoomRoute';

describe('chatRoomIdFromRoute', () => {
  it('returns the room id for chat room routes', () => {
    expect(chatRoomIdFromRoute('/chat/[serverId]/[roomId]', 'room-1')).toBe('room-1');
    expect(chatRoomIdFromRoute('/chat/[serverId]/[roomId]/[threadId]', 'room-1')).toBe('room-1');
    expect(
      chatRoomIdFromRoute('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', 'room-1')
    ).toBe('room-1');
  });

  it('ignores non-room routes that also use a roomId param', () => {
    expect(chatRoomIdFromRoute('/chat/[serverId]/manage/rooms/[roomId]', 'room-1')).toBe(null);
  });

  it('ignores chat routes without a room id', () => {
    expect(chatRoomIdFromRoute('/chat/[serverId]', undefined)).toBe(null);
    expect(chatRoomIdFromRoute('/chat/notifications', undefined)).toBe(null);
  });
});
