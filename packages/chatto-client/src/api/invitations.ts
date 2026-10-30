import { InvitationCodeService } from '@chatto/api-types/api/v1/invitations_connect';
import { createChattoClient, type ConnectAPIConfig } from './connect.js';

/** Member issuance only: the returned secret must not be cached or persisted. */
export function createMemberInvitationAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(InvitationCodeService, config);
  return {
    /** Creates a seven-day, single-use code. Do not automatically retry this operation. */
    async create(options: { signal?: AbortSignal } = {}) {
      const response = await client.createInvitationCode({}, { signal: options.signal });
      if (!response.invitationCode) throw new Error('Invitation-code response was incomplete.');
      return response.invitationCode;
    }
  };
}
