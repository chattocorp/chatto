import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { createVoiceCallAPI } from '$lib/api-client/voiceCalls';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listActiveCallRooms: vi.fn(),
  getActiveCall: vi.fn(),
  batchGetActiveCalls: vi.fn(),
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
    mocks.getActiveCall.mockReset();
    mocks.batchGetActiveCalls.mockReset();
    mocks.listCallParticipants.mockReset();
    mocks.joinCall.mockReset();
    mocks.getCallToken.mockReset();
    mocks.leaveCall.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listActiveCallRooms: mocks.listActiveCallRooms,
      getActiveCall: mocks.getActiveCall,
      batchGetActiveCalls: mocks.batchGetActiveCalls,
      listCallParticipants: mocks.listCallParticipants,
      joinCall: mocks.joinCall,
      getCallToken: mocks.getCallToken,
      leaveCall: mocks.leaveCall
    });
  });

  it('maps voice call reads and sends bearer auth', async () => {
    const participant = {
      user: {
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        deleted: false,
        avatarUrl: 'https://cdn/avatar.webp'
      },
      joinedAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
      callId: 'call-1'
    };
    mocks.listActiveCallRooms.mockResolvedValue({
      calls: [
        {
          roomId: 'room-1',
          callId: 'call-1',
          participants: [participant]
        }
      ]
    });
    mocks.getActiveCall.mockResolvedValue({
      call: {
        roomId: 'room-1',
        callId: 'call-1',
        participants: [participant]
      }
    });
    mocks.batchGetActiveCalls.mockResolvedValue({
      calls: [
        {
          roomId: 'room-1',
          callId: 'call-1',
          participants: [participant]
        }
      ]
    });
    mocks.listCallParticipants.mockResolvedValue({
      participants: [participant]
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

    await expect(api.listActiveCalls()).resolves.toMatchObject([
      {
        roomId: 'room-1',
        callId: 'call-1',
        participants: [{ user: { id: 'U1' }, callId: 'call-1' }]
      }
    ]);
    await expect(api.getActiveCall('room-1')).resolves.toMatchObject({
      roomId: 'room-1',
      callId: 'call-1',
      participants: [{ user: { id: 'U1' }, callId: 'call-1' }]
    });
    await expect(api.batchGetActiveCalls(['room-1', 'missing'])).resolves.toMatchObject([
      {
        roomId: 'room-1',
        callId: 'call-1',
        participants: [{ user: { id: 'U1' }, callId: 'call-1' }]
      }
    ]);
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
    expect(mocks.getActiveCall).toHaveBeenCalledWith(
      { roomId: 'room-1' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.batchGetActiveCalls).toHaveBeenCalledWith(
      { roomIds: ['room-1', 'missing'] },
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

  it('returns null when an active call is missing', async () => {
    mocks.getActiveCall.mockRejectedValue(new ConnectError('not found', Code.NotFound));

    const api = createVoiceCallAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.getActiveCall('room-1')).resolves.toBeNull();
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
