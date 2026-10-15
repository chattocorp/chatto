import { describe, expect, it } from 'vitest';
import { PERMISSION_DEFINITIONS } from '@chatto/client/util/permissionCatalog';
import {
  comparePermissionRows,
  getPermissionCategoryLabel,
  getPermissionDescription,
  PERMISSION_METADATA
} from './permissions';

describe('PERMISSION_METADATA', () => {
  it('keeps invite links and other server permissions in one category', () => {
    expect(
      ['server.manage', 'invite-link.manage', 'room.manage', 'invite-link.create'].sort(
        comparePermissionRows
      )
    ).toEqual(['room.manage', 'invite-link.create', 'invite-link.manage', 'server.manage']);
  });

  it('gives every catalogue permission localized text', () => {
    expect(Object.keys(PERMISSION_METADATA).sort()).toEqual(
      Object.keys(PERMISSION_DEFINITIONS).sort()
    );
    for (const [permission, metadata] of Object.entries(PERMISSION_METADATA)) {
      expect(metadata.description(), permission).not.toMatch(/^rbac\./);
      expect(metadata.help(), permission).not.toMatch(/^rbac\./);
      expect(metadata.scopes, permission).toBe(PERMISSION_DEFINITIONS[permission].scopes);
    }
  });

  it('falls back to the ID for unknown permissions', () => {
    expect(getPermissionDescription('future.permission')).toBe('future.permission');
  });

  it('labels permission categories', () => {
    expect(getPermissionCategoryLabel('server')).toBe('Server');
    expect(getPermissionCategoryLabel('other')).toBe('Other');
  });
});
