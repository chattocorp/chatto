import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createVoiceCallAPI } from '$lib/api-client/voiceCalls';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
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
    mocks.joinCall.mockReset();
    mocks.getCallToken.mockReset();
    mocks.leaveCall.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      joinCall: mocks.joinCall,
      getCallToken: mocks.getCallToken,
      leaveCall: mocks.leaveCall
    });
  });

  it('maps call commands without auth headers', async () => {
    mocks.joinCall.mockResolvedValue({ joined: true });
    mocks.leaveCall.mockResolvedValue({ left: true });
    mocks.getCallToken.mockResolvedValue({ token: 'jwt', e2eeKey: 'key', callId: 'call-1' });

    const api = createVoiceCallAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.joinCall('room-1')).resolves.toBe(true);
    await expect(api.getCallToken('room-1')).resolves.toEqual({
      token: 'jwt',
      e2eeKey: 'key',
      callId: 'call-1'
    });
    await expect(api.leaveCall('room-1')).resolves.toBe(true);

    expect(mocks.joinCall).toHaveBeenCalledWith({ roomId: 'room-1' }, { headers: undefined });
    expect(mocks.getCallToken).toHaveBeenCalledWith(
      { roomId: 'room-1' },
      { headers: undefined }
    );
    expect(mocks.leaveCall).toHaveBeenCalledWith({ roomId: 'room-1' }, { headers: undefined });
  });
});
