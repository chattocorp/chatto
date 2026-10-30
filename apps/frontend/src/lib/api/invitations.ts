import { Timestamp } from '@bufbuild/protobuf';
import { AdminInvitationCodeService } from '@chatto/api-types/admin/v1/invitations_connect';
import {
  InvitationCodeStatus,
  type InvitationCode as APIInvitationCode
} from '@chatto/api-types/admin/v1/invitations_pb';
import { createChattoClient, type ConnectAPIConfig } from '@chatto/client/api/connect';

/** A single-use account admission code, visible only to invitation managers. */
export type InvitationCode = {
  id: string;
  code: string;
  createdBy: string;
  createdAt: string;
  expiresAt: string | null;
  status: 'active' | 'expired' | 'used' | 'revoked';
  revokedAt: string | null;
  /** Plain-text creation note for invitation managers; empty when omitted. */
  note: string;
};

export function createInvitationCodeAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminInvitationCodeService, config);
  return {
    async list(offset = 0, limit = 100, options: { signal?: AbortSignal } = {}) {
      const response = await client.listInvitationCodes(
        { page: { offset, limit } },
        { signal: options.signal }
      );
      return {
        invitationCodes: response.invitationCodes.map(mapInvitationCode),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },
    async create(input: { expiresAt: string | null; note?: string }) {
      const response = await client.createInvitationCode({
        expiresAt: input.expiresAt ? Timestamp.fromDate(new Date(input.expiresAt)) : undefined,
        note: input.note ?? ''
      });
      if (!response.invitationCode) throw new Error('Invitation-code response was incomplete.');
      return mapInvitationCode(response.invitationCode);
    },
    async revoke(id: string) {
      const response = await client.revokeInvitationCode({ id });
      if (!response.invitationCode) throw new Error('Invitation-code response was incomplete.');
      return mapInvitationCode(response.invitationCode);
    }
  };
}

function mapInvitationCode(invitation: APIInvitationCode): InvitationCode {
  return {
    id: invitation.id,
    code: invitation.code,
    createdBy: invitation.createdBy,
    createdAt: invitation.createdAt?.toDate().toISOString() ?? '',
    expiresAt: invitation.expiresAt?.toDate().toISOString() ?? null,
    status: mapInvitationCodeStatus(invitation.status),
    revokedAt: invitation.revokedAt?.toDate().toISOString() ?? null,
    note: invitation.note
  };
}

function mapInvitationCodeStatus(status: InvitationCodeStatus): InvitationCode['status'] {
  switch (status) {
    case InvitationCodeStatus.EXPIRED:
      return 'expired';
    case InvitationCodeStatus.USED:
      return 'used';
    case InvitationCodeStatus.REVOKED:
      return 'revoked';
    case InvitationCodeStatus.ACTIVE:
      return 'active';
    default:
      throw new Error('Unknown invitation-code status.');
  }
}
