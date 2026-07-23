<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import type { DirectoryMember } from '$lib/api-client/memberDirectory';
  import { DataTable, Panel } from '$lib/components/admin';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import { Button, Combobox } from '$lib/ui/form';
  import { useProjectionEvent } from '$lib/hooks';
  import { toast } from '$lib/ui/toast';
  import { getAvatarInitials } from '$lib/utils/initials';
  import * as m from '$lib/i18n/messages';
  import { RoomMemberManagementStore } from './RoomMemberManagementStore.svelte';

  let {
    serverId,
    roomId,
    roomName,
    isUniversal,
    archived,
    canManageMembers,
    scrollRoot,
    store
  }: {
    serverId: string;
    roomId: string;
    roomName: string;
    isUniversal: boolean;
    archived: boolean;
    canManageMembers: boolean;
    scrollRoot?: HTMLElement;
    store: RoomMemberManagementStore;
  } = $props();

  let selectedUser = $state<DirectoryMember | null>(null);
  let selectedUserId = $state('');
  let selectedUserText = $state('');
  let removeCandidate = $state<DirectoryMember | null>(null);
  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  const canEditMembership = $derived(canManageMembers && !isUniversal && !archived);
  const columns = $derived(canEditMembership ? 3 : 2);

  $effect(() => {
    const selectedServerId = serverId;
    const selectedRoomId = roomId;
    untrack(() => {
      clearLocalState();
      store.setRoom(selectedServerId, selectedRoomId);
      void store.loadFirstPage();
    });
  });

  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'roomUpsert':
          if (operation.operation.value.room?.room?.id === roomId) {
            void store.refresh();
            return;
          }
          break;
        case 'roomRemove':
          if (operation.operation.value.roomId === roomId) {
            clearLocalState();
            if (store.clearRoom(serverId, roomId)) void store.loadFirstPage();
            return;
          }
          break;
      }
    }
  });

  onDestroy(() => {
    if (searchTimer) clearTimeout(searchTimer);
  });

  function memberLabel(member: DirectoryMember): string {
    return `${member.displayName} @${member.login}`;
  }

  function scheduleDirectorySearch(text: string): void {
    selectedUser = null;
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => void store.searchDirectory(text), 200);
  }

  function clearLocalState(): void {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = null;
    selectedUser = null;
    selectedUserId = '';
    selectedUserText = '';
    removeCandidate = null;
  }

  function clearSelectedUser(): void {
    clearLocalState();
    store.clearDirectorySearch();
  }

  async function addSelectedMember(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!selectedUser || !canEditMembership || store.addingUserId) return;
    const user = selectedUser;
    try {
      if (!(await store.addMember(user))) return;
      clearSelectedUser();
      toast.success(m['admin.rooms_admin.member_added']({ name: user.displayName }));
    } catch (error) {
      toast.error(
        m['admin.rooms_admin.add_member_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  async function confirmRemoveMember(): Promise<void> {
    const user = removeCandidate;
    if (!user || !canEditMembership) return;
    try {
      if (!(await store.removeMember(user))) return;
      removeCandidate = null;
      toast.success(m['admin.rooms_admin.member_removed']({ name: user.displayName }));
    } catch (error) {
      toast.error(
        m['admin.rooms_admin.remove_member_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }
</script>

<Panel
  title={m['admin.nav.members']()}
  icon="iconify uil--users-alt"
  count={store.totalCount}
  noPadding
>
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
          items={store.directoryResults}
          getValue={(user) => user.id}
          getLabel={memberLabel}
          loading={store.directoryLoading}
          error={store.directoryError ?? undefined}
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
        disabled={!selectedUser}
        loading={!!store.addingUserId}
        loadingText={m['admin.rooms_admin.adding_member']()}
      >
        {m['admin.rooms_admin.add_member']()}
      </Button>
    </form>
  {/if}

  {#if store.loadError}
    <div class="border-b border-border p-5">
      <Hint tone="danger">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{m['admin.rooms_admin.load_members_failed']({ error: store.loadError })}</span>
          <Button variant="secondary" size="sm" onclick={() => void store.refresh()}>
            {m['common.retry']()}
          </Button>
        </div>
      </Hint>
    </div>
  {/if}

  {#if store.loading && store.members.length === 0}
    <div class="p-5 text-muted">{m['admin.members.loading']()}</div>
  {:else}
    <DataTable
      items={store.members}
      {columns}
      emptyMessage={m['admin.members.empty']()}
      hasMore={store.hasMore && !store.loadError}
      loadingMore={store.loadingMore}
      onLoadMore={() => store.loadMore()}
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
              disabled={!!store.addingUserId || !!store.removingUserId}
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
    loading={store.removingUserId === removeCandidate.id}
    onconfirm={() => void confirmRemoveMember()}
    onclose={() => (removeCandidate = null)}
  >
    {m['admin.rooms_admin.remove_member_prompt']({
      name: removeCandidate.displayName,
      room: `#${roomName}`
    })}
  </ConfirmDialog>
{/if}
