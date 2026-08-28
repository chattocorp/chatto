import { describe, expect, it } from 'vitest';
import {
  getIncludedByPermission,
  getIncludingPermissions,
  PERMISSION_METADATA
} from './permissions';

describe('PERMISSION_METADATA', () => {
  it('covers every current backend permission', () => {
    expect(Object.keys(PERMISSION_METADATA).sort()).toEqual([
      'admin.view-audit',
      'admin.view-users',
      'message.attach',
      'message.echo',
      'message.manage',
      'message.post',
      'message.post-in-thread',
      'message.react',
      'message.read',
      'message.read.interactions',
      'role.assign',
      'role.manage',
      'room.ban-member',
      'room.create',
      'room.join',
      'room.list',
      'room.manage',
      'server.manage',
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

  it('derives inclusion from registered dotted ancestors', () => {
    const permissions = ['message.read', 'message.read.interactions', 'message.post-in-thread'];
    expect(getIncludedByPermission(permissions, 'message.read.interactions')).toBe('message.read');
    expect(getIncludedByPermission(permissions, 'message.read')).toBeNull();
    expect(getIncludedByPermission(permissions, 'message.post-in-thread')).toBeNull();
  });

  it('returns all registered dotted ancestors in order', () => {
    const permissions = [
      'server.manage',
      'server.manage.neighbors',
      'server.manage.neighbors.publish'
    ];
    expect(getIncludingPermissions(permissions, 'server.manage.neighbors.publish')).toEqual([
      'server.manage.neighbors',
      'server.manage'
    ]);
    expect(getIncludingPermissions(['server.manage'], 'server.manage.neighbors')).toEqual([]);
  });
});
