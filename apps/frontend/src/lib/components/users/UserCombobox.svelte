<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { createMemberDirectoryAPI, type DirectoryMember } from '$lib/api-client/memberDirectory';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';
  import { queryClient } from '$lib/query/client';
  import { directoryQueryKeys } from '$lib/query/directory';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Combobox } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';

  type User = DirectoryMember;

  let {
    id,
    label,
    value = $bindable(''),
    text = $bindable(''),
    placeholder = m('admin.members.search_placeholder')
  }: {
    id: string;
    label: string;
    value?: string;
    text?: string;
    placeholder?: string;
  } = $props();

  const serverScope = useServerScope();

  const SEARCH_LIMIT = 10;
  let activeSearch = $state('');
  let debouncePending = $state(false);
  const searchDebounce = useDebounce();
  const usersQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const search = activeSearch;
      return {
        queryKey: directoryQueryKeys.users(serverId, connection, search, SEARCH_LIMIT),
        queryFn: ({ signal }) =>
          connection
            .getAPI(createMemberDirectoryAPI)
            .listUsers(search, SEARCH_LIMIT, 0, { signal }),
        enabled: search.length > 0
      };
    },
    () => queryClient
  );
  const users = $derived<User[]>(
    activeSearch && !debouncePending ? (usersQuery.data?.members ?? []) : []
  );
  const loading = $derived(debouncePending || (!!activeSearch && usersQuery.isFetching));

  function userLabel(user: User): string {
    const handle = user.login ? `@${user.login}` : user.id;
    return [user.displayName, handle].filter(Boolean).join(' ');
  }

  function scheduleSearch(query: string) {
    searchDebounce.cancel();
    const search = query.trim();

    if (!search) {
      activeSearch = '';
      debouncePending = false;
      return;
    }

    debouncePending = true;
    searchDebounce.run(() => {
      activeSearch = search;
      debouncePending = false;
    }, 200);
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
    <UserAvatar {user} size="xs" useLiveProfile={false} class="shrink-0" />
    <span class="min-w-0 truncate text-sm text-text">{user.displayName}</span>
    <span class="min-w-0 truncate text-sm text-muted">@{user.login}</span>
  {/snippet}
</Combobox>
