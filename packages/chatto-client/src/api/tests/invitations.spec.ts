import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { InviteLinkService } from '@chatto/api-types/api/v1/invitations_connect';
import { InviteLinkStatus } from '@chatto/api-types/api/v1/invitations_pb';
import { createInviteLinkAPI } from '../invitations.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(InviteLinkService);
const api = () =>
  createInviteLinkAPI(fakeServer((router) => router.service(InviteLinkService, mocks)));
const link = {
  id: 'I1',
  link: 'https://chat.example/invite/example',
  createdBy: 'U1',
  createdAt: Timestamp.fromDate(new Date('2026-10-15T12:00:00Z')),
  status: InviteLinkStatus.ACTIVE,
  useCount: 0
};

describe('invite-link API', () => {
  beforeEach(() => vi.resetAllMocks());

  it('keeps server-filtered pagination and maps optional constraints', async () => {
    mocks.listInviteLinks.mockReturnValue({
      inviteLinks: [link],
      page: { totalCount: 3n, hasMore: true }
    });
    const page = await api().list(1, 1);
    expect(receivedRequest(mocks.listInviteLinks)?.page).toMatchObject({ offset: 1, limit: 1 });
    expect(page).toMatchObject({
      totalCount: 3,
      hasMore: true,
      inviteLinks: [{ id: 'I1', createdBy: 'U1', maxUses: null, expiresAt: null, status: 'active' }]
    });
  });

  it('round-trips create, get, and revoke with expiry and use limits', async () => {
    const expiresAt = '2026-10-22T12:00:00.000Z';
    mocks.createInviteLink.mockReturnValue({
      inviteLink: { ...link, maxUses: 1, expiresAt: Timestamp.fromDate(new Date(expiresAt)) }
    });
    mocks.getInviteLink.mockReturnValue({ inviteLink: link });
    mocks.revokeInviteLink.mockReturnValue({
      inviteLink: { ...link, status: InviteLinkStatus.REVOKED, revokedAt: link.createdAt }
    });
    const client = api();
    expect(await client.create({ maxUses: 1, expiresAt })).toMatchObject({ maxUses: 1, expiresAt });
    expect(receivedRequest(mocks.createInviteLink)?.maxUses).toBe(1);
    expect(await client.get('I1')).toMatchObject({ link: link.link });
    expect(receivedRequest(mocks.getInviteLink)?.id).toBe('I1');
    expect(await client.revoke('I1')).toMatchObject({
      status: 'revoked',
      revokedAt: '2026-10-15T12:00:00.000Z'
    });
    expect(receivedRequest(mocks.revokeInviteLink)?.id).toBe('I1');
  });

  it('omits unrestricted constraints and preserves server errors', async () => {
    mocks.createInviteLink.mockReturnValue({ inviteLink: link });
    const client = api();
    await client.create({ maxUses: null, expiresAt: null });
    expect(receivedRequest(mocks.createInviteLink)).toBeDefined();
    expect(receivedRequest(mocks.createInviteLink)?.maxUses).toBeUndefined();
    expect(receivedRequest(mocks.createInviteLink)?.expiresAt).toBeUndefined();
    mocks.getInviteLink.mockRejectedValue(new ConnectError('not found', Code.NotFound));
    await expect(client.get('another-link')).rejects.toMatchObject({ code: Code.NotFound });
  });

  it('rejects incomplete mutation and get responses', async () => {
    mocks.createInviteLink.mockReturnValue({});
    mocks.getInviteLink.mockReturnValue({});
    mocks.revokeInviteLink.mockReturnValue({});
    const client = api();
    for (const result of [
      client.create({ maxUses: null, expiresAt: null }),
      client.get('I1'),
      client.revoke('I1')
    ]) {
      await expect(result).rejects.toThrow('Invite-link response was incomplete');
    }
  });
});
