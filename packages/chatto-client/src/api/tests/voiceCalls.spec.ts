import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createVoiceCallAPI } from '../voiceCalls.js';
import { VoiceCallService } from '@chatto/api-types/api/v1/voice_calls_connect';
import { CallMediaPublisherKind } from '@chatto/api-types/api/v1/voice_calls_pb';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(VoiceCallService);

describe('createVoiceCallAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('maps call commands', async () => {
    mocks.joinCall.mockReturnValue({ joined: true });
    mocks.leaveCall.mockReturnValue({ left: true });
    mocks.createCallToken.mockReturnValue({ token: 'jwt', e2eeKey: 'key', callId: 'call-1' });
    mocks.createCallMediaPublisherToken.mockReturnValue({
      token: 'publisher-jwt',
      e2eeKey: 'key',
      callId: 'call-1'
    });

    const api = createVoiceCallAPI(fakeServer((router) => router.service(VoiceCallService, mocks)));

    await expect(api.joinCall('room-1')).resolves.toBe(true);
    await expect(api.createCallToken('room-1')).resolves.toEqual({
      token: 'jwt',
      e2eeKey: 'key',
      callId: 'call-1'
    });
    await expect(api.createGameSharePublisherToken('room-1')).resolves.toEqual({
      token: 'publisher-jwt',
      e2eeKey: 'key',
      callId: 'call-1'
    });
    await expect(api.leaveCall('room-1')).resolves.toBe(true);

    expect(receivedRequest(mocks.joinCall)).toMatchObject({ roomId: 'room-1' });
    expect(receivedRequest(mocks.createCallToken)).toMatchObject({ roomId: 'room-1' });
    expect(receivedRequest(mocks.leaveCall)).toMatchObject({ roomId: 'room-1' });
  });

  it('returns null for incomplete call tokens', async () => {
    mocks.createCallToken.mockReturnValue({ token: 'jwt', callId: 'call-1' });
    mocks.createCallMediaPublisherToken.mockReturnValue({ e2eeKey: 'key' });
    const api = createVoiceCallAPI(fakeServer((router) => router.service(VoiceCallService, mocks)));

    await expect(api.createCallToken('room-1')).resolves.toBeNull();
    await expect(api.createGameSharePublisherToken('room-1')).resolves.toBeNull();
    expect(receivedRequest(mocks.createCallMediaPublisherToken)).toMatchObject({
      roomId: 'room-1',
      kind: CallMediaPublisherKind.GAME_SHARE
    });
  });
});
