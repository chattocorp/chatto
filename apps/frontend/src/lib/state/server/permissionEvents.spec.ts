import { describe, expect, it } from 'vitest';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { affectsViewerPermissions } from './permissionEvents';

describe('permission event recipients', () => {
  it('keeps unrelated and cosmetic updates without reloading', () => {
    for (const event of [
      new RealtimeEvent({ event: { case: 'roleUpdated', value: { roleName: 'helper' } } }),
      new RealtimeEvent({ event: { case: 'rolesReordered', value: { roleNames: ['helper'] } } }),
      new RealtimeEvent({
        event: { case: 'roleAssigned', value: { userId: 'bob', roleName: 'helper' } }
      }),
      new RealtimeEvent({ event: { case: 'rolePermissionsChanged', value: { roleName: 'other' } } })
    ])
      expect(affectsViewerPermissions(event, 'alice', ['helper'])).toBe(false);
  });

  it('reloads for own assignments, retained deleted roles, everyone, and direct decisions', () => {
    for (const event of [
      new RealtimeEvent({
        event: { case: 'roleAssigned', value: { userId: 'alice', roleName: 'new' } }
      }),
      new RealtimeEvent({
        event: { case: 'roleRevoked', value: { userId: 'alice', roleName: 'helper' } }
      }),
      new RealtimeEvent({ event: { case: 'roleDeleted', value: { roleName: 'helper' } } }),
      new RealtimeEvent({
        event: { case: 'rolePermissionsChanged', value: { roleName: 'everyone' } }
      }),
      new RealtimeEvent({ event: { case: 'viewerPermissionsChanged', value: {} } })
    ])
      expect(affectsViewerPermissions(event, 'alice', ['helper'])).toBe(true);
  });

  it('does not assume unknown viewer roles mean no role assignments', () => {
    const event = new RealtimeEvent({
      event: { case: 'rolePermissionsChanged', value: { roleName: 'helper' } }
    });
    expect(affectsViewerPermissions(event, 'alice', undefined)).toBe(true);
    expect(affectsViewerPermissions(event, 'alice', [])).toBe(false);
  });
});
