import { Code, ConnectError } from '$lib/api-client/connect';
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { createRoomDirectoryAPI } from '$lib/api-client/roomDirectory';
import type { UserPermissionMatrix } from '$lib/api-client/permissions';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { MatrixData, MatrixScope } from './SubjectPermissionsMatrix.svelte';

/** Compose account membership from room resources, without extending permission DTOs.
 * Reads are bounded and abort with the owning account query. A forbidden lookup
 * leaves the cell unavailable; it must never appear as a known non-membership.
 */
export async function loadAccountMemberships(
  connection: Pick<ServerConnection, 'getAPI'>,
  matrix: UserPermissionMatrix,
  isBot: boolean,
  canManageAccounts: boolean,
  signal: AbortSignal
): Promise<MatrixData & Pick<UserPermissionMatrix, 'userId' | 'page'>> {
  const scopes: MatrixScope[] = matrix.scopes.map((scope) => ({ ...scope }));
  const roomScopes = scopes.filter((scope) => scope.kind === 'ROOM');
  const joinableScopes = new Set(
    matrix.cells
      .filter((cell) => cell.permission === 'room.join' && cell.effective === 'ALLOW')
      .map((cell) => cell.scopeId)
  );
  const directory = connection.getAPI(createRoomDirectoryAPI);
  const members = connection.getAPI(createMemberDirectoryAPI);
  // RoomDirectory's batch contract accepts at most 100 room IDs.
  for (let offset = 0; offset < roomScopes.length; offset += 100) {
    signal.throwIfAborted();
    const batch = roomScopes.slice(offset, offset + 100);
    const rooms = await directory.batchGetRooms(
      batch.map((scope) => scope.id.slice('room:'.length)),
      { signal }
    );
    const byID = new Map(rooms.map((room) => [room.id, room]));
    let next = 0;
    async function worker() {
      while (next < batch.length) {
        signal.throwIfAborted();
        const scope = batch[next++];
        const roomID = scope.id.slice('room:'.length);
        const room = byID.get(roomID);
        if (!room) continue;
        try {
          const joined =
            (await members.batchGetRoomMembers(roomID, [matrix.userId], { signal })).length > 0;
          const manager = canManageAccounts || room.canManageRoom;
          const allowed = joinableScopes.has(scope.id);
          scope.membership = {
            joined,
            automatic: room.isUniversal,
            canJoin:
              !joined && !room.isUniversal && !room.archived && (manager || (isBot && allowed)),
            canLeave: joined && !room.isUniversal && (manager || isBot)
          };
        } catch (error) {
          if (error instanceof ConnectError && error.code === Code.PermissionDenied) continue;
          throw error;
        }
      }
    }
    await Promise.all(Array.from({ length: Math.min(6, batch.length) }, worker));
  }
  return { ...matrix, scopes };
}
