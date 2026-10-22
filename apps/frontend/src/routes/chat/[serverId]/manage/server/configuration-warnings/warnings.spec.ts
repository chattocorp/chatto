import { describe, expect, it } from 'vitest';
import type { MatrixDecision, RolePermissionMatrix } from '@chatto/client/api/permissions';
import { roomsWithoutRead } from './warnings';

function matrix(post: MatrixDecision, read: MatrixDecision): RolePermissionMatrix {
  return {
    roleName: 'member',
    applicablePermissions: ['message.post', 'message.read'],
    scopes: ['ROOM', 'GROUP', 'SERVER', 'DM'].map((kind) => ({
      id: kind,
      label: kind,
      kind: kind as 'ROOM' | 'GROUP' | 'SERVER' | 'DM',
      parentGroupId: ''
    })),
    cells: ['ROOM', 'GROUP', 'SERVER', 'DM'].flatMap((scopeId) => [
      { scopeId, permission: 'message.post', override: 'NONE', effective: post },
      { scopeId, permission: 'message.read', override: 'NONE', effective: read }
    ])
  };
}

describe('roomsWithoutRead', () => {
  it.each(['NONE', 'DENY'] as const)(
    'flags inherited posting with %s reading only in rooms',
    (read) => {
      expect(roomsWithoutRead(matrix('ALLOW', read))?.map((scope) => scope.id)).toEqual(['ROOM']);
    }
  );
  it('uses effective decisions instead of explicit overrides', () => {
    const data = matrix('ALLOW', 'ALLOW');
    data.cells[1].override = 'DENY';
    expect(roomsWithoutRead(data)).toEqual([]);
    data.cells[0].override = 'ALLOW';
    data.cells[0].effective = 'DENY';
    data.cells[1].effective = 'NONE';
    expect(roomsWithoutRead(data)).toEqual([]);
  });
  it.each(['NONE', 'DENY'] as const)('ignores %s posting', (post) => {
    expect(roomsWithoutRead(matrix(post, 'NONE'))).toEqual([]);
  });
  it('rejects incomplete room decisions instead of reporting no warnings', () => {
    const data = matrix('ALLOW', 'NONE');
    data.cells = data.cells.filter((cell) => cell.permission !== 'message.read');
    expect(roomsWithoutRead(data)).toBeNull();
  });
});
