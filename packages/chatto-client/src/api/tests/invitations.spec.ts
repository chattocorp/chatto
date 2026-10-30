import { describe, expect, it } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { InvitationCodeService } from '@chatto/api-types/api/v1/invitations_connect';
import { createMemberInvitationAPI } from '../invitations.js';
import { fakeServer, mockService } from '../../testing/fakeServer.js';

describe('member code issuance', () => {
  it('returns the creation result and has no recovery operations', async () => {
    const service = mockService(InvitationCodeService);
    service.createInvitationCode.mockReturnValue({ invitationCode: { id: 'one', code: 'secret' } });
    const api = createMemberInvitationAPI(
      fakeServer((router) => router.service(InvitationCodeService, service))
    );
    expect(await api.create()).toMatchObject({ id: 'one', code: 'secret' });
    expect(Object.keys(api)).toEqual(['create']);
  });

  it('rejects missing results and propagates cancellation without retry', async () => {
    const service = mockService(InvitationCodeService);
    service.createInvitationCode.mockReturnValue({});
    const api = createMemberInvitationAPI(
      fakeServer((router) => router.service(InvitationCodeService, service))
    );
    await expect(api.create()).rejects.toThrow('incomplete');
    service.createInvitationCode.mockRejectedValue(
      new ConnectError('Denied', Code.PermissionDenied)
    );
    await expect(api.create()).rejects.toMatchObject({ code: Code.PermissionDenied });
    expect(service.createInvitationCode).toHaveBeenCalledTimes(2);
    await expect(api.create({ signal: AbortSignal.abort() })).rejects.toMatchObject({
      code: Code.Canceled
    });
  });
});
