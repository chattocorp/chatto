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

  it('defines the explicit message read inclusion without a general hierarchy', () => {
    const definitions = [
      { permission: 'message.read' },
      { permission: 'message.read.interactions', includedByPermission: 'message.read' }
    ];
    expect(getIncludedByPermission(definitions, 'message.read.interactions')).toBe('message.read');
    expect(getIncludedByPermission(definitions, 'message.read')).toBeNull();
    expect(getIncludedByPermission(definitions, 'message.post-in-thread')).toBeNull();
    expect(getIncludedByPermission(undefined, 'message.read.interactions')).toBeNull();
  });

  it('returns transitive inclusion without looping on invalid server metadata', () => {
    const definitions = [
      { permission: 'server.manage' },
      { permission: 'server.manage.neighbors', includedByPermission: 'server.manage' },
      {
        permission: 'server.manage.neighbors.publish',
        includedByPermission: 'server.manage.neighbors'
      }
    ];
    expect(getIncludingPermissions(definitions, 'server.manage.neighbors.publish')).toEqual([
      'server.manage.neighbors',
      'server.manage'
    ]);
    expect(
      getIncludingPermissions(
        [
          { permission: 'cycle.a', includedByPermission: 'cycle.b' },
          { permission: 'cycle.b', includedByPermission: 'cycle.a' }
        ],
        'cycle.a'
      )
    ).toEqual(['cycle.b']);
  });
});
