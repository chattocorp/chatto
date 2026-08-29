import { describe, expect, it } from 'vitest';
import {
  getIncludedByPermission,
  getIncludingPermissions,
  getPermissionCategory,
  getPermissionCategoryLabel,
  PERMISSION_METADATA
} from './permissions';

describe('PERMISSION_METADATA', () => {
  it('covers every current backend permission', () => {
    expect(Object.keys(PERMISSION_METADATA).sort()).toEqual([
      'admin.view-audit',
      'admin.view-users',
      'bot.create',
      'bot.manage',
      'message.attach',
      'message.echo',
      'message.manage',
      'message.post',
      'message.post-in-thread',
      'message.react',
      'message.read',
      'message.read-interactions',
      'role.assign',
      'role.manage',
      'room.ban-member',
      'room.create',
      'room.join',
      'room.list',
      'room.manage',
      'server.manage',
      'server.manage-neighbors',
      'user.delete-any',
      'user.delete-self',
      'user.invite',
      'user.manage-accounts',
      'user.manage-permissions'
    ]);
  });

  it('does not list retired message edit/delete permissions', () => {
    expect(PERMISSION_METADATA).not.toHaveProperty('message.edit-own');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.edit-any');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.delete-own');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.delete-any');
  });

  it('uses explicit inclusion metadata', () => {
    const permissions = ['message.read', 'message.read-interactions', 'message.post-in-thread'];
    expect(getIncludedByPermission(permissions, 'message.read-interactions')).toBe('message.read');
    expect(getIncludedByPermission(permissions, 'message.read')).toBeNull();
    expect(getIncludedByPermission(permissions, 'message.post-in-thread')).toBeNull();
    expect(
      getIncludedByPermission(
        ['server.manage', 'server.manage-neighbors'],
        'server.manage-neighbors'
      )
    ).toBe('server.manage');
  });

  it('does not derive inclusion from identifier punctuation', () => {
    const permissions = ['server.manage', 'server.manage.neighbors'];
    expect(getIncludingPermissions(permissions, 'server.manage.neighbors')).toEqual([]);
    expect(getIncludingPermissions(['server.manage'], 'server.manage.neighbors')).toEqual([]);
  });

  it('uses explicit categories with a presentation-only fallback for newer IDs', () => {
    expect(getPermissionCategory('server.manage')).toBe('server');
    expect(getPermissionCategory('room.future-capability')).toBe('room');
    expect(getPermissionCategory('future.permission')).toBe('other');
    expect(getPermissionCategoryLabel('server')).toBe('Server');
    expect(getPermissionCategoryLabel('other')).toBe('Other');
  });
});
