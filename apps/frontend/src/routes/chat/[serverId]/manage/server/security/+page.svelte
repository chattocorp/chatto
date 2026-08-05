<script lang="ts">
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import { getServerSecurityConfig, updateBlockedUsernames } from '$lib/api-client/serverState';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { Panel } from '$lib/components/admin';
  import { Hint, PaneContent } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import * as m from '$lib/i18n/messages';

  const serverScope = useServerScope();
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

  const securityMutation = createMutation(
    () => ({
      mutationFn: ({ connection, blockedUsernames }: SecurityMutationVariables) =>
        updateBlockedUsernames(connection.apiConfig, blockedUsernames),
      onSuccess: (config, variables) => {
        if (!isCurrentSession(variables)) return;
        queryClient.setQueryData(variables.queryKey, config);
        toast.success(m['admin.security.settings_saved']());
      },
      onError: (mutationError, variables) => {
        if (!isCurrentSession(variables)) return;
        toast.error(mutationError instanceof Error ? mutationError.message : String(mutationError));
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
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({ title: m['admin.security.title']() })}
/>

<PaneHeader
  title={m['admin.security.title']()}
  subtitle={m['admin.security.subtitle']()}
  showMobileNav
/>

<PaneContent>
  <div class="flex flex-col gap-6">
  <Panel title={m['admin.security.blocked_usernames']()} icon="iconify icon-[uil--shield-exclamation]">
    {#if loading}
      <div class="text-muted">{m['admin.common.loading']()}</div>
    {:else}
      <form onsubmit={save} class="flex flex-col gap-4">
        {#if error}
          <Hint tone="danger">{error}</Hint>
        {/if}

        <TextArea
          label={m['admin.security.blocked_usernames']()}
          id="blocked-usernames"
          bind:value={blockedUsernames}
          rows={6}
          disabled={saving}
          description={m['admin.security.blocked_usernames_description']()}
        />

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={!changed || saving} loading={saving}>
            <span class="iconify icon-[uil--check]"></span>
            {m['rbac.role_form.save']()}
          </Button>
        </div>
      </form>
    {/if}
  </Panel>
  </div>
</PaneContent>
