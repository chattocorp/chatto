import { serverSessionQueryRoot, type QueryKeyScope } from './keys';

function directoryRoot(scope: QueryKeyScope) {
  return [...serverSessionQueryRoot(scope), 'directory'] as const;
}

export const directoryQueryKeys = {
  root: directoryRoot,
  users(scope: QueryKeyScope, search: string, limit: number) {
    return [...directoryRoot(scope), 'users', { search, limit }] as const;
  },
  room(scope: QueryKeyScope, roomId: string) {
    return [...directoryRoot(scope), 'room', roomId] as const;
  },
  roomMembers(scope: QueryKeyScope, roomId: string) {
    return [...directoryQueryKeys.room(scope, roomId), 'members'] as const;
  },
  eligibleRoomMembers(scope: QueryKeyScope, roomId: string, search: string, limit: number) {
    return [
      ...directoryQueryKeys.room(scope, roomId),
      'eligible-members',
      { search, limit }
    ] as const;
  }
};
