import { describe, expect, it } from 'vitest';
import { PERMISSION_DEFINITIONS } from '@chatto/client/util/permissionCatalog';
import {
  getPermissionCategoryLabel,
  getPermissionDescription,
  PERMISSION_METADATA
} from './permissions';

describe('PERMISSION_METADATA', () => {
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
