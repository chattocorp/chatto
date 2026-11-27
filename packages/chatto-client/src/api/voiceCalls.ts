import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { VoiceCallService } from '@chatto/api-types/api/v1/voice_calls_connect';
import { CallMediaPublisherKind } from '@chatto/api-types/api/v1/voice_calls_pb';

export type VoiceCallToken = {
  token: string;
  e2eeKey: string;
  callId: string;
};

/**
 * An exact call target occupies its own wire field. A server that predates
 * this target sees no room ID and rejects the request safely.
 */
function callTarget(roomId: string, expectedCallId?: string) {
  return expectedCallId === undefined ? { roomId } : { call: { roomId, callId: expectedCallId } };
}

export function createVoiceCallAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(VoiceCallService, config);

  return {
    async joinCall(roomId: string, expectedCallId?: string): Promise<boolean> {
      return (await client.joinCall(callTarget(roomId, expectedCallId))).joined;
    },

    async createCallToken(roomId: string, expectedCallId?: string): Promise<VoiceCallToken | null> {
      const response = await client.createCallToken(callTarget(roomId, expectedCallId));
      if (!response.token || !response.e2eeKey || !response.callId) return null;
      return {
        token: response.token,
        e2eeKey: response.e2eeKey,
        callId: response.callId
      };
    },

    async createGameSharePublisherToken(roomId: string): Promise<VoiceCallToken | null> {
      const response = await client.createCallMediaPublisherToken({
        roomId,
        kind: CallMediaPublisherKind.GAME_SHARE
      });
      if (!response.token || !response.e2eeKey || !response.callId) return null;
      return {
        token: response.token,
        e2eeKey: response.e2eeKey,
        callId: response.callId
      };
    },

    async leaveCall(roomId: string, expectedCallId?: string): Promise<boolean> {
      return (await client.leaveCall(callTarget(roomId, expectedCallId))).left;
    }
  };
}

export type VoiceCallAPI = ReturnType<typeof createVoiceCallAPI>;
