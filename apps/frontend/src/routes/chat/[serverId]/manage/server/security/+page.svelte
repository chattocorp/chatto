<script lang="ts">
  import {
    createInfiniteQuery,
    createMutation,
    createQuery,
    type InfiniteData
  } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import {
    createOAuthClientAPI,
    type OAuthClient,
    type EditableOAuthClientPolicyName
  } from '$lib/api-client/oauthClients';
  import { getServerSecurityConfig, updateBlockedUsernames } from '$lib/api-client/serverState';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import DataTable from '$lib/ui/DataTable.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Hint, PaneContent } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

  const PAGE_SIZE = 20;

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  let scrollContainer = $state<HTMLDivElement>();
  let privacyGeneration = 0;
  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    privacyGeneration += 1;
    removeCacheRemovalListener();
  });

  type SecurityMutationVariables = {
    serverId: string;
    connection: ServerConnection;
    queryKey: ReturnType<typeof adminQueryKeys.securityConfig>;
    blockedUsernames: string;
    privacyGeneration: number;
  };

  type OAuthClientPolicyMutationVariables = {
    serverId: string;
    connection: ServerConnection;
    clientId: string;
    policy: EditableOAuthClientPolicyName;
    privacyGeneration: number;
  };

  type OAuthClientPage = {
    oauthClients: OAuthClient[];
    totalCount: number;
    hasMore: boolean;
  };

  const pendingOAuthClientPolicies = new SvelteSet<string>();

  function oauthClientPolicyMutationKey(variables: OAuthClientPolicyMutationVariables): string {
    return JSON.stringify([
      variables.serverId,
      variables.connection.queryScope,
      variables.privacyGeneration,
      variables.clientId
    ]);
  }

  const securityQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: adminQueryKeys.securityConfig(serverId, connection),
        queryFn: ({ signal }) => getServerSecurityConfig(connection.apiConfig, { signal })
      };
    },
    () => queryClient
  );

  const oauthClientsQuery = createInfiniteQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: adminQueryKeys.oauthClients(serverId, connection),
        queryFn: ({ pageParam, signal }) =>
          connection.getAPI(createOAuthClientAPI).list(pageParam, PAGE_SIZE, { signal }),
        initialPageParam: 0,
        getNextPageParam: (lastPage, _pages, lastPageParam) =>
          lastPage.hasMore && lastPage.oauthClients.length > 0
            ? lastPageParam + lastPage.oauthClients.length
            : undefined
      };
    },
    () => queryClient
  );

  function isCurrentSession(
    variables: SecurityMutationVariables | undefined
  ): variables is SecurityMutationVariables {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  function isCurrentOAuthClientSession(
    variables: OAuthClientPolicyMutationVariables | undefined
  ): variables is OAuthClientPolicyMutationVariables {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  const securityMutation = createMutation(
    () => ({
      mutationFn: ({ connection, blockedUsernames }: SecurityMutationVariables) =>
        updateBlockedUsernames(connection.apiConfig, blockedUsernames),
      onSuccess: (config, variables) => {
        if (!isCurrentSession(variables)) return;
        queryClient.setQueryData(variables.queryKey, config);
        toast.success(m('admin.security.settings_saved'));
      },
      onError: (mutationError, variables) => {
        if (!isCurrentSession(variables)) return;
        toast.error(mutationError instanceof Error ? mutationError.message : String(mutationError));
      }
    }),
    () => queryClient
  );

  const oauthClientPolicyMutation = createMutation(
    () => ({
      mutationFn: ({ connection, clientId, policy }: OAuthClientPolicyMutationVariables) =>
        connection.getAPI(createOAuthClientAPI).updatePolicy(clientId, policy),
      onSuccess: (client, variables) => {
        if (!isCurrentOAuthClientSession(variables)) return;
        const queryKey = adminQueryKeys.oauthClients(variables.serverId, variables.connection);
        queryClient.setQueryData<InfiniteData<OAuthClientPage, number>>(queryKey, (current) =>
          current
            ? {
                ...current,
                pages: current.pages.map((page) => ({
                  ...page,
                  oauthClients: page.oauthClients.map((cached) =>
                    cached.clientId === client.clientId ? client : cached
                  )
                }))
              }
            : current
        );
        void queryClient.invalidateQueries({
          queryKey
        });
        toast.success(m('admin.security.oauth_clients.policy_saved'));
      },
      onError: (mutationError, variables) => {
        if (!isCurrentOAuthClientSession(variables)) return;
        toast.error(mutationError instanceof Error ? mutationError.message : String(mutationError));
      },
      onSettled: (_client, _mutationError, variables) => {
        pendingOAuthClientPolicies.delete(oauthClientPolicyMutationKey(variables));
      }
    }),
    () => queryClient
  );

  const securityConfig = $derived(securityQuery.data ?? null);
  let blockedUsernames = $derived(securityConfig?.blockedUsernames ?? '');
  const loading = $derived(securityQuery.isPending);
  const saving = $derived(
    securityMutation.isPending && isCurrentSession(securityMutation.variables)
  );
  const changed = $derived(
    securityConfig !== null && blockedUsernames !== securityConfig.blockedUsernames
  );
  const error = $derived.by(() => {
    const queryError = securityQuery.error;
    if (queryError) return queryError instanceof Error ? queryError.message : String(queryError);
    if (securityMutation.isError && isCurrentSession(securityMutation.variables)) {
      return securityMutation.error instanceof Error
        ? securityMutation.error.message
        : String(securityMutation.error);
    }
    return null;
  });
  const oauthClients = $derived(
    (oauthClientsQuery.data?.pages ?? []).flatMap((page) => page.oauthClients)
  );
  const oauthClientsLoading = $derived(oauthClientsQuery.isPending);
  const oauthClientsLoadingMore = $derived(oauthClientsQuery.isFetchingNextPage);
  const oauthClientsHasMore = $derived(oauthClientsQuery.hasNextPage);

  function save(e: Event) {
    e.preventDefault();
    if (!changed || saving) return;
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    securityMutation.mutate({
      serverId,
      connection,
      queryKey: adminQueryKeys.securityConfig(serverId, connection),
      blockedUsernames,
      privacyGeneration
    });
  }

  function updateOAuthClientPolicy(client: OAuthClient, event: Event) {
    const select = event.currentTarget as HTMLSelectElement;
    const policy = select.value;
    if (client.policy === 'unknown') {
      select.value = client.policy;
      return;
    }
    if (
      policy === client.policy ||
      !isEditableOAuthClientPolicy(policy)
    ) {
      return;
    }

    // Keep displaying the last server-confirmed security policy until the
    // mutation succeeds and the authoritative list has been refreshed.
    select.value = client.policy;
    const variables = {
      serverId: serverScope.serverId,
      connection: serverScope.connection,
      clientId: client.clientId,
      policy,
      privacyGeneration
    };
    const mutationKey = oauthClientPolicyMutationKey(variables);
    if (pendingOAuthClientPolicies.has(mutationKey)) return;

    pendingOAuthClientPolicies.add(mutationKey);
    oauthClientPolicyMutation.mutate(variables);
  }

  function isEditableOAuthClientPolicy(value: string): value is EditableOAuthClientPolicyName {
    return value === 'default' || value === 'trusted' || value === 'blocked';
  }

  function policySaving(client: OAuthClient): boolean {
    if (client.policy === 'unknown') return false;
    return pendingOAuthClientPolicies.has(
      oauthClientPolicyMutationKey({
        serverId: serverScope.serverId,
        connection: serverScope.connection,
        clientId: client.clientId,
        policy: client.policy,
        privacyGeneration
      })
    );
  }

  function formatTimestamp(value: string): string {
    return formatDateTime(value, userSettings, activeLocale);
  }

  async function loadMoreOAuthClients() {
    if (!oauthClientsLoading && !oauthClientsLoadingMore && oauthClientsHasMore) {
      await oauthClientsQuery.fetchNextPage();
    }
  }
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.security.title') })}
/>

<PaneHeader
  title={m('admin.security.title')}
  subtitle={m('admin.security.subtitle')}
  showMobileNav
/>

<PaneContent bind:scrollContainer>
  <div class="flex flex-col gap-6">
    <Panel
      title={m('admin.security.blocked_usernames')}
      icon="iconify icon-[uil--shield-exclamation]"
    >
      {#if loading}
        <div class="text-muted">{m('admin.common.loading')}</div>
      {:else}
        <form onsubmit={save} class="flex flex-col gap-4">
          {#if error}
            <Hint tone="danger">{error}</Hint>
          {/if}

          <TextArea
            label={m('admin.security.blocked_usernames')}
            id="blocked-usernames"
            bind:value={blockedUsernames}
            rows={6}
            disabled={saving}
            description={m('admin.security.blocked_usernames_description')}
          />

          <div class="flex items-center gap-3">
            <Button type="submit" disabled={!changed || saving} loading={saving}>
              <span class="iconify icon-[uil--check]"></span>
              {m('rbac.role_form.save')}
            </Button>
          </div>
        </form>
      {/if}
    </Panel>

    <Panel
      title={m('admin.security.oauth_clients.title')}
      icon="iconify icon-[uil--apps]"
      noPadding
    >
      <div class="border-b border-border px-5 py-4 text-sm text-muted">
        {m('admin.security.oauth_clients.description')}
      </div>
      {#if oauthClientsQuery.error}
        <div class="p-5"><Hint tone="danger">{String(oauthClientsQuery.error)}</Hint></div>
      {/if}
      {#if oauthClientsQuery.data !== undefined}
        <DataTable
          items={oauthClients}
          columns={5}
          getKey={(client) => client.clientId}
          emptyMessage={m('admin.security.oauth_clients.empty')}
          hasMore={oauthClientsHasMore}
          loadingMore={oauthClientsLoadingMore}
          onLoadMore={loadMoreOAuthClients}
          loadMoreRoot={scrollContainer}
          loadingMoreMessage={m('admin.common.loading')}
        >
        {#snippet header()}
          <th class="table-header-cell">{m('admin.security.oauth_clients.application')}</th>
          <th class="table-header-cell">{m('admin.security.oauth_clients.origins')}</th>
          <th class="table-header-cell">{m('admin.security.oauth_clients.users')}</th>
          <th class="table-header-cell">{m('admin.security.oauth_clients.last_authorization')}</th>
          <th class="table-header-cell">{m('admin.security.oauth_clients.policy')}</th>
        {/snippet}
        {#snippet row(client)}
          <td class="max-w-72 px-4 py-3 align-top">
            <div class="font-medium">{client.clientName || m('admin.common.unknown')}</div>
            <div class="mt-1 truncate font-mono text-xs text-muted" title={client.clientId}>
              <bdi dir="ltr">{client.clientId}</bdi>
            </div>
          </td>
          <td class="max-w-64 px-4 py-3 align-top text-sm text-muted">
            {#each client.redirectOrigins as origin, index (origin)}
              {#if index > 0}, {/if}<bdi dir="ltr">{origin}</bdi>
            {/each}
          </td>
          <td class="px-4 py-3 align-top">{client.authorizedUserCount}</td>
          <td class="px-4 py-3 align-top whitespace-nowrap text-sm text-muted">
            {formatTimestamp(client.lastAuthorizationAt)}
          </td>
          <td class="min-w-44 px-4 py-3 align-top">
            <select
              class="input"
              name="oauth-client-policy"
              value={client.policy}
              aria-label={m('admin.security.oauth_clients.policy_for', {
                client: client.clientName || client.clientId
              })}
              disabled={client.policy === 'unknown' || policySaving(client)}
              onchange={(event) => updateOAuthClientPolicy(client, event)}
            >
              {#if client.policy === 'unknown'}
                <option value="unknown">
                  {m('admin.common.unknown')} ({client.policyCode})
                </option>
              {/if}
              <option value="default">{m('admin.security.oauth_clients.policy_default')}</option>
              <option value="trusted">{m('admin.security.oauth_clients.policy_trusted')}</option>
              <option value="blocked">{m('admin.security.oauth_clients.policy_blocked')}</option>
            </select>
          </td>
        {/snippet}
        </DataTable>
      {:else if oauthClientsLoading}
        <div class="p-5 text-muted">{m('admin.common.loading')}</div>
      {/if}
    </Panel>
  </div>
</PaneContent>
