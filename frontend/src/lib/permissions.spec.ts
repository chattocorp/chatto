import { describe, expect, it } from 'vitest';
import { PERMISSION_METADATA } from './permissions';

describe('PERMISSION_METADATA', () => {
  it('covers every current backend permission', () => {
    expect(Object.keys(PERMISSION_METADATA).sort()).toEqual([
      'admin.view-audit',
      'admin.view-system',
      'admin.view-users',
      'message.echo',
      'message.manage',
      'message.post',
      'message.post-in-thread',
      'message.react',
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
      'user.manage-permissions'
    ]);
  });

  it('does not list retired message edit/delete permissions', () => {
    expect(PERMISSION_METADATA).not.toHaveProperty('message.edit-own');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.edit-any');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.delete-own');
    expect(PERMISSION_METADATA).not.toHaveProperty('message.delete-any');
  });
});
