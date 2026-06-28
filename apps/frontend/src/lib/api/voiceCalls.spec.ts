import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createVoiceCallAPI } from './voiceCalls';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listActiveCallRooms: vi.fn(),
  listCallParticipants: vi.fn(),
  joinCall: vi.fn(),
  getCallToken: vi.fn(),
  leaveCall: vi.fn()
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

describe('createVoiceCallAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listActiveCallRooms.mockReset();
    mocks.listCallParticipants.mockReset();
    mocks.joinCall.mockReset();
    mocks.getCallToken.mockReset();
    mocks.leaveCall.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listActiveCallRooms: mocks.listActiveCallRooms,
      listCallParticipants: mocks.listCallParticipants,
      joinCall: mocks.joinCall,
      getCallToken: mocks.getCallToken,
      leaveCall: mocks.leaveCall
    });
  });

  it('maps voice call reads and sends bearer auth', async () => {
    mocks.listActiveCallRooms.mockResolvedValue({ roomIds: ['room-1'] });
    mocks.listCallParticipants.mockResolvedValue({
      participants: [
        {
          user: {
            user: {
              id: 'U1',
              login: 'alice',
              displayName: 'Alice',
              deleted: false,
              avatarUrl: 'https://cdn/avatar.webp'
            }
          },
          joinedAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
          callId: 'call-1'
        }
      ]
    });
    mocks.getCallToken.mockResolvedValue({
      token: 'jwt',
      e2eeKey: 'key',
      callId: 'call-1'
    });

    const api = createVoiceCallAPI({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.listActiveCallRoomIds()).resolves.toEqual(['room-1']);
    await expect(api.listCallParticipants('room-1')).resolves.toEqual([
      {
        user: {
          id: 'U1',
          login: 'alice',
          displayName: 'Alice',
          deleted: false,
          avatarUrl: 'https://cdn/avatar.webp'
        },
        joinedAt: '2026-06-01T12:00:00.000Z',
        callId: 'call-1'
      }
    ]);
    await expect(api.getCallToken('room-1')).resolves.toEqual({
      token: 'jwt',
      e2eeKey: 'key',
      callId: 'call-1'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.listActiveCallRooms).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.listCallParticipants).toHaveBeenCalledWith(
      { roomId: 'room-1' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.getCallToken).toHaveBeenCalledWith(
      { roomId: 'room-1' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('maps join and leave commands without auth headers', async () => {
    mocks.joinCall.mockResolvedValue({ joined: true });
    mocks.leaveCall.mockResolvedValue({ left: true });

    const api = createVoiceCallAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.joinCall('room-1')).resolves.toBe(true);
    await expect(api.leaveCall('room-1')).resolves.toBe(true);

    expect(mocks.joinCall).toHaveBeenCalledWith({ roomId: 'room-1' }, { headers: undefined });
    expect(mocks.leaveCall).toHaveBeenCalledWith({ roomId: 'room-1' }, { headers: undefined });
  });
});
