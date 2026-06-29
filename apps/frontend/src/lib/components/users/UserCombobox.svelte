<script lang="ts">
  import { onDestroy } from 'svelte';
  import { createMemberDirectoryAPI, type DirectoryMember } from '@chatto/api-client/memberDirectory';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { Combobox } from '$lib/ui/form';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { getAvatarInitials } from '$lib/utils/initials';

  type User = DirectoryMember;

  let {
    id,
    label,
    value = $bindable(''),
    text = $bindable(''),
    placeholder = 'Search users or enter an actor ID...'
  }: {
    id: string;
    label: string;
    value?: string;
    text?: string;
    placeholder?: string;
  } = $props();

  const connection = useConnection();

  let users = $state.raw<User[]>([]);
  let loading = $state(false);
  let requestId = 0;
  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  onDestroy(() => {
    if (searchTimer) clearTimeout(searchTimer);
  });

  function userLabel(user: User): string {
    const handle = user.login ? `@${user.login}` : user.id;
    return [user.displayName, handle].filter(Boolean).join(' ');
  }

  function scheduleSearch(query: string) {
    if (searchTimer) clearTimeout(searchTimer);
    const search = query.trim();
    const currentRequest = ++requestId;

    if (!search) {
      users = [];
      loading = false;
      return;
    }

    loading = true;
    searchTimer = setTimeout(() => {
      void searchUsers(search, currentRequest);
    }, 200);
  }

  async function searchUsers(search: string, currentRequest: number) {
    try {
      const currentConnection = connection();
      const api = createMemberDirectoryAPI({
        baseUrl: currentConnection.connectBaseUrl,
        bearerToken: currentConnection.bearerToken
      });
      const result = await api.listServerMembers(search, 10, 0);
      if (currentRequest !== requestId) return;
      users = result.members;
    } catch {
      if (currentRequest === requestId) {
        users = [];
      }
    } finally {
      if (currentRequest === requestId) {
        loading = false;
      }
    }
  }
</script>

<Combobox
  {id}
  {label}
  bind:value
  bind:text
  items={users}
  getValue={(user) => user.id}
  getLabel={userLabel}
  {placeholder}
  {loading}
  emptyMessage="No users found"
  clearLabel="Clear actor"
  ontextchange={scheduleSearch}
>
  {#snippet item({ item: user })}
    {#if user.avatarUrl}
      <SkeletonImg
        loading="lazy"
        src={user.avatarUrl}
        alt=""
        class="h-6 w-6 shrink-0 rounded-full object-cover"
      />
    {:else}
      <div
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-200 text-xs font-semibold text-muted"
      >
        {getAvatarInitials(user.displayName, user.login)}
      </div>
    {/if}
    <span class="min-w-0 truncate text-sm text-text">{user.displayName}</span>
    <span class="min-w-0 truncate text-sm text-muted">@{user.login}</span>
  {/snippet}
</Combobox>
