import { Timestamp } from '@bufbuild/protobuf';
import { AdminInvitationService } from '@chatto/api-types/admin/v1/invitations_connect';
import {
  InvitationStatus,
  type Invitation as APIInvitation
} from '@chatto/api-types/admin/v1/invitations_pb';
import { authHeaders, createChattoClient } from './connect.js';

export type Invitation = {
  id: string;
  code: string;
  createdBy: string;
  createdAt: string;
  maxUses: number | null;
  expiresAt: string | null;
  useCount: number;
  status: 'active' | 'expired' | 'exhausted' | 'revoked';
  revokedAt: string | null;
};

export type InvitationAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export function createInvitationAPI(config: InvitationAPIConfig) {
  const client = createChattoClient(AdminInvitationService, config);
  const headers = () => authHeaders(config);
  return {
    async list(offset = 0, limit = 100, options: { signal?: AbortSignal } = {}) {
      const response = await client.listInvitations(
        { page: { offset, limit } },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return {
        invitations: response.invitations.map(mapInvitation),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },
    async create(input: { maxUses: number | null; expiresAt: string | null }) {
      const response = await client.createInvitation(
        {
          maxUses: input.maxUses ?? undefined,
          expiresAt: input.expiresAt ? Timestamp.fromDate(new Date(input.expiresAt)) : undefined
        },
        { headers: headers() }
      );
      if (!response.invitation) throw new Error('Invitation response was incomplete.');
      return mapInvitation(response.invitation);
    },
    async revoke(id: string) {
      const response = await client.revokeInvitation({ id }, { headers: headers() });
      if (!response.invitation) throw new Error('Invitation response was incomplete.');
      return mapInvitation(response.invitation);
    }
  };
}

function mapInvitation(invitation: APIInvitation): Invitation {
  return {
    id: invitation.id,
    code: invitation.code,
    createdBy: invitation.createdBy,
    createdAt: invitation.createdAt?.toDate().toISOString() ?? '',
    maxUses: invitation.maxUses ?? null,
    expiresAt: invitation.expiresAt?.toDate().toISOString() ?? null,
    useCount: invitation.useCount,
    status: mapInvitationStatus(invitation.status),
    revokedAt: invitation.revokedAt?.toDate().toISOString() ?? null
  };
}

function mapInvitationStatus(status: InvitationStatus): Invitation['status'] {
  switch (status) {
    case InvitationStatus.EXPIRED:
      return 'expired';
    case InvitationStatus.EXHAUSTED:
      return 'exhausted';
    case InvitationStatus.REVOKED:
      return 'revoked';
    default:
      return 'active';
  }
}
