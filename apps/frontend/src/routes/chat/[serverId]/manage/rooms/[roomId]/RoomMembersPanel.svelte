<script lang="ts">
  import { createInfiniteQuery, createMutation, createQuery } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import type { DirectoryMember } from '$lib/api-client/memberDirectory';
  import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { DataTable, Panel } from '$lib/components/admin';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import { Button, Combobox } from '$lib/ui/form';
  import { useProjectionEvent } from '$lib/hooks';
  import { toast } from '$lib/ui/toast';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';
  import { queryClient } from '$lib/query/client';
  import { directoryQueryKeys } from '$lib/query/directory';
  import {
    ELIGIBLE_ROOM_MEMBER_LIMIT,
    flattenRoomMembers,
    invalidateEligibleRoomMemberQueries,
    invalidateRoomMemberQueries,
    listEligibleRoomMembers,
    nextRoomMembersPageParam,
    purgeRoomMemberQueries,
    ROOM_MEMBER_MANAGEMENT_PAGE_SIZE,
    roomMembersQueryPage
  } from '$lib/query/roomMembers';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import * as m from '$lib/i18n/messages';

  let {
    serverId,
    roomId,
    roomName,
    isUniversal,
    archived,
    canManageMembers,
    scrollRoot
  }: {
    serverId: string;
    roomId: string;
    roomName: string;
    isUniversal: boolean;
    archived: boolean;
    canManageMembers: boolean;
    scrollRoot?: HTMLElement;
  } = $props();

  const serverScope = useServerScope();

  let selectedUser = $state<DirectoryMember | null>(null);
  let selectedUserId = $state('');
  let selectedUserText = $state('');
  let removeCandidate = $state<DirectoryMember | null>(null);
  let activeDirectorySearch = $state('');
  let directoryDebouncePending = $state(false);
  let privacyGeneration = 0;
  let disposed = false;
  const searchDebounce = useDebounce();

  const canEditMembership = $derived(canManageMembers && !isUniversal && !archived);
  const columns = $derived(canEditMembership ? 3 : 2);

  const membersQuery = createInfiniteQuery(
    () => {
      const connection = serverScope.connection;
      const targetServerId = serverId;
      const targetRoomId = roomId;
      return {
        queryKey: directoryQueryKeys.roomMembers(targetServerId, connection, targetRoomId),
        queryFn: async ({ pageParam, signal }) => {
          const page = await connection
            .getAPI(createMemberDirectoryAPI)
            .listRoomMembers(targetRoomId, '', ROOM_MEMBER_MANAGEMENT_PAGE_SIZE, pageParam, {
              signal
            });
          return roomMembersQueryPage(page, pageParam);
        },
        initialPageParam: 0,
        getNextPageParam: (lastPage, _pages, lastPageParam) =>
          nextRoomMembersPageParam(lastPage, lastPageParam)
      };
    },
    () => queryClient
  );

  const eligibleMembersQuery = createQuery(
    () => {
      const connection = serverScope.connection;
      const targetServerId = serverId;
      const targetRoomId = roomId;
      const search = activeDirectorySearch;
      return {
        queryKey: directoryQueryKeys.eligibleRoomMembers(
          targetServerId,
          connection,
          targetRoomId,
          search,
          ELIGIBLE_ROOM_MEMBER_LIMIT
        ),
        queryFn: ({ signal }) =>
          listEligibleRoomMembers(
            connection.getAPI(createMemberDirectoryAPI),
            targetRoomId,
            search,
            ELIGIBLE_ROOM_MEMBER_LIMIT,
            signal
          ),
        enabled: search.length > 0
      };
    },
    () => queryClient
  );

  type MemberMutationScope = {
    serverId: string;
    roomId: string;
    connection: ServerConnection;
    privacyGeneration: number;
    user: DirectoryMember;
  };

  const addMemberMutation = createMutation(
    () => ({
      mutationFn: (target: MemberMutationScope) =>
        target.connection
          .getAPI(createRoomCommandAPI)
          .addMember({ roomId: target.roomId, userId: target.user.id })
    }),
    () => queryClient
  );

  const removeMemberMutation = createMutation(
    () => ({
      mutationFn: (target: MemberMutationScope) =>
        target.connection
          .getAPI(createRoomCommandAPI)
          .removeMember({ roomId: target.roomId, userId: target.user.id })
    }),
    () => queryClient
  );

  const members = $derived(flattenRoomMembers(membersQuery.data));
  const totalCount = $derived(membersQuery.data?.pages.at(-1)?.totalCount ?? 0);
  const hasMore = $derived(membersQuery.hasNextPage);
  const loading = $derived(membersQuery.isPending);
  const loadingMore = $derived(membersQuery.isFetchingNextPage);
  const loadError = $derived(
    membersQuery.error instanceof Error
      ? membersQuery.error.message
      : membersQuery.error
        ? String(membersQuery.error)
        : null
  );
  const directoryResults = $derived(
    activeDirectorySearch && !directoryDebouncePending ? (eligibleMembersQuery.data ?? []) : []
  );
  const directoryLoading = $derived(
    directoryDebouncePending || (!!activeDirectorySearch && eligibleMembersQuery.isFetching)
  );
  const directoryError = $derived(
    eligibleMembersQuery.error instanceof Error
      ? eligibleMembersQuery.error.message
      : eligibleMembersQuery.error
        ? String(eligibleMembersQuery.error)
        : null
  );
  const addingUserId = $derived(
    addMemberMutation.isPending && isCurrentTarget(addMemberMutation.variables)
      ? (addMemberMutation.variables?.user.id ?? null)
      : null
  );
  const removingUserId = $derived(
    removeMemberMutation.isPending && isCurrentTarget(removeMemberMutation.variables)
      ? (removeMemberMutation.variables?.user.id ?? null)
      : null
  );

  onDestroy(() => {
    disposed = true;
    privacyGeneration += 1;
    searchDebounce.cancel();
  });

  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'roomUpsert':
          if (operation.operation.value.room?.room?.id === roomId) {
            void invalidateRoomMemberQueries(serverId, serverScope.connection, roomId);
            return;
          }
          break;
        case 'roomRemove':
          if (operation.operation.value.roomId === roomId) {
            privacyGeneration += 1;
            clearLocalState();
            purgeRoomMemberQueries(serverId, serverScope.connection, roomId);
            return;
          }
          break;
        case 'userRemove': {
          const userId = operation.operation.value.userId;
          const affectsSelection = selectedUser?.id === userId;
          const affectsRemoval = removeCandidate?.id === userId;
          const affectsMutation =
            addMemberMutation.variables?.user.id === userId ||
            removeMemberMutation.variables?.user.id === userId;
          if (affectsSelection || affectsRemoval || affectsMutation) privacyGeneration += 1;
          if (affectsSelection) clearSelectedUser();
          if (affectsRemoval) removeCandidate = null;
          break;
        }
      }
    }
  });

  function memberLabel(member: DirectoryMember): string {
    return `${member.displayName} @${member.login}`;
  }

  function scheduleDirectorySearch(text: string): void {
    selectedUser = null;
    const search = text.trim();
    searchDebounce.cancel();
    if (!search) {
      activeDirectorySearch = '';
      directoryDebouncePending = false;
      return;
    }
    directoryDebouncePending = true;
    searchDebounce.run(() => {
      activeDirectorySearch = search;
      directoryDebouncePending = false;
    }, 200);
  }

  function clearLocalState(): void {
    searchDebounce.cancel();
    selectedUser = null;
    selectedUserId = '';
    selectedUserText = '';
    removeCandidate = null;
    activeDirectorySearch = '';
    directoryDebouncePending = false;
  }

  function clearSelectedUser(): void {
    clearLocalState();
  }

  function mutationTarget(user: DirectoryMember): MemberMutationScope {
    return {
      serverId,
      roomId,
      connection: serverScope.connection,
      privacyGeneration,
      user
    };
  }

  function isCurrentTarget(target: MemberMutationScope | undefined): boolean {
    return (
      target !== undefined &&
      !disposed &&
      serverScope.isCurrent() &&
      target.serverId === serverId &&
      target.roomId === roomId &&
      target.connection.queryScope === serverScope.connection.queryScope &&
      target.privacyGeneration === privacyGeneration
    );
  }

  async function reconcileMembership(target: MemberMutationScope): Promise<void> {
    await invalidateRoomMemberQueries(target.serverId, target.connection, target.roomId);
    void invalidateEligibleRoomMemberQueries(target.serverId, target.connection, target.roomId);
  }

  async function addSelectedMember(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!selectedUser || !canEditMembership || addingUserId || removingUserId) return;
    const user = selectedUser;
    const target = mutationTarget(user);
    try {
      await addMemberMutation.mutateAsync(target);
      if (!isCurrentTarget(target)) return;
      queryClient.setQueryData<DirectoryMember[]>(
        directoryQueryKeys.eligibleRoomMembers(
          target.serverId,
          target.connection,
          target.roomId,
          activeDirectorySearch,
          ELIGIBLE_ROOM_MEMBER_LIMIT
        ),
        (current) => current?.filter((candidate) => candidate.id !== user.id)
      );
      await reconcileMembership(target);
      if (!isCurrentTarget(target)) return;
      clearSelectedUser();
      toast.success(m['admin.rooms_admin.member_added']({ name: user.displayName }));
    } catch (error) {
      if (!isCurrentTarget(target)) return;
      toast.error(
        m['admin.rooms_admin.add_member_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  async function confirmRemoveMember(): Promise<void> {
    const user = removeCandidate;
    if (!user || !canEditMembership || addingUserId || removingUserId) return;
    const target = mutationTarget(user);
    try {
      await removeMemberMutation.mutateAsync(target);
      if (!isCurrentTarget(target)) return;
      await reconcileMembership(target);
      if (!isCurrentTarget(target)) return;
      removeCandidate = null;
      toast.success(m['admin.rooms_admin.member_removed']({ name: user.displayName }));
    } catch (error) {
      if (!isCurrentTarget(target)) return;
      toast.error(
        m['admin.rooms_admin.remove_member_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  async function loadMore(): Promise<void> {
    if (loading || loadingMore || !hasMore) return;
    await membersQuery.fetchNextPage();
  }
</script>

<Panel title={m['admin.nav.members']()} icon="iconify uil--users-alt" count={totalCount} noPadding>
  {#if isUniversal}
    <div class="border-b border-border p-5">
      <Hint>{m['admin.rooms_admin.universal_members_description']()}</Hint>
    </div>
  {:else if archived}
    <div class="border-b border-border p-5">
      <Hint>{m['admin.rooms_admin.archived_members_description']()}</Hint>
    </div>
  {:else if canManageMembers}
    <form
      class="flex flex-col items-end gap-3 border-b border-border p-5 sm:flex-row"
      onsubmit={addSelectedMember}
    >
      <div class="w-full sm:max-w-md">
        <Combobox
          id="room-member-picker"
          label={m['admin.rooms_admin.add_member']()}
          placeholder={m['admin.members.search_placeholder']()}
          bind:value={selectedUserId}
          bind:text={selectedUserText}
          items={directoryResults}
          getValue={(user) => user.id}
          getLabel={memberLabel}
          loading={directoryLoading}
          error={directoryError ?? undefined}
          allowFreeform={false}
          emptyMessage={m['admin.users.empty']()}
          clearLabel={m['common.clear']()}
          ontextchange={scheduleDirectorySearch}
          onselect={(user) => (selectedUser = user)}
          onclear={clearSelectedUser}
        >
          {#snippet item({ item: user })}
            {#if user.avatarUrl}
              <SkeletonImg
                loading="lazy"
                src={user.avatarUrl}
                alt=""
                class="h-8 w-8 shrink-0 rounded-full object-cover"
              />
            {:else}
              <div
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-sm font-semibold text-muted"
              >
                {getAvatarInitials(user.displayName, user.login)}
              </div>
            {/if}
            <span class="min-w-0 truncate">{user.displayName}</span>
            <span class="min-w-0 truncate text-muted">@{user.login}</span>
          {/snippet}
        </Combobox>
      </div>
      <Button
        type="submit"
        disabled={!selectedUser || !!removingUserId}
        loading={!!addingUserId}
        loadingText={m['admin.rooms_admin.adding_member']()}
      >
        {m['admin.rooms_admin.add_member']()}
      </Button>
    </form>
  {/if}

  {#if loadError}
    <div class="border-b border-border p-5">
      <Hint tone="danger">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{m['admin.rooms_admin.load_members_failed']({ error: loadError })}</span>
          <Button variant="secondary" size="sm" onclick={() => void membersQuery.refetch()}>
            {m['common.retry']()}
          </Button>
        </div>
      </Hint>
    </div>
  {/if}

  {#if loading && members.length === 0}
    <div class="p-5 text-muted">{m['admin.members.loading']()}</div>
  {:else}
    <DataTable
      items={members}
      {columns}
      emptyMessage={m['admin.members.empty']()}
      hasMore={hasMore && !loadError}
      {loadingMore}
      onLoadMore={loadMore}
      loadMoreRoot={scrollRoot}
      loadingMoreMessage={m['admin.members.loading_more']()}
      hoverable={false}
    >
      {#snippet header()}
        <th class="table-header-cell">{m['admin.common.user']()}</th>
        <th class="table-header-cell">{m['admin.users.login']()}</th>
        {#if canEditMembership}
          <th class="table-header-cell text-right">
            <span class="sr-only">{m['admin.rooms_admin.remove_member']()}</span>
          </th>
        {/if}
      {/snippet}
      {#snippet row(member)}
        <td class="px-4 py-3">
          <div class="flex min-w-0 items-center gap-3">
            {#if member.avatarUrl}
              <SkeletonImg
                loading="lazy"
                src={member.avatarUrl}
                alt=""
                class="h-8 w-8 shrink-0 rounded-full object-cover"
              />
            {:else}
              <div
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface text-sm font-semibold text-muted"
              >
                {getAvatarInitials(member.displayName, member.login)}
              </div>
            {/if}
            <span class="min-w-0 truncate font-medium text-text-top">{member.displayName}</span>
          </div>
        </td>
        <td class="px-4 py-3 text-muted">@{member.login}</td>
        {#if canEditMembership}
          <td class="px-4 py-3 text-right">
            <Button
              variant="danger-secondary"
              size="sm"
              disabled={!!addingUserId || !!removingUserId}
              onclick={() => (removeCandidate = member)}
            >
              {m['admin.rooms_admin.remove_member']()}
            </Button>
          </td>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</Panel>

{#if removeCandidate}
  <ConfirmDialog
    title={m['admin.rooms_admin.remove_member']()}
    actionLabel={m['admin.rooms_admin.remove_member']()}
    actionIcon="iconify uil--user-minus"
    loading={removingUserId === removeCandidate.id}
    onconfirm={() => void confirmRemoveMember()}
    onclose={() => (removeCandidate = null)}
  >
    {m['admin.rooms_admin.remove_member_prompt']({
      name: removeCandidate.displayName,
      room: `#${roomName}`
    })}
  </ConfirmDialog>
{/if}
