import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { VoiceCallService } from "@chatto/api-types/api/v1/voice_calls_connect";

export type VoiceCallAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type VoiceCallParticipantUser = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl: string | null;
};

export type VoiceCallParticipant = {
  user: VoiceCallParticipantUser;
  joinedAt: string;
  callId: string;
};

export type VoiceCallToken = {
  token: string;
  e2eeKey: string;
  callId: string;
};

export function createVoiceCallAPI(config: VoiceCallAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(VoiceCallService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  return {
    async listActiveCallRoomIds(): Promise<string[]> {
      return (await client.listActiveCallRooms({}, { headers: headers() }))
        .roomIds;
    },

    async listCallParticipants(
      roomId: string,
    ): Promise<VoiceCallParticipant[]> {
      const response = await client.listCallParticipants(
        { roomId },
        { headers: headers() },
      );
      return response.participants.flatMap((participant) => {
        const summary = participant.user?.user;
        if (!summary) return [];
        return [
          {
            user: {
              id: summary.id,
              login: summary.login,
              displayName: summary.displayName,
              deleted: summary.deleted,
              avatarUrl: summary.avatarUrl ?? null,
            },
            joinedAt:
              participant.joinedAt?.toDate().toISOString() ??
              new Date(0).toISOString(),
            callId: participant.callId,
          },
        ];
      });
    },

    async joinCall(roomId: string): Promise<boolean> {
      return (await client.joinCall({ roomId }, { headers: headers() })).joined;
    },

    async getCallToken(roomId: string): Promise<VoiceCallToken | null> {
      const response = await client.getCallToken(
        { roomId },
        { headers: headers() },
      );
      if (!response.token || !response.e2eeKey || !response.callId) return null;
      return {
        token: response.token,
        e2eeKey: response.e2eeKey,
        callId: response.callId,
      };
    },

    async leaveCall(roomId: string): Promise<boolean> {
      return (await client.leaveCall({ roomId }, { headers: headers() })).left;
    },
  };
}

export type VoiceCallAPI = ReturnType<typeof createVoiceCallAPI>;
