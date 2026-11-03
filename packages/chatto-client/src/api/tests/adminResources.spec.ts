import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';
import { AdminRoomLayoutService } from '@chatto/api-types/admin/v1/room_layout_connect';
import { createAdminUserAPI } from '../adminUsers.js';
import { createAdminRoomLayoutAPI } from '../adminRoomLayout.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

describe('canonical administrative resources', () => {
  it('reads account data without a member bootstrap or role catalogue', async () => {
    const service = mockService(AdminUserService);
    service.getUser.mockReturnValue({
      user: { profile: { id: 'target' }, roleNames: ['helper'], verifiedEmails: [] }
    });
    service.listUsers.mockReturnValue({ userIds: ['target'], page: { totalCount: 1n } });
    service.batchGetUsers.mockReturnValue({ users: [{ profile: { id: 'target' } }] });
    const api = createAdminUserAPI(
      fakeServer((router) => router.service(AdminUserService, service))
    );
    expect((await api.getUser({ login: 'target' })).profile?.id).toBe('target');
    expect(receivedRequest(service.getUser)).toMatchObject({
      target: { case: 'login', value: 'target' }
    });
    expect((await api.listUsers({ page: { limit: 20 } })).userIds).toEqual(['target']);
    expect((await api.batchGetUsers(['target']))[0]?.profile?.id).toBe('target');
    expect(service.getMember).not.toHaveBeenCalled();
    expect(service.batchGetMembers).not.toHaveBeenCalled();
  });

  it('adapts older-server account reads without inventing hidden cooldown data', async () => {
    const service = mockService(AdminUserService);
    for (const method of [service.getUser, service.listUsers, service.batchGetUsers]) {
      method.mockImplementation(() => {
        throw new ConnectError('older server', Code.Unimplemented);
      });
    }
    service.getMember.mockReturnValue({
      member: { user: { id: 'target' }, roles: ['helper'], verifiedEmails: ['target@example.com'] }
    });
    service.listMembers.mockReturnValue({ userIds: ['target'], page: { totalCount: 1n } });
    service.batchGetMembers.mockReturnValue({ members: [{ user: { id: 'target' } }] });
    const api = createAdminUserAPI(
      fakeServer((router) => router.service(AdminUserService, service))
    );
    const user = await api.getUser({ userId: 'target' });
    expect(user.profile?.id).toBe('target');
    expect(user.roleNames).toEqual(['helper']);
    expect(user.usernameCooldown).toBeUndefined();
    expect((await api.listUsers({})).userIds).toEqual(['target']);
    expect((await api.batchGetUsers(['target']))[0]?.profile?.id).toBe('target');
  });

  it('does not hide access errors or malformed resources behind legacy reads', async () => {
    const service = mockService(AdminUserService);
    service.getUser.mockImplementation(() => {
      throw new ConnectError('denied', Code.PermissionDenied);
    });
    const api = createAdminUserAPI(
      fakeServer((router) => router.service(AdminUserService, service))
    );
    await expect(api.getUser({ userId: 'target' })).rejects.toMatchObject({
      code: Code.PermissionDenied
    });
    service.getUser.mockReturnValue({});
    await expect(api.getUser({ userId: 'target' })).rejects.toThrow('did not include an account');
    expect(service.getMember).not.toHaveBeenCalled();
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
