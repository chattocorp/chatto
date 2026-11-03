import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';
import { createAdminAccountAPI } from '../adminAccounts.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

describe('canonical administrative resources', () => {
  it('reads private email and cooldown data without a user bootstrap', async () => {
    const service = mockService(AdminUserService);
    service.listVerifiedEmails.mockReturnValue({
      verifiedEmails: [{ email: 'target@example.com', primary: true }]
    });
    service.getUsernameCooldown.mockReturnValue({ cooldown: { userId: 'target' } });
    const api = createAdminAccountAPI(
      fakeServer((router) => router.service(AdminUserService, service))
    );
    expect((await api.listVerifiedEmails('target'))[0]?.primary).toBe(true);
    expect(receivedRequest(service.listVerifiedEmails)).toMatchObject({ userId: 'target' });
    expect((await api.getUsernameCooldown('target')).lastLoginChange).toBeUndefined();
    expect(receivedRequest(service.getUsernameCooldown)).toMatchObject({ userId: 'target' });
    expect(service.getMember).not.toHaveBeenCalled();
    expect(service.batchGetMembers).not.toHaveBeenCalled();
  });

  it('propagates private-read failures without falling back to a member bootstrap', async () => {
    const service = mockService(AdminUserService);
    const api = createAdminAccountAPI(
      fakeServer((router) => router.service(AdminUserService, service))
    );
    for (const code of [Code.PermissionDenied, Code.Unimplemented, Code.Canceled]) {
      service.listVerifiedEmails.mockImplementation(() => {
        throw new ConnectError('read failed', code);
      });
      service.getUsernameCooldown.mockImplementation(() => {
        throw new ConnectError('read failed', code);
      });
      await expect(api.listVerifiedEmails('target')).rejects.toMatchObject({ code });
      await expect(api.getUsernameCooldown('target')).rejects.toMatchObject({ code });
    }
    expect(service.getMember).not.toHaveBeenCalled();
  });

  it('distinguishes an empty email list from missing cooldown history', async () => {
    const service = mockService(AdminUserService);
    service.listVerifiedEmails.mockReturnValue({});
    service.getUsernameCooldown.mockReturnValue({});
    const api = createAdminAccountAPI(
      fakeServer((router) => router.service(AdminUserService, service))
    );
    expect(await api.listVerifiedEmails('target')).toEqual([]);
    await expect(api.getUsernameCooldown('target')).rejects.toThrow('did not include history');
  });
});
