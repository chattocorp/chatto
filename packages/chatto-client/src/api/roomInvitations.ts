import { RoomInvitationService } from '@chatto/api-types/api/v1/room_invitations_connect';
import { createChattoClient, type ConnectAPIConfig } from './connect.js';

/** Invitation commands operate on durable invitation state, independently of notifications. */
export function createRoomInvitationAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(RoomInvitationService, config);
  return {
    /** Invite an existing human member; duplicate sends preserve the original deadline. */
    async createInvitation(
      roomId: string,
      recipientId: string,
      options?: { signal?: AbortSignal }
    ) {
      return (await client.createRoomInvitation({ roomId, recipientId }, options)).invitation;
    },
    /** Read an invitation owned by the sender or recipient. */
    async getInvitation(invitationId: string, options?: { signal?: AbortSignal }) {
      return (await client.getRoomInvitation({ invitationId }, options)).invitation;
    },
    /** Read one page of received or sent invitations. */
    async listInvitations(options?: {
      signal?: AbortSignal;
      offset?: number;
      sent?: boolean;
      pendingOnly?: boolean;
    }) {
      return client.listRoomInvitations(
        {
          page: { limit: 50, offset: options?.offset ?? 0 },
          sent: options?.sent,
          pendingOnly: options?.pendingOnly
        },
        options
      );
    },
    /** Accept and join atomically; repeating acceptance never rejoins after a later leave. */
    async acceptInvitation(invitationId: string, options?: { signal?: AbortSignal }) {
      return (await client.acceptRoomInvitation({ invitationId }, options)).invitation;
    },
    /** Decline without changing membership. */
    async declineInvitation(invitationId: string, options?: { signal?: AbortSignal }) {
      return (await client.declineRoomInvitation({ invitationId }, options)).invitation;
    }
  };
}
