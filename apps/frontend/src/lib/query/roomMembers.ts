import type { InfiniteData, QueryKey } from '@tanstack/svelte-query';
import type {
  DirectoryMember,
  MemberDirectoryAPI,
  MemberDirectoryPage
} from '$lib/api-client/memberDirectory';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { registerRoomMemberQueryCache } from './cacheRegistry';
import { queryClient } from './client';
import { directoryQueryKeys } from './directory';

type RoomMemberQueryConnection = Pick<ServerConnection, 'queryScope'>;

export const ROOM_MEMBER_MANAGEMENT_PAGE_SIZE = 20;
export const ELIGIBLE_ROOM_MEMBER_LIMIT = 20;

export type RoomMembersQueryPage = MemberDirectoryPage & {
  /** Next server offset, kept independently from client-side deduplication. */
  nextOffset: number;
};

export type RoomMembersData = InfiniteData<RoomMembersQueryPage, unknown>;

export function roomMembersQueryPage(
  page: MemberDirectoryPage,
  pageParam: number
): RoomMembersQueryPage {
  return { ...page, nextOffset: pageParam + page.members.length };
}

export function nextRoomMembersPageParam(
  lastPage: RoomMembersQueryPage,
  lastPageParam: number
): number | undefined {
  return lastPage.hasMore && lastPage.nextOffset > lastPageParam ? lastPage.nextOffset : undefined;
}

/** Flatten offset pages without rendering rows repeated across a page boundary. */
export function flattenRoomMembers(data: RoomMembersData | undefined): DirectoryMember[] {
  const seen = new Set<string>();
  return (data?.pages ?? []).flatMap((page) =>
    page.members.filter((member) => {
      if (seen.has(member.id)) return false;
      seen.add(member.id);
      return true;
    })
  );
}

/**
 * Fill the room-member picker from directory pages, excluding users who are
 * already in the room. A page containing only existing members must not stop
 * the search before an eligible result on a later page.
 */
export async function listEligibleRoomMembers(
  api: Pick<MemberDirectoryAPI, 'listUsers' | 'batchGetRoomMembers'>,
  roomId: string,
  search: string,
  limit = ELIGIBLE_ROOM_MEMBER_LIMIT,
  signal?: AbortSignal
): Promise<DirectoryMember[]> {
  const eligible: DirectoryMember[] = [];
  const seenIds = new Set<string>();
  let offset = 0;
  let hasMore = true;

  while (hasMore && eligible.length < limit) {
    const page = await api.listUsers(search, limit, offset, { signal });
    const candidates = page.members.filter((member) => !member.deleted && !seenIds.has(member.id));
    for (const candidate of candidates) seenIds.add(candidate.id);

    const existing = candidates.length
      ? await api.batchGetRoomMembers(
          roomId,
          candidates.map((member) => member.id),
          { signal }
        )
      : [];
    const existingIds = new Set(existing.map((member) => member.id));
    eligible.push(...candidates.filter((member) => !existingIds.has(member.id)));

    hasMore = page.hasMore && page.members.length > 0;
    offset += page.members.length;
  }

  return eligible.slice(0, limit);
}

export function invalidateRoomMemberQueries(
  serverId: string,
  connection: RoomMemberQueryConnection,
  roomId: string
): Promise<void> {
  return queryClient.invalidateQueries({
    queryKey: directoryQueryKeys.roomMembers(serverId, connection, roomId),
    exact: true
  });
}

export function invalidateEligibleRoomMemberQueries(
  serverId: string,
  connection: RoomMemberQueryConnection,
  roomId: string
): Promise<void> {
  return queryClient.invalidateQueries({
    queryKey: directoryQueryKeys.room(serverId, connection, roomId),
    predicate: (query) => query.queryKey[7] === 'eligible-members'
  });
}

function emptyRoomQueryData(queryKey: QueryKey): RoomMembersData | DirectoryMember[] | undefined {
  if (queryKey[7] === 'members') return { pages: [], pageParams: [] };
  if (queryKey[7] === 'eligible-members') return [];
  return undefined;
}

function isRoomMemberQuery(queryKey: QueryKey, serverId: string, roomId: string): boolean {
  return (
    queryKey[0] === 'server' &&
    queryKey[1] === serverId &&
    queryKey[4] === 'directory' &&
    queryKey[5] === 'room' &&
    queryKey[6] === roomId &&
    (queryKey[7] === 'members' || queryKey[7] === 'eligible-members')
  );
}

function isAnyRoomMemberQuery(queryKey: QueryKey, serverId: string): boolean {
  return (
    queryKey[0] === 'server' &&
    queryKey[1] === serverId &&
    queryKey[4] === 'directory' &&
    queryKey[5] === 'room' &&
    typeof queryKey[6] === 'string' &&
    (queryKey[7] === 'members' || queryKey[7] === 'eligible-members')
  );
}

function purgeMatchingRoomMemberQueries(predicate: (queryKey: QueryKey) => boolean): void {
  const queries = queryClient.getQueryCache().findAll({
    predicate: (query) => predicate(query.queryKey)
  });
  for (const query of queries) {
    const empty = emptyRoomQueryData(query.queryKey);
    if (empty !== undefined) queryClient.setQueryData(query.queryKey, empty);
  }

  void queryClient
    .cancelQueries({ predicate: (query) => predicate(query.queryKey) }, { revert: false })
    .then(() =>
      queryClient.invalidateQueries({
        predicate: (query) => predicate(query.queryKey),
        refetchType: 'none'
      })
    );
}

/**
 * Remove every cached identity for a room and leave the snapshots dormant.
 * The authoritative admin-room reread remounts this panel for archived rooms;
 * deleted or inaccessible rooms remain unmounted and cannot race projection
 * authorization by refetching immediately after the event.
 * Cancellation uses `revert: false` so an older in-flight response cannot
 * restore the pre-event snapshot.
 */
export function purgeRoomMemberQueries(
  serverId: string,
  connection: RoomMemberQueryConnection,
  roomId: string
): void {
  const queryKey = directoryQueryKeys.room(serverId, connection, roomId);
  purgeMatchingRoomMemberQueries((candidate) =>
    queryKey.every((part, index) => candidate[index] === part)
  );
}

function purgeRoomMemberQueriesAcrossSessions(serverId: string, roomId: string): void {
  purgeMatchingRoomMemberQueries((queryKey) => isRoomMemberQuery(queryKey, serverId, roomId));
}

function invalidateRoomMemberQueriesAcrossSessions(serverId: string, roomId: string): void {
  void queryClient.invalidateQueries({
    predicate: (query) => isRoomMemberQuery(query.queryKey, serverId, roomId)
  });
}

function scrubRoomMemberUserAcrossSessions(serverId: string, userId: string): void {
  const predicate = (queryKey: QueryKey) => isAnyRoomMemberQuery(queryKey, serverId);
  const queries = queryClient.getQueryCache().findAll({
    predicate: (query) => predicate(query.queryKey)
  });

  for (const query of queries) {
    if (query.queryKey[7] === 'members') {
      queryClient.setQueryData<RoomMembersData>(query.queryKey, (current) => {
        if (!current) return current;
        const removed = current.pages.some((page) =>
          page.members.some((member) => member.id === userId)
        );
        if (!removed) return current;
        return {
          ...current,
          pages: current.pages.map((page) => ({
            ...page,
            members: page.members.filter((member) => member.id !== userId),
            totalCount: Math.max(0, page.totalCount - 1)
          }))
        };
      });
    } else {
      queryClient.setQueryData<DirectoryMember[]>(query.queryKey, (current) =>
        current?.filter((member) => member.id !== userId)
      );
    }
  }

  void queryClient
    .cancelQueries({ predicate: (query) => predicate(query.queryKey) }, { revert: false })
    .then(() =>
      queryClient.invalidateQueries({
        predicate: (query) => predicate(query.queryKey),
        refetchType: 'none'
      })
    );
}

registerRoomMemberQueryCache({
  invalidateRoom: invalidateRoomMemberQueriesAcrossSessions,
  purgeRoom: purgeRoomMemberQueriesAcrossSessions,
  scrubUser: scrubRoomMemberUserAcrossSessions
});
