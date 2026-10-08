import {
  connectPost,
  expectPermissionDecisionUpdate,
  type E2EPermissionDecisionUpdateResponse
} from './connectHelpers';
import { withBootstrapAdminRequest } from './adminRequest';

/**
 * The permissions that open rooms to everyone. They match the room-scope
 * defaults of the seeded #general room (`DefaultOpenRoomEveryonePermissions`
 * in the Go core).
 */
export const OPEN_ROOM_EVERYONE_PERMISSIONS = [
  'room.list',
  'room.join',
  'message.read',
  'message.post',
  'message.attach',
  'message.react',
  'message.echo',
  'call.start',
  'call.join',
  'call.voice',
  'call.camera',
  'call.screenshare'
] as const;

/**
 * Allow the open-room permissions for everyone at server scope, as an operator
 * who opens the whole server would. New servers start closed (ADR-116): new
 * rooms and room groups give everyone no access. Most E2E tests exercise other
 * behavior in rooms that they create, so the `server` fixture opens each test
 * server. Tests of the real closed defaults opt out with
 * `test.use({ openServerToEveryone: false })`.
 */
export async function openServerToEveryone(baseURL: string): Promise<void> {
  await withBootstrapAdminRequest(baseURL, async (adminRequest) => {
    for (const permission of OPEN_ROOM_EVERYONE_PERMISSIONS) {
      const data = await connectPost<E2EPermissionDecisionUpdateResponse>(
        adminRequest,
        'chatto.admin.v1.AdminPermissionService/SetRolePermission',
        { roleName: 'everyone', permission, decision: 'PERMISSION_DECISION_ALLOW' }
      );
      expectPermissionDecisionUpdate(data, { permission, decision: 'PERMISSION_DECISION_ALLOW' });
    }
  });
}
