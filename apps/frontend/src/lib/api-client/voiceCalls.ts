import {
  authHeaders,
  Code,
  ConnectError,
  createChattoClient,
} from "./connect.js";
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

export type ActiveVoiceCall = {
  roomId: string;
  callId: string;
  participants: VoiceCallParticipant[];
};

export type VoiceCallToken = {
  token: string;
  e2eeKey: string;
  callId: string;
};

type APICallParticipant = {
  user?: {
    user?: {
      id: string;
      login: string;
      displayName: string;
      deleted: boolean;
      avatarUrl?: string;
    };
  };
  joinedAt?: { toDate(): Date };
  callId: string;
};

export function createVoiceCallAPI(config: VoiceCallAPIConfig) {
  const client = createChattoClient(VoiceCallService, config);
  const headers = () => authHeaders(config);

  return {
    async listActiveCalls(): Promise<ActiveVoiceCall[]> {
      const response = await client.listActiveCallRooms(
        {},
        { headers: headers() },
      );
      return response.calls.map(activeCall);
    },

    async getActiveCall(roomId: string): Promise<ActiveVoiceCall | null> {
      try {
        const response = await client.getActiveCall(
          { roomId },
          { headers: headers() },
        );
        return response.call ? activeCall(response.call) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetActiveCalls(roomIds: string[]): Promise<ActiveVoiceCall[]> {
      const response = await client.batchGetActiveCalls(
        { roomIds },
        { headers: headers() },
      );
      return response.calls.map(activeCall);
    },

    async listCallParticipants(
      roomId: string,
    ): Promise<VoiceCallParticipant[]> {
      const response = await client.listCallParticipants(
        { roomId },
        { headers: headers() },
      );
      return response.participants.flatMap(callParticipant);
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

function activeCall(call: {
  roomId: string;
  callId: string;
  participants: readonly APICallParticipant[];
}): ActiveVoiceCall {
  return {
    roomId: call.roomId,
    callId: call.callId,
    participants: call.participants.flatMap(callParticipant),
  };
}

function callParticipant(
  participant: APICallParticipant,
): VoiceCallParticipant[] {
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
}
