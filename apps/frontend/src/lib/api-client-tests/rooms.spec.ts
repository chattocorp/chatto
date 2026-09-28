import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomThreadingMode } from '$lib/roomThreading';

import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { createRoomCommandAPI } from '$lib/api-client/rooms';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';
import {
  normalizeRoomName,
  roomNameCharacterCount,
  roomNameValidationError
} from '$lib/utils/roomName';

describe('room name helpers', () => {
  it('normalizes Unicode names and counts code points', () => {
    expect(normalizeRoomName('  Ku\u0308che  ')).toBe('Küche');
    expect(normalizeRoomName('は\u3099')).toBe('ば');
    expect(normalizeRoomName('Pho\u0300ng')).toBe('Phòng');
    expect(roomNameCharacterCount('繁體中文')).toBe(4);
    expect(roomNameCharacterCount('𐐀'.repeat(30))).toBe(30);
    expect(roomNameCharacterCount('𐐀'.repeat(31))).toBe(31);
  });

  it.each([
    ['Arabic with Arabic-Indic digits', 'غرفة_١٢٣'],
    ['Armenian', 'սենյակ'],
    ['Traditional Chinese (zh-TW)', '繁體中文聊天室'],
    ['Cyrillic', 'Комната'],
    ['Deseret supplementary-plane letters', '𐐀𐐨'],
    ['Ethiopic', 'ክፍል'],
    ['Georgian', 'ოთახი'],
    ['Greek', 'Δωμάτιο'],
    ['Hebrew', 'חדר'],
    ['Japanese hiragana', 'ひらがな'],
    ['Japanese kanji', '会議室'],
    ['Japanese katakana', 'カタカナ'],
    ['Korean Hangul', '회의실'],
    ['Latin with Vietnamese diacritics', 'Phòng'],
    ['Turkish dotted capital I', 'İstanbul'],
    ['Devanagari decimal digits', 'room_१२३'],
    ['fullwidth decimal digits', '部屋１２３'],
    ['mixed scripts with separators', 'Küche / 聊天室-١٢٣'],
    ['combining mark that remains after NFC', 'room\u0338'],
    ['Devanagari vowel mark', 'कमरा'],
    ['emoji sequence', 'room👩‍💻'],
    ['left-to-right formatting mark', 'room\u200ename'],
    ['non-decimal superscript number', 'room²'],
    ['Thai combining mark', 'ห้อง'],
    ['zero-width joiner', 'room\u200dname'],
    ['spaces, punctuation, and emoji', 'Team chat 💬!']
  ])('accepts %s', (_description, name) => {
    expect(roomNameValidationError(name)).toBeUndefined();
  });

  it.each([
    ['empty input', '', 'empty'],
    ['whitespace-only input', ' \t ', 'empty'],
    ['format-only input', '\u200d\u2060', 'empty'],
    ['control character', 'room\u0000name', 'invalid'],
    ['line break', 'room\nname', 'invalid'],
    ['line separator', 'room\u2028name', 'invalid'],
    ['paragraph separator', 'room\u2029name', 'invalid'],
    ['31 code points', '𐐀'.repeat(31), 'too_long']
  ] as const)('rejects %s', (_description, name, error) => {
    expect(roomNameValidationError(name)).toBe(error);
  });
});

const mocks = mockService(RoomService);

function roomAPI() {
  return createRoomCommandAPI(fakeServer((router) => router.service(RoomService, mocks)));
}

describe('createRoomCommandAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('creates a room and maps the response', async () => {
    mocks.createRoom.mockReturnValue({
      room: {
        id: 'room-1',
        name: 'general',
        description: 'General chat',
        archived: false,
        groupId: 'group-1',
        universal: true,
        slowModeSeconds: 0,
        threadingMode: RoomThreadingMode.REQUIRED
      }
    });

    const api = roomAPI();
    const room = await api.createRoom({
      name: 'general',
      description: 'General chat',
      groupId: 'group-1',
      universal: true,
      threadingMode: RoomThreadingMode.REQUIRED
    });

    expect(receivedRequest(mocks.createRoom)).toMatchObject({
      name: 'general',
      description: 'General chat',
      groupId: 'group-1',
      universal: true,
      threadingMode: RoomThreadingMode.REQUIRED
    });
    expect(room).toEqual({
      id: 'room-1',
      name: 'general',
      description: 'General chat',
      archived: false,
      groupId: 'group-1',
      universal: true,
      slowModeSeconds: 0,
      threadingMode: RoomThreadingMode.REQUIRED
    });
  });

  it('updates room metadata and universal state through RoomService', async () => {
    mocks.updateRoom.mockReturnValue({
      room: {
        id: 'room-1',
        name: 'renamed',
        description: 'Updated',
        archived: false,
        groupId: 'group-1',
        universal: true,
        slowModeSeconds: 0,
        threadingMode: RoomThreadingMode.ENCOURAGED
      }
    });

    const api = roomAPI();

    await expect(
      api.updateRoom({
        roomId: 'room-1',
        name: 'renamed',
        description: 'Updated',
        universal: true,
        threadingMode: RoomThreadingMode.ENCOURAGED
      })
    ).resolves.toEqual({
      id: 'room-1',
      name: 'renamed',
      description: 'Updated',
      archived: false,
      groupId: 'group-1',
      universal: true,
      slowModeSeconds: 0,
      threadingMode: RoomThreadingMode.ENCOURAGED
    });

    expect(receivedRequest(mocks.updateRoom)).toMatchObject({
      roomId: 'room-1',
      name: 'renamed',
      description: 'Updated',
      universal: true,
      threadingMode: RoomThreadingMode.ENCOURAGED,
      updateMask: { paths: ['name', 'description', 'universal', 'threading_mode'] }
    });

    await api.updateRoom({ roomId: 'room-1', universal: false });

    expect(mocks.updateRoom.mock.lastCall?.[0]).toMatchObject({
      roomId: 'room-1',
      name: undefined,
      description: undefined,
      universal: false,
      updateMask: { paths: ['universal'] }
    });
  });

  it('uses Connect room and directory membership commands', async () => {
    mocks.joinRoom.mockReturnValue({ room: { id: 'room-1', name: 'general' } });
    mocks.startDM.mockReturnValue({ room: { id: 'dm-1', name: '' } });
    mocks.leaveRoom.mockReturnValue({});
    mocks.addMember.mockReturnValue({
      member: {
        user: {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice',
          deleted: false,
          presenceStatus: APIPresenceStatus.ONLINE
        },
        roles: []
      }
    });
    mocks.removeMember.mockReturnValue({ removed: true });
    mocks.joinRoomGroup.mockReturnValue({ joinedRoomIds: ['room-1', 'room-2'] });

    const api = roomAPI();

    await expect(api.joinRoom('room-1')).resolves.toMatchObject({ id: 'room-1' });
    await expect(api.startDM(['user-1'])).resolves.toMatchObject({ id: 'dm-1' });
    await expect(api.leaveRoom('room-1')).resolves.toBe(true);
    await expect(api.addMember({ roomId: 'room-1', userId: 'user-1' })).resolves.toMatchObject({
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      presenceStatus: PresenceStatus.ONLINE
    });
    await expect(api.removeMember({ roomId: 'room-1', userId: 'user-1' })).resolves.toBe(true);
    await expect(api.joinGroup('group-1')).resolves.toEqual(['room-1', 'room-2']);

    expect(receivedRequest(mocks.joinRoom)).toMatchObject({ roomId: 'room-1' });
    expect(receivedRequest(mocks.startDM)).toMatchObject({ participantIds: ['user-1'] });
    expect(receivedRequest(mocks.leaveRoom)).toMatchObject({ roomId: 'room-1' });
    expect(receivedRequest(mocks.addMember)).toMatchObject({ roomId: 'room-1', userId: 'user-1' });
    expect(receivedRequest(mocks.removeMember)).toMatchObject({
      roomId: 'room-1',
      userId: 'user-1'
    });
    expect(receivedRequest(mocks.joinRoomGroup)).toMatchObject({ groupId: 'group-1' });
  });

  it('updates typing indicators through RoomService', async () => {
    mocks.refreshTypingIndicator.mockReturnValue({});

    const api = roomAPI();

    await expect(api.refreshTypingIndicator('room-1', 'thread-root-1')).resolves.toBe(true);

    expect(receivedRequest(mocks.refreshTypingIndicator)).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 'thread-root-1'
    });
  });

  it('sends removal and suspension commands through RoomService', async () => {
    mocks.removeUser.mockReturnValue({});
    mocks.liftSuspension.mockReturnValue({});

    const api = roomAPI();

    await expect(
      api.removeUser({
        roomId: 'room-1',
        userId: 'user-1',
        reason: 'policy',
        suspension: { kind: 'until', expiresAt: '2026-06-01T12:00:00.000Z' }
      })
    ).resolves.toBe(true);
    await expect(
      api.liftSuspension({ roomId: 'room-1', userId: 'user-1', reason: 'appeal' })
    ).resolves.toBe(true);

    expect(receivedRequest(mocks.removeUser)).toMatchObject({
      roomId: 'room-1',
      userId: 'user-1',
      reason: 'policy',
      suspension: {
        case: 'suspensionExpiresAt',
        value: expect.objectContaining({ toDate: expect.any(Function) })
      }
    });
    expect(receivedRequest(mocks.liftSuspension)).toMatchObject({
      roomId: 'room-1',
      userId: 'user-1',
      reason: 'appeal'
    });
  });

  it('lists active room suspensions through RoomService and maps hydrated references', async () => {
    mocks.listSuspensions.mockReturnValue({
      suspensions: [
        {
          id: 'ban-1',
          roomId: 'room-1',
          room: {
            id: 'room-1',
            name: 'general',
            description: 'General chat',
            archived: false,
            groupId: 'group-1',
            universal: false,
            slowModeSeconds: undefined
          },
          userId: 'user-1',
          user: {
            user: {
              id: 'user-1',
              login: 'alice',
              displayName: 'Alice',
              deleted: false,
              avatarUrl: 'https://cdn/avatar.webp',
              presenceStatus: APIPresenceStatus.AWAY
            },
            roles: [],
            createdAt: Timestamp.fromDate(new Date('2026-01-01T09:00:00Z'))
          },
          moderatorId: 'mod-1',
          moderator: {
            user: {
              id: 'mod-1',
              login: 'mod',
              displayName: 'Moderator',
              deleted: false,
              presenceStatus: APIPresenceStatus.OFFLINE
            },
            roles: []
          },
          reason: 'policy',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
          expiresAt: Timestamp.fromDate(new Date('2026-06-02T12:00:00Z'))
        }
      ],
      page: { totalCount: 1n, hasMore: false }
    });

    const api = roomAPI();
    await expect(api.listSuspensions({ roomId: 'room-1' })).resolves.toEqual({
      suspensions: [
        {
          id: 'ban-1',
          roomId: 'room-1',
          room: {
            id: 'room-1',
            name: 'general',
            description: 'General chat',
            archived: false,
            groupId: 'group-1',
            universal: false,
            slowModeSeconds: 0,
            threadingMode: RoomThreadingMode.ENABLED
          },
          userId: 'user-1',
          user: {
            id: 'user-1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false,
            avatarUrl: 'https://cdn/avatar.webp',
            bio: null,
            timezone: null,
            presenceStatus: PresenceStatus.AWAY,
            customStatus: null,
            isBot: false,
            roles: [],
            createdAt: '2026-01-01T09:00:00.000Z'
          },
          moderatorId: 'mod-1',
          moderator: {
            id: 'mod-1',
            login: 'mod',
            displayName: 'Moderator',
            deleted: false,
            avatarUrl: null,
            bio: null,
            timezone: null,
            presenceStatus: PresenceStatus.OFFLINE,
            customStatus: null,
            isBot: false,
            roles: [],
            createdAt: null
          },
          reason: 'policy',
          createdAt: '2026-06-01T12:00:00.000Z',
          expiresAt: '2026-06-02T12:00:00.000Z'
        }
      ],
      totalCount: 1,
      hasMore: false
    });

    expect(receivedRequest(mocks.listSuspensions)).toMatchObject({
      roomId: 'room-1',
      page: { limit: 100, offset: 0 }
    });
    await expect(
      api.listSuspensions({ roomId: 'room-1' }, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('propagates Connect errors', async () => {
    mocks.joinRoom.mockImplementation(() => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });

    const api = roomAPI();

    await expect(api.joinRoom('room-1')).rejects.toMatchObject({
      code: Code.Unauthenticated,
      rawMessage: 'authentication required'
    });
  });

  it('preserves core-style room length validation messages for CreateRoom', async () => {
    mocks.createRoom.mockImplementation(() => {
      throw new ConnectError(
        'validation error: name must be at most 30 characters',
        Code.InvalidArgument
      );
    });

    const api = roomAPI();

    await expect(
      api.createRoom({
        name: '𐐀'.repeat(31),
        description: null,
        groupId: 'group-1'
      })
    ).rejects.toThrow('room name must be 30 characters or less');
  });

  it('preserves core-style room description length validation messages for CreateRoom', async () => {
    mocks.createRoom.mockImplementation(() => {
      throw new ConnectError(
        'validation error: description must be at most 500 characters',
        Code.InvalidArgument
      );
    });

    const api = roomAPI();

    await expect(
      api.createRoom({
        name: 'general',
        description: 'a'.repeat(501),
        groupId: 'group-1'
      })
    ).rejects.toThrow('room description must be 500 characters or less');
  });
});
