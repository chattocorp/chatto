import { describe, expect, it } from 'vitest';
import { RealtimeEventSchema } from '@chatto/api-types/realtime/v1/realtime_pb';
import { affectsViewerPermissions } from './permissionEvents';
import { create } from '@bufbuild/protobuf';

describe('permission event recipients', () => {
  it('keeps unrelated and cosmetic updates without reloading', () => {
    for (const event of [
      create(RealtimeEventSchema, {
        event: { case: 'roleUpdated', value: { roleName: 'helper' } }
      }),
      create(RealtimeEventSchema, {
        event: { case: 'rolesReordered', value: { roleNames: ['helper'] } }
      }),
      create(RealtimeEventSchema, {
        event: { case: 'roleAssigned', value: { userId: 'bob', roleName: 'helper' } }
      }),
      create(RealtimeEventSchema, {
        event: { case: 'rolePermissionsChanged', value: { roleName: 'other' } }
      })
    ])
      expect(affectsViewerPermissions(event, 'alice', ['helper'])).toBe(false);
  });

  it('reloads for own assignments, retained deleted roles, everyone, and direct decisions', () => {
    for (const event of [
      create(RealtimeEventSchema, {
        event: { case: 'roleAssigned', value: { userId: 'alice', roleName: 'new' } }
      }),
      create(RealtimeEventSchema, {
        event: { case: 'roleRevoked', value: { userId: 'alice', roleName: 'helper' } }
      }),
      create(RealtimeEventSchema, {
        event: { case: 'roleDeleted', value: { roleName: 'helper' } }
      }),
      create(RealtimeEventSchema, {
        event: { case: 'rolePermissionsChanged', value: { roleName: 'everyone' } }
      }),
      create(RealtimeEventSchema, { event: { case: 'viewerPermissionsChanged', value: {} } })
    ])
      expect(affectsViewerPermissions(event, 'alice', ['helper'])).toBe(true);
  });

  it('does not assume unknown viewer roles mean no role assignments', () => {
    const event = create(RealtimeEventSchema, {
      event: { case: 'rolePermissionsChanged', value: { roleName: 'helper' } }
    });
    expect(affectsViewerPermissions(event, 'alice', undefined)).toBe(true);
    expect(affectsViewerPermissions(event, 'alice', [])).toBe(false);
  });
});
