import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomThreadingMode } from '$lib/roomThreading';

import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { createRoomCommandAPI } from '$lib/api-client/rooms';
import {
  normalizeRoomName,
  roomNameCharacterCount,
  roomNameValidationError
} from '$lib/utils/roomName';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  createRoom: vi.fn(),
  updateRoom: vi.fn(),
  joinRoom: vi.fn(),
  startDM: vi.fn(),
  leaveRoom: vi.fn(),
  addMember: vi.fn(),
  removeMember: vi.fn(),
  listSuspensions: vi.fn(),
  joinRoomGroup: vi.fn(),
  refreshTypingIndicator: vi.fn(),
  removeUser: vi.fn(),
  liftSuspension: vi.fn()
}));

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

describe('createRoomCommandAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.createRoom.mockReset();
    mocks.updateRoom.mockReset();
    mocks.joinRoom.mockReset();
    mocks.startDM.mockReset();
    mocks.leaveRoom.mockReset();
    mocks.addMember.mockReset();
    mocks.removeMember.mockReset();
    mocks.listSuspensions.mockReset();
    mocks.joinRoomGroup.mockReset();
    mocks.refreshTypingIndicator.mockReset();
    mocks.removeUser.mockReset();
    mocks.liftSuspension.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      createRoom: mocks.createRoom,
      updateRoom: mocks.updateRoom,
      joinRoom: mocks.joinRoom,
      startDM: mocks.startDM,
      leaveRoom: mocks.leaveRoom,
      addMember: mocks.addMember,
      removeMember: mocks.removeMember,
      listSuspensions: mocks.listSuspensions,
      joinRoomGroup: mocks.joinRoomGroup,
      refreshTypingIndicator: mocks.refreshTypingIndicator,
      removeUser: mocks.removeUser,
      liftSuspension: mocks.liftSuspension
    });
  });

  it('creates a room and maps the response', async () => {
    mocks.createRoom.mockResolvedValue({
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

    const api = createRoomCommandAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });
    const room = await api.createRoom({
      name: 'general',
      description: 'General chat',
      groupId: 'group-1',
      universal: true,
      threadingMode: RoomThreadingMode.REQUIRED
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith(
      expect.objectContaining({
        baseUrl: 'https://remote.example.test/api/connect',
        useBinaryFormat: true
      })
    );
    expect(mocks.createRoom).toHaveBeenCalledWith({
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
    mocks.updateRoom.mockResolvedValue({
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

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

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

    expect(mocks.updateRoom).toHaveBeenCalledWith({
      roomId: 'room-1',
      name: 'renamed',
      description: 'Updated',
      universal: true,
      threadingMode: RoomThreadingMode.ENCOURAGED,
      updateMask: { paths: ['name', 'description', 'universal', 'threading_mode'] }
    });

    await api.updateRoom({ roomId: 'room-1', universal: false });

    expect(mocks.updateRoom).toHaveBeenLastCalledWith({
      roomId: 'room-1',
      name: undefined,
      description: undefined,
      universal: false,
      updateMask: { paths: ['universal'] }
    });
  });

  it('uses Connect room and directory membership commands', async () => {
    mocks.joinRoom.mockResolvedValue({ room: { id: 'room-1', name: 'general' } });
    mocks.startDM.mockResolvedValue({ room: { id: 'dm-1', name: '' } });
    mocks.leaveRoom.mockResolvedValue({});
    mocks.addMember.mockResolvedValue({
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
    mocks.removeMember.mockResolvedValue({ removed: true });
    mocks.joinRoomGroup.mockResolvedValue({ joinedRoomIds: ['room-1', 'room-2'] });

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

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

    expect(mocks.joinRoom).toHaveBeenCalledWith({ roomId: 'room-1' });
    expect(mocks.startDM).toHaveBeenCalledWith({ participantIds: ['user-1'] });
    expect(mocks.leaveRoom).toHaveBeenCalledWith({ roomId: 'room-1' });
    expect(mocks.addMember).toHaveBeenCalledWith({ roomId: 'room-1', userId: 'user-1' });
    expect(mocks.removeMember).toHaveBeenCalledWith({ roomId: 'room-1', userId: 'user-1' });
    expect(mocks.joinRoomGroup).toHaveBeenCalledWith({ groupId: 'group-1' });
  });

  it('updates typing indicators through RoomService', async () => {
    mocks.refreshTypingIndicator.mockResolvedValue({});

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

    await expect(api.refreshTypingIndicator('room-1', 'thread-root-1')).resolves.toBe(true);

    expect(mocks.refreshTypingIndicator).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 'thread-root-1'
    });
  });

  it('sends removal and suspension commands through RoomService', async () => {
    mocks.removeUser.mockResolvedValue({});
    mocks.liftSuspension.mockResolvedValue({});

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

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

    expect(mocks.removeUser).toHaveBeenCalledWith({
      roomId: 'room-1',
      userId: 'user-1',
      reason: 'policy',
      suspension: {
        case: 'suspensionExpiresAt',
        value: expect.objectContaining({ toDate: expect.any(Function) })
      }
    });
    expect(mocks.liftSuspension).toHaveBeenCalledWith({
      roomId: 'room-1',
      userId: 'user-1',
      reason: 'appeal'
    });
  });

  it('lists active room suspensions through RoomService and maps hydrated references', async () => {
    mocks.listSuspensions.mockResolvedValue({
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

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });
    const controller = new AbortController();

    await expect(
      api.listSuspensions({ roomId: 'room-1' }, { signal: controller.signal })
    ).resolves.toEqual({
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

    expect(mocks.listSuspensions).toHaveBeenCalledWith(
      { roomId: 'room-1', page: { limit: 100, offset: 0 } },
      { signal: controller.signal }
    );
  });

  it('propagates Connect errors unchanged', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.joinRoom.mockRejectedValue(err);

    const api = createRoomCommandAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'expired-token'
    });

    await expect(api.joinRoom('room-1')).rejects.toBe(err);
  });

  it('preserves core-style room length validation messages for CreateRoom', async () => {
    mocks.createRoom.mockRejectedValue(
      new ConnectError('validation error: name must be at most 30 characters', Code.InvalidArgument)
    );

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(
      api.createRoom({
        name: '𐐀'.repeat(31),
        description: null,
        groupId: 'group-1'
      })
    ).rejects.toThrow('room name must be 30 characters or less');
  });

  it('preserves core-style room description length validation messages for CreateRoom', async () => {
    mocks.createRoom.mockRejectedValue(
      new ConnectError(
        'validation error: description must be at most 500 characters',
        Code.InvalidArgument
      )
    );

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(
      api.createRoom({
        name: 'general',
        description: 'a'.repeat(501),
        groupId: 'group-1'
      })
    ).rejects.toThrow('room description must be 500 characters or less');
  });
});
