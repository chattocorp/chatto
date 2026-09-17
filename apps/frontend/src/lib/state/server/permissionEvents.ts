import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';

/** Decide cache invalidation from public subjects, without resolving permissions locally.
 * Unknown viewer roles require a conservative reload. Role removal is checked
 * against retained roles before the membership event is applied.
 */
export function affectsViewerPermissions(
  event: RealtimeEvent,
  viewerId: string | null,
  roles: readonly string[] | undefined
): boolean {
  const payload = event.event;
  switch (payload.case) {
    case 'viewerPermissionsChanged':
      return true;
    case 'roleAssigned':
    case 'roleRevoked':
      return !viewerId || payload.value.userId === viewerId;
    case 'roleDeleted':
    case 'rolePermissionsChanged':
      return (
        payload.value.roleName === 'everyone' ||
        roles === undefined ||
        roles.includes(payload.value.roleName)
      );
    default:
      return false;
  }
}
