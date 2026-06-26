import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createRoomCommandAPI } from './rooms';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  createRoom: vi.fn(),
  joinRoom: vi.fn(),
  startDM: vi.fn(),
  leaveRoom: vi.fn(),
  joinGroup: vi.fn(),
  banRoomMember: vi.fn(),
  unbanRoomMember: vi.fn()
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

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    handleAuthenticationRequired: mocks.handleAuthenticationRequired
  }
}));

describe('createRoomCommandAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();
    mocks.createRoom.mockReset();
    mocks.joinRoom.mockReset();
    mocks.startDM.mockReset();
    mocks.leaveRoom.mockReset();
    mocks.joinGroup.mockReset();
    mocks.banRoomMember.mockReset();
    mocks.unbanRoomMember.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      createRoom: mocks.createRoom,
      joinRoom: mocks.joinRoom,
      startDM: mocks.startDM,
      leaveRoom: mocks.leaveRoom,
      joinGroup: mocks.joinGroup,
      banRoomMember: mocks.banRoomMember,
      unbanRoomMember: mocks.unbanRoomMember
    });
  });

  it('creates a room with bearer auth and maps the response', async () => {
    mocks.createRoom.mockResolvedValue({
      room: {
        id: 'room-1',
        name: 'general',
        description: 'General chat',
        archived: false,
        groupId: 'group-1',
        universal: true
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
      universal: true
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.createRoom).toHaveBeenCalledWith(
      {
        name: 'general',
        description: 'General chat',
        groupId: 'group-1',
        universal: true
      },
      { headers: { Authorization: 'Bearer remote-token' } }
    );
    expect(room).toEqual({
      id: 'room-1',
      name: 'general',
      description: 'General chat',
      archived: false,
      groupId: 'group-1',
      universal: true
    });
  });

  it('uses Connect room and directory membership commands', async () => {
    mocks.joinRoom.mockResolvedValue({ room: { id: 'room-1', name: 'general' } });
    mocks.startDM.mockResolvedValue({ room: { id: 'dm-1', name: '' } });
    mocks.leaveRoom.mockResolvedValue({ left: true });
    mocks.joinGroup.mockResolvedValue({ joinedRoomIds: ['room-1', 'room-2'] });

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(api.joinRoom('room-1')).resolves.toMatchObject({ id: 'room-1' });
    await expect(api.startDM(['user-1'])).resolves.toMatchObject({ id: 'dm-1' });
    await expect(api.leaveRoom('room-1')).resolves.toBe(true);
    await expect(api.joinGroup('group-1')).resolves.toEqual(['room-1', 'room-2']);

    expect(mocks.joinRoom).toHaveBeenCalledWith({ roomId: 'room-1' }, { headers: undefined });
    expect(mocks.startDM).toHaveBeenCalledWith(
      { participantIds: ['user-1'] },
      { headers: undefined }
    );
    expect(mocks.leaveRoom).toHaveBeenCalledWith({ roomId: 'room-1' }, { headers: undefined });
    expect(mocks.joinGroup).toHaveBeenCalledWith({ groupId: 'group-1' }, { headers: undefined });
  });

  it('sends ban and unban commands through RoomService', async () => {
    mocks.banRoomMember.mockResolvedValue({ banned: true });
    mocks.unbanRoomMember.mockResolvedValue({ unbanned: true });

    const api = createRoomCommandAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

    await expect(
      api.banRoomMember({
        roomId: 'room-1',
        userId: 'user-1',
        reason: 'policy',
        expiresAt: '2026-06-01T12:00:00.000Z'
      })
    ).resolves.toBe(true);
    await expect(
      api.unbanRoomMember({ roomId: 'room-1', userId: 'user-1', reason: 'appeal' })
    ).resolves.toBe(true);

    expect(mocks.banRoomMember).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        userId: 'user-1',
        reason: 'policy',
        expiresAt: expect.objectContaining({ toDate: expect.any(Function) })
      },
      { headers: { Authorization: 'Bearer remote-token' } }
    );
    expect(mocks.unbanRoomMember).toHaveBeenCalledWith(
      { roomId: 'room-1', userId: 'user-1', reason: 'appeal' },
      { headers: { Authorization: 'Bearer remote-token' } }
    );
  });

  it('marks the server authentication stale on unauthenticated Connect errors', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.joinRoom.mockRejectedValue(err);

    const api = createRoomCommandAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'expired-token'
    });

    await expect(api.joinRoom('room-1')).rejects.toBe(err);
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
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
        name: 'a'.repeat(31),
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
