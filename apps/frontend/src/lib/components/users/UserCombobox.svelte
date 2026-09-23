<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { BOT_ACCOUNT_LABEL, isBotAccount } from '$lib/render/accountName';
  import BotBadge from '$lib/components/users/BotBadge.svelte';
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
    placeholder = m('admin.members.search_placeholder'),
    humanOnly = false,
    allowFreeform = true,
    emptyMessage = m('admin.users.empty'),
    clearLabel = m('common.clear')
  }: {
    id: string;
    label: string;
    value?: string;
    text?: string;
    placeholder?: string;
    humanOnly?: boolean;
    allowFreeform?: boolean;
    emptyMessage?: string;
    clearLabel?: string;
  } = $props();

  const serverScope = useServerScope();

  const SEARCH_LIMIT = 10;
  let activeSearch = $state('');
  let debouncePending = $state(false);
  let selectedUser = $state<User | null>(null);
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
    activeSearch && !debouncePending
      ? (usersQuery.data?.members ?? []).filter((user) => !humanOnly || !user.isBot)
      : []
  );
  const loading = $derived(debouncePending || (!!activeSearch && usersQuery.isFetching));

  function userLabel(user: User): string {
    const handle = user.login ? `@${user.login}` : user.id;
    return [user.displayName, handle].filter(Boolean).join(' ');
  }

  function scheduleSearch(query: string) {
    selectedUser = null;
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
  {allowFreeform}
  {emptyMessage}
  {clearLabel}
  ontextchange={scheduleSearch}
  onselect={(user) => (selectedUser = user)}
  onclear={() => (selectedUser = null)}
  selectionDescription={isBotAccount(selectedUser) ? BOT_ACCOUNT_LABEL : undefined}
>
  {#snippet selectionAdornment()}
    {#if isBotAccount(selectedUser)}<BotBadge />{/if}
  {/snippet}
  {#snippet item({ item: user })}
    <UserAvatar {user} size="xs" useLiveProfile={false} class="shrink-0" />
    <AccountName name={user.displayName} identity={user} class="text-sm text-text" />
    <span class="min-w-0 truncate text-sm text-muted">@{user.login}</span>
  {/snippet}
</Combobox>
