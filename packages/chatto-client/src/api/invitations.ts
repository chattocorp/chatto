/** Invite-link requests shared by frontends and integrations. All reads are
 * scoped by the server to owned links or effective global management authority. */
import { Timestamp } from '@bufbuild/protobuf';
import { InviteLinkService } from '@chatto/api-types/api/v1/invitations_connect';
import {
  InviteLinkStatus,
  type InviteLink as APIInviteLink
} from '@chatto/api-types/api/v1/invitations_pb';
import { createChattoClient, type ConnectAPIConfig } from './connect.js';

/** A visible bearer link and its current lifecycle state. Keep it in memory only. */
export type InviteLink = {
  id: string;
  link: string;
  createdBy: string;
  createdAt: string;
  maxUses: number | null;
  expiresAt: string | null;
  useCount: number;
  status: 'active' | 'expired' | 'exhausted' | 'revoked';
  revokedAt: string | null;
};

/** Creates the invite-link facade. Hosts must clear cached results when the
 * session, permissions, or privileged mode changes. */
export function createInviteLinkAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(InviteLinkService, config);
  return {
    async list(offset = 0, limit = 100, options: { signal?: AbortSignal } = {}) {
      const response = await client.listInviteLinks(
        { page: { offset, limit } },
        { signal: options.signal }
      );
      return {
        inviteLinks: response.inviteLinks.map(mapInviteLink),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },
    async get(id: string, options: { signal?: AbortSignal } = {}) {
      const response = await client.getInviteLink({ id }, { signal: options.signal });
      if (!response.inviteLink) throw new Error('Invite-link response was incomplete.');
      return mapInviteLink(response.inviteLink);
    },
    async create(
      input: { maxUses: number | null; expiresAt: string | null },
      options: { signal?: AbortSignal } = {}
    ) {
      const response = await client.createInviteLink(
        {
          maxUses: input.maxUses ?? undefined,
          expiresAt: input.expiresAt ? Timestamp.fromDate(new Date(input.expiresAt)) : undefined
        },
        { signal: options.signal }
      );
      if (!response.inviteLink) throw new Error('Invite-link response was incomplete.');
      return mapInviteLink(response.inviteLink);
    },
    async revoke(id: string, options: { signal?: AbortSignal } = {}) {
      const response = await client.revokeInviteLink({ id }, { signal: options.signal });
      if (!response.inviteLink) throw new Error('Invite-link response was incomplete.');
      return mapInviteLink(response.inviteLink);
    }
  };
}

function mapInviteLink(inviteLink: APIInviteLink): InviteLink {
  return {
    id: inviteLink.id,
    link: inviteLink.link,
    createdBy: inviteLink.createdBy,
    createdAt: inviteLink.createdAt?.toDate().toISOString() ?? '',
    maxUses: inviteLink.maxUses ?? null,
    expiresAt: inviteLink.expiresAt?.toDate().toISOString() ?? null,
    useCount: inviteLink.useCount,
    status: mapInviteLinkStatus(inviteLink.status),
    revokedAt: inviteLink.revokedAt?.toDate().toISOString() ?? null
  };
}

function mapInviteLinkStatus(status: InviteLinkStatus): InviteLink['status'] {
  switch (status) {
    case InviteLinkStatus.EXPIRED:
      return 'expired';
    case InviteLinkStatus.EXHAUSTED:
      return 'exhausted';
    case InviteLinkStatus.REVOKED:
      return 'revoked';
    default:
      return 'active';
  }
}
