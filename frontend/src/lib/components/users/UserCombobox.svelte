<script lang="ts">
  import { onDestroy } from 'svelte';
  import { graphql } from '$lib/gql';
  import type { UserComboboxMembersQuery } from '$lib/gql/graphql';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { Combobox } from '$lib/ui/form';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { getAvatarInitials } from '$lib/utils/initials';

  type User = UserComboboxMembersQuery['server']['members']['users'][number];

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

  const MembersQuery = graphql(`
    query UserComboboxMembers($search: String) {
      server {
        members(search: $search, limit: 10, offset: 0) {
          users {
            id
            login
            displayName
            avatarUrl
          }
        }
      }
    }
  `);

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
      const result = await connection()
        .client.query(MembersQuery, { search }, { requestPolicy: 'network-only' })
        .toPromise();
      if (currentRequest !== requestId) return;
      users = result.data?.server.members.users ?? [];
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
