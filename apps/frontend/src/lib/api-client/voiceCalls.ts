import { authHeaders, createChattoClient } from './connect.js';
import { VoiceCallService } from '@chatto/api-types/api/v1/voice_calls_connect';

export type VoiceCallAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type VoiceCallToken = {
  token: string;
  e2eeKey: string;
  callId: string;
};

export function createVoiceCallAPI(config: VoiceCallAPIConfig) {
  const client = createChattoClient(VoiceCallService, config);
  const headers = () => authHeaders(config);

  return {
    async joinCall(roomId: string): Promise<boolean> {
      return (await client.joinCall({ roomId }, { headers: headers() })).joined;
    },

    async getCallToken(roomId: string): Promise<VoiceCallToken | null> {
      const response = await client.getCallToken({ roomId }, { headers: headers() });
      if (!response.token || !response.e2eeKey || !response.callId) return null;
      return {
        token: response.token,
        e2eeKey: response.e2eeKey,
        callId: response.callId
      };
    },

    async leaveCall(roomId: string): Promise<boolean> {
      return (await client.leaveCall({ roomId }, { headers: headers() })).left;
    }
  };
}

export type VoiceCallAPI = ReturnType<typeof createVoiceCallAPI>;
