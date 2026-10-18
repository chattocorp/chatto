import { beforeAll, describe, expect, it } from 'vitest';
import { PERMISSION_DEFINITIONS } from '@chatto/client/util/permissionCatalog';
import { loadLocaleMessages } from './i18n/messages';
import {
  comparePermissionRows,
  getPermissionCategoryLabel,
  getPermissionDescription,
  PERMISSION_METADATA
} from './permissions';

// Permission help and category headings belong to the management catalogs.
beforeAll(() => loadLocaleMessages('en-GB'));

describe('PERMISSION_METADATA', () => {
  it('groups server invites separately from server settings', () => {
    expect(
      ['server.manage', 'invite-link.manage', 'room.manage', 'invite-link.create'].sort(
        comparePermissionRows
      )
    ).toEqual(['room.manage', 'server.manage', 'invite-link.create', 'invite-link.manage']);
  });

  it('gives every catalogue permission localized text', () => {
    expect(Object.keys(PERMISSION_METADATA).sort()).toEqual(
      Object.keys(PERMISSION_DEFINITIONS).sort()
    );
    for (const [permission, metadata] of Object.entries(PERMISSION_METADATA)) {
      expect(metadata.description(), permission).not.toMatch(/^⟦/);
      expect(metadata.help(), permission).not.toMatch(/^⟦/);
      expect(metadata.scopes, permission).toBe(PERMISSION_DEFINITIONS[permission].scopes);
    }
  });

  it('falls back to the ID for unknown permissions', () => {
    expect(getPermissionDescription('future.permission')).toBe('future.permission');
  });

  it('labels permission categories', () => {
    expect(getPermissionCategoryLabel('server')).toBe('Server');
    expect(getPermissionCategoryLabel('server-invite')).toBe('Server invites');
    expect(getPermissionCategoryLabel('other')).toBe('Other');
  });
});
