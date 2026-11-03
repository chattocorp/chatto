import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';
import { AdminRoomLayoutService } from '@chatto/api-types/admin/v1/room_layout_connect';
import { createAdminAccountAPI } from '../adminAccounts.js';
import { createAdminRoomLayoutAPI } from '../adminRoomLayout.js';
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

  it('reads canonical layout metadata and patches without replacing other fields', async () => {
    const service = mockService(AdminRoomLayoutService);
    service.getRoomGroup.mockReturnValue({ roomGroup: { id: 'group', name: 'Name' } });
    service.getRoomGroupLayout.mockReturnValue({ layout: { group: { id: 'group' }, items: [] } });
    service.getRoomLayout.mockReturnValue({ layout: { groups: [] } });
    service.getSidebarLink.mockReturnValue({
      link: { link: { id: 'link', url: 'https://example.com' }, groupId: 'group' }
    });
    service.createRoomGroup.mockReturnValue({ roomGroup: { id: 'group', name: 'Name' } });
    service.updateRoomGroup.mockReturnValue({ roomGroup: { id: 'group', description: '' } });
    const api = createAdminRoomLayoutAPI(
      fakeServer((router) => router.service(AdminRoomLayoutService, service))
    );
    expect((await api.getRoomGroup('group')).name).toBe('Name');
    expect((await api.getRoomGroupLayout('group')).items).toEqual([]);
    expect((await api.getRoomLayout()).groups).toEqual([]);
    expect((await api.getSidebarLink('link')).groupId).toBe('group');
    expect((await api.createRoomGroup({ name: 'Name' })).id).toBe('group');
    await api.updateRoomGroup({ groupId: 'group', description: '' });
    expect(receivedRequest(service.updateRoomGroup)?.updateMask?.paths).toEqual(['description']);
  });

  it('adapts equivalent old metadata and server snapshots, but never fabricates scoped layouts', async () => {
    const service = mockService(AdminRoomLayoutService);
    service.getRoomGroup.mockReturnValue({ group: { id: 'group', name: 'Old metadata' } });
    service.getRoomLayout.mockImplementation(() => {
      throw new ConnectError('older server', Code.Unimplemented);
    });
    service.listRoomGroups.mockReturnValue({
      groups: [{ id: 'group', items: [{ item: { case: 'sidebarLink', value: { id: 'link' } } }] }]
    });
    service.getRoomGroupLayout.mockImplementation(() => {
      throw new ConnectError('older server', Code.Unimplemented);
    });
    const api = createAdminRoomLayoutAPI(
      fakeServer((router) => router.service(AdminRoomLayoutService, service))
    );
    expect((await api.getRoomGroup('group')).name).toBe('Old metadata');
    expect((await api.getRoomLayout()).groups[0]?.items).toHaveLength(1);
    service.listRoomGroups.mockClear();
    await expect(api.getRoomGroupLayout('group')).rejects.toMatchObject({
      code: Code.Unimplemented
    });
    expect(service.listRoomGroups).not.toHaveBeenCalled();
  });
});
