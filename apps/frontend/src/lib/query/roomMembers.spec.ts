import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type {
  DirectoryMember,
  MemberDirectoryAPI,
  MemberDirectoryPage
} from '$lib/api-client/memberDirectory';
import { queryClient } from './client';
import {
  invalidateRegisteredRoomMemberQueries,
  purgeRegisteredRoomMemberQueries,
  scrubRegisteredRoomMemberUser
} from './cacheRegistry';
import { directoryQueryKeys } from './directory';
import {
  flattenRoomMembers,
  listEligibleRoomMembers,
  nextRoomMembersPageParam,
  purgeRoomMemberQueries,
  roomMembersQueryPage,
  type RoomMembersData
} from './roomMembers';

function member(id: string, overrides: Partial<DirectoryMember> = {}): DirectoryMember {
  return {
    id,
    login: id,
    displayName: id.toUpperCase(),
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE,
    customStatus: null,
    roles: ['everyone'],
    createdAt: null,
    ...overrides
  };
}

function page(
  members: DirectoryMember[],
  totalCount = members.length,
  hasMore = false
): MemberDirectoryPage {
  return { members, totalCount, hasMore };
}

function data(...pages: MemberDirectoryPage[]): RoomMembersData {
  return {
    pages: pages.map((value, index) => ({
      ...value,
      nextOffset: index * 20 + value.members.length
    })),
    pageParams: pages.map((_, index) => index * 20)
  };
}

describe('room member queries', () => {
  afterEach(() => queryClient.clear());

  it('flattens offset pages without duplicate member rows', () => {
    expect(
      flattenRoomMembers(
        data(page([member('alice')], 2, true), page([member('alice'), member('bob')], 2, false))
      ).map((candidate) => candidate.id)
    ).toEqual(['alice', 'bob']);
  });

  it('advances pagination by the raw server page length', () => {
    const queryPage = roomMembersQueryPage(page([member('alice'), member('bob')], 3, true), 20);

    expect(queryPage.nextOffset).toBe(22);
    expect(nextRoomMembersPageParam(queryPage, 20)).toBe(22);
    expect(nextRoomMembersPageParam({ ...queryPage, hasMore: false }, 20)).toBeUndefined();
  });

  it('continues directory paging when the first page has only existing members', async () => {
    const existing = Array.from({ length: 20 }, (_, index) => member(`existing-${index}`));
    const bob = member('bob');
    const signal = new AbortController().signal;
    const listUsers = vi
      .fn()
      .mockResolvedValueOnce(page(existing, 21, true))
      .mockResolvedValueOnce(page([bob], 21, false));
    const batchGetRoomMembers = vi.fn().mockResolvedValueOnce(existing).mockResolvedValueOnce([]);

    await expect(
      listEligibleRoomMembers(
        { listUsers, batchGetRoomMembers } as Pick<
          MemberDirectoryAPI,
          'listUsers' | 'batchGetRoomMembers'
        >,
        'room-1',
        'b',
        20,
        signal
      )
    ).resolves.toEqual([bob]);

    expect(listUsers).toHaveBeenNthCalledWith(1, 'b', 20, 0, { signal });
    expect(listUsers).toHaveBeenNthCalledWith(2, 'b', 20, 20, { signal });
    expect(batchGetRoomMembers).toHaveBeenNthCalledWith(
      1,
      'room-1',
      existing.map((candidate) => candidate.id),
      { signal }
    );
  });

  it('clears a removed room before its authorization is revalidated', async () => {
    const connection = { queryScope: 'session-1' };
    const queryKey = directoryQueryKeys.roomMembers('server-1', connection, 'room-1');
    queryClient.setQueryData(queryKey, data(page([member('private-user')])));

    purgeRoomMemberQueries('server-1', connection, 'room-1');

    expect(flattenRoomMembers(queryClient.getQueryData(queryKey))).toEqual([]);
    await vi.waitFor(() => expect(queryClient.getQueryState(queryKey)?.isInvalidated).toBe(true));
  });

  it('purges retained room identities across every cached session', () => {
    const first = directoryQueryKeys.roomMembers('server-1', { queryScope: 'session-1' }, 'room-1');
    const second = directoryQueryKeys.roomMembers(
      'server-1',
      { queryScope: 'session-2' },
      'room-1'
    );
    const unrelated = directoryQueryKeys.roomMembers(
      'server-1',
      { queryScope: 'session-1' },
      'room-2'
    );
    queryClient.setQueryData(first, data(page([member('private-1')])));
    queryClient.setQueryData(second, data(page([member('private-2')])));
    queryClient.setQueryData(unrelated, data(page([member('public')])));

    purgeRegisteredRoomMemberQueries('server-1', 'room-1');

    expect(flattenRoomMembers(queryClient.getQueryData(first))).toEqual([]);
    expect(flattenRoomMembers(queryClient.getQueryData(second))).toEqual([]);
    expect(flattenRoomMembers(queryClient.getQueryData(unrelated))).toEqual([member('public')]);
  });

  it('marks off-screen room-member snapshots stale after a projected update', () => {
    const queryKey = directoryQueryKeys.roomMembers(
      'server-1',
      { queryScope: 'session-1' },
      'room-1'
    );
    queryClient.setQueryData(queryKey, data(page([member('alice')])));

    invalidateRegisteredRoomMemberQueries('server-1', 'room-1');

    expect(queryClient.getQueryState(queryKey)?.isInvalidated).toBe(true);
  });

  it('scrubs a removed user from member and eligible-user caches', () => {
    const connection = { queryScope: 'session-1' };
    const membersKey = directoryQueryKeys.roomMembers('server-1', connection, 'room-1');
    const eligibleKey = directoryQueryKeys.eligibleRoomMembers(
      'server-1',
      connection,
      'room-2',
      'a',
      20
    );
    queryClient.setQueryData(
      membersKey,
      data(page([member('removed'), member('retained')], 2, false))
    );
    queryClient.setQueryData(eligibleKey, [member('removed'), member('candidate')]);

    scrubRegisteredRoomMemberUser('server-1', 'removed');

    expect(flattenRoomMembers(queryClient.getQueryData(membersKey))).toEqual([member('retained')]);
    expect(queryClient.getQueryData(eligibleKey)).toEqual([member('candidate')]);
    expect(queryClient.getQueryData<RoomMembersData>(membersKey)?.pages[0]?.totalCount).toBe(1);
  });
});
