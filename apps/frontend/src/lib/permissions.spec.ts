import { describe, expect, it } from 'vitest';
import {
  getIncludedByPermission,
  getIncludingPermissions,
  PERMISSION_METADATA
} from './permissions';

describe('PERMISSION_METADATA', () => {
  it('covers every current backend permission', () => {
    expect(Object.keys(PERMISSION_METADATA).sort()).toEqual([
      'audit.read',
      'message.attach',
      'message.echo',
      'message.manage',
      'message.post',
      'message.post.replies',
      'message.react',
      'message.read',
      'message.read.interactions',
      'role.manage',
      'role.manage.assignments',
      'room.create',
      'room.join',
      'room.list',
      'room.manage',
      'room.manage.bans',
      'server.manage',
      'user.delete',
      'user.delete.self',
      'user.invite',
      'user.manage',
      'user.manage.permissions',
      'user.read'
    ]);
  });

  it('does not list retired message edit/delete permissions', () => {
    expect(PERMISSION_METADATA).not.toHaveProperty('message.edit-own');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.edit-any');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.delete-own');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.delete-any');
  });

  it('derives inclusion from registered dotted ancestors', () => {
    const permissions = [
      'message.read',
      'message.read.interactions',
      'message.post',
      'message.post.replies',
      'role.manage',
      'role.manage.assignments',
      'room.manage',
      'room.manage.bans',
      'user.delete',
      'user.delete.self',
      'user.manage',
      'user.manage.permissions'
    ];
    expect(getIncludedByPermission(permissions, 'message.read.interactions')).toBe('message.read');
    expect(getIncludedByPermission(permissions, 'message.read')).toBeNull();
    expect(getIncludedByPermission(permissions, 'message.post.replies')).toBe('message.post');
    expect(getIncludedByPermission(permissions, 'role.manage.assignments')).toBe('role.manage');
    expect(getIncludedByPermission(permissions, 'room.manage.bans')).toBe('room.manage');
    expect(getIncludedByPermission(permissions, 'user.delete.self')).toBe('user.delete');
    expect(getIncludedByPermission(permissions, 'user.manage.permissions')).toBe('user.manage');
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
