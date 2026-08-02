<script lang="ts">
  import { Code, ConnectError } from '@connectrpc/connect';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import {
    beginExplicitSignOutRedirect,
    cancelExplicitSignOutRedirect,
    hardRedirectAfterSignOut
  } from '$lib/auth/signOut';
  import { clearCachedUser } from '$lib/auth/loadAuth';
  import { notifyLogout } from '$lib/auth/sessionChannel';
  import type { CurrentUserState } from '$lib/auth/currentUser.svelte';
  import {
    createExternalIdentityAPI,
    type ExternalIdentityProviderInfo,
    type LinkedExternalIdentityInfo
  } from '$lib/api-client/externalIdentities';
  import * as m from '$lib/i18n/messages';
  import { registerServerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { ConfirmDialog, Dialog, FormSection, Hint } from '$lib/ui';
  import { Button, FormError, TextInput } from '$lib/ui/form';

  let {
    currentUser,
    accountSettingsPath
  }: {
    currentUser: CurrentUserState;
    accountSettingsPath: string;
  } = $props();

  const serverScope = useServerScope();
  let componentActive = true;
  let privacyGeneration = 0;
  const removeCacheRemovalListener = registerServerQueryCacheRemovalListener((removedServerId) => {
    if (removedServerId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    componentActive = false;
    privacyGeneration += 1;
    removeCacheRemovalListener();
  });

  type IdentityMutationScope = {
    serverId: string;
    connection: ServerConnection;
    privacyGeneration: number;
  };
  type LinkVariables = IdentityMutationScope & {
    provider: ExternalIdentityProviderInfo;
    currentPassword?: string;
  };
  type DisconnectVariables = IdentityMutationScope & {
    subjectHash: string;
    providerLabel: string;
    currentPassword?: string;
  };

  const identitiesQuery = createQuery(
    () => {
      const activeServerId = serverScope.serverId;
      const activeConnection = serverScope.connection;
      return {
        queryKey: settingsQueryKeys.externalIdentities(activeServerId, activeConnection),
        queryFn: ({ signal }) =>
          activeConnection.getAPI(createExternalIdentityAPI).list({ signal }),
        // A provider callback returns to this route and must not reuse the pre-link snapshot.
        refetchOnMount: 'always' as const
      };
    },
    () => queryClient
  );

  const providers = $derived(identitiesQuery.data?.providers ?? []);
  const linkedIdentities = $derived(identitiesQuery.data?.linkedIdentities ?? []);
  const loading = $derived(identitiesQuery.isPending && !identitiesQuery.data);
  let actionError = $state('');
  let linkFreshAuthProvider = $state<ExternalIdentityProviderInfo | null>(null);
  let linkCurrentPassword = $state('');
  let linkFreshAuthError = $state('');
  let disconnectTarget = $state<{ subjectHash: string; providerLabel: string } | null>(null);
  let disconnectFreshAuthTarget = $state<{
    subjectHash: string;
    providerLabel: string;
  } | null>(null);
  let disconnectCurrentPassword = $state('');
  let disconnectFreshAuthError = $state('');
  let blockedDisconnectProviderLabel = $state('');
  let showDisconnectBlockedModal = $state(false);

  function mutationScope(): IdentityMutationScope {
    return {
      serverId: serverScope.serverId,
      connection: serverScope.connection,
      privacyGeneration
    };
  }

  function isCurrentConnection(
    variables: IdentityMutationScope | undefined
  ): variables is IdentityMutationScope {
    return (
      variables !== undefined &&
      componentActive &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope
    );
  }

  function isCurrentSession(
    variables: IdentityMutationScope | undefined
  ): variables is IdentityMutationScope {
    return isCurrentConnection(variables) && variables.privacyGeneration === privacyGeneration;
  }

  const linkMutation = createMutation(
    () => ({
      mutationFn: ({ connection: activeConnection, provider, currentPassword }: LinkVariables) =>
        activeConnection.getAPI(createExternalIdentityAPI).startLink({
          providerId: provider.id,
          redirectPath: accountSettingsPath,
          currentPassword
        })
    }),
    () => queryClient
  );

  const disconnectMutation = createMutation(
    () => ({
      mutationFn: ({
        connection: activeConnection,
        subjectHash,
        currentPassword
      }: DisconnectVariables) =>
        activeConnection.getAPI(createExternalIdentityAPI).disconnect(subjectHash, currentPassword)
    }),
    () => queryClient
  );

  const linkingProviderId = $derived(
    linkMutation.isPending && isCurrentSession(linkMutation.variables)
      ? linkMutation.variables.provider.id
      : ''
  );
  const disconnectingSubjectHash = $derived(
    disconnectMutation.isPending && isCurrentSession(disconnectMutation.variables)
      ? disconnectMutation.variables.subjectHash
      : ''
  );
  const error = $derived.by(() => {
    if (actionError) return actionError;
    const queryError = identitiesQuery.error;
    return queryError
      ? queryError instanceof Error
        ? queryError.message
        : m['settings.account.sso.load_failed']()
      : '';
  });

  const hasPassword = $derived(currentUser.user?.hasPassword ?? false);
  const unconfiguredLinkedIdentities = $derived(
    linkedIdentities.filter(
      (identity) =>
        !providers.some((provider) => provider.linkedIdentitySubjectHash === identity.subjectHash)
    )
  );
  const hasRows = $derived(providers.length > 0 || unconfiguredLinkedIdentities.length > 0);
  const disconnectWouldRemoveLastMethod = $derived(!hasPassword && linkedIdentities.length <= 1);

  function providerIcon(type: string): string {
    switch (type) {
      case 'github':
        return 'mdi--github';
      case 'gitlab':
        return 'mdi--gitlab';
      case 'google':
        return 'mdi--google';
      case 'discord':
        return 'mdi--discord';
      default:
        return 'mdi--shield-account';
    }
  }

  async function startProviderLink(
    provider: ExternalIdentityProviderInfo,
    currentPassword?: string
  ) {
    const variables: LinkVariables = { ...mutationScope(), provider, currentPassword };
    actionError = '';
    try {
      const startUrl = await linkMutation.mutateAsync(variables);
      if (!isCurrentSession(variables)) return;
      window.location.href = startUrl;
    } catch (err) {
      if (!isCurrentSession(variables)) return;
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition && hasPassword) {
        linkFreshAuthProvider = provider;
        linkCurrentPassword = '';
        linkFreshAuthError = '';
      } else if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        actionError = m['settings.account.sso.fresh_auth_required']();
      } else if (currentPassword !== undefined) {
        linkFreshAuthError =
          err instanceof Error ? err.message : m['settings.account.sso.link_failed']();
      } else {
        actionError = err instanceof Error ? err.message : m['settings.account.sso.link_failed']();
      }
    }
  }

  function closeLinkFreshAuthDialog() {
    if (linkingProviderId) return;
    linkFreshAuthProvider = null;
    linkCurrentPassword = '';
    linkFreshAuthError = '';
  }

  async function confirmLinkFreshAuth(e: Event) {
    e.preventDefault();
    if (!linkFreshAuthProvider || !linkCurrentPassword) {
      linkFreshAuthError = m['settings.account.password.current_required']();
      return;
    }
    const provider = linkFreshAuthProvider;
    linkFreshAuthError = '';
    await startProviderLink(provider, linkCurrentPassword);
  }

  function openDisconnectProvider(provider: ExternalIdentityProviderInfo) {
    if (!provider.linkedIdentitySubjectHash) return;
    openDisconnectDialog(provider.linkedIdentitySubjectHash, provider.label);
  }

  function openDisconnectIdentity(identity: LinkedExternalIdentityInfo) {
    openDisconnectDialog(identity.subjectHash, identity.providerLabel);
  }

  function openDisconnectDialog(subjectHash: string, providerLabel: string) {
    actionError = '';
    if (disconnectWouldRemoveLastMethod) {
      blockedDisconnectProviderLabel = providerLabel;
      showDisconnectBlockedModal = true;
      return;
    }
    disconnectTarget = { subjectHash, providerLabel };
  }

  function closeDisconnectDialog() {
    if (disconnectingSubjectHash) return;
    disconnectTarget = null;
  }

  function closeDisconnectFreshAuthDialog() {
    if (disconnectingSubjectHash) return;
    disconnectFreshAuthTarget = null;
    disconnectCurrentPassword = '';
    disconnectFreshAuthError = '';
  }

  function closeDisconnectBlockedModal() {
    showDisconnectBlockedModal = false;
    blockedDisconnectProviderLabel = '';
  }

  async function confirmDisconnectIdentity(currentPassword?: string) {
    if (!disconnectTarget) return;
    await disconnectIdentity(disconnectTarget, currentPassword);
  }

  function finishDisconnectedSession(signedOutServerId: string) {
    if (serverRegistry.isOriginServer(signedOutServerId)) {
      clearCachedUser();
    }
    serverRegistry.clearServerAuthentication(signedOutServerId);
    hardRedirectAfterSignOut('/');
    if (serverRegistry.isOriginServer(signedOutServerId)) {
      notifyLogout();
    }
  }

  async function disconnectIdentity(
    target: { subjectHash: string; providerLabel: string },
    currentPassword?: string
  ) {
    const { subjectHash, providerLabel } = target;
    const variables: DisconnectVariables = {
      ...mutationScope(),
      subjectHash,
      providerLabel,
      currentPassword
    };
    actionError = '';
    try {
      beginExplicitSignOutRedirect();
      await disconnectMutation.mutateAsync(variables);
      const signedOutServerId = variables.connection.serverId ?? variables.serverId;
      if (!isCurrentSession(variables)) {
        cancelExplicitSignOutRedirect();
        return;
      }
      disconnectTarget = null;
      disconnectFreshAuthTarget = null;
      disconnectCurrentPassword = '';
      disconnectFreshAuthError = '';
      finishDisconnectedSession(signedOutServerId);
    } catch (err) {
      if (
        err instanceof ConnectError &&
        err.code === Code.Unauthenticated &&
        isCurrentConnection(variables)
      ) {
        finishDisconnectedSession(variables.connection.serverId ?? variables.serverId);
        return;
      }
      if (!isCurrentSession(variables)) {
        cancelExplicitSignOutRedirect();
        return;
      }
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        cancelExplicitSignOutRedirect();
        disconnectTarget = null;
        if (hasPassword) {
          disconnectFreshAuthTarget = { subjectHash, providerLabel };
          disconnectCurrentPassword = '';
          disconnectFreshAuthError = '';
        } else {
          actionError = m['settings.account.sso.disconnect_fresh_auth_required']();
        }
      } else if (currentPassword !== undefined) {
        cancelExplicitSignOutRedirect();
        disconnectFreshAuthError =
          err instanceof Error ? err.message : m['settings.account.sso.disconnect_failed']();
      } else {
        cancelExplicitSignOutRedirect();
        actionError =
          err instanceof Error ? err.message : m['settings.account.sso.disconnect_failed']();
        disconnectTarget = null;
      }
    }
  }

  async function confirmDisconnectFreshAuth(e: Event) {
    e.preventDefault();
    if (!disconnectFreshAuthTarget || !disconnectCurrentPassword) {
      disconnectFreshAuthError = m['settings.account.password.current_required']();
      return;
    }
    disconnectFreshAuthError = '';
    await disconnectIdentity(disconnectFreshAuthTarget, disconnectCurrentPassword);
  }

  function disconnectButtonLabel(subjectHash: string) {
    return disconnectingSubjectHash === subjectHash
      ? m['settings.account.sso.disconnecting']()
      : m['settings.account.sso.disconnect_button']();
  }
</script>

<FormSection title={m['settings.account.sso.title']()} maxWidth="max-w-md">
  <div class="flex flex-col gap-4">
    {#if loading}
      <p class="text-sm text-muted">{m['settings.account.sso.loading']()}</p>
    {:else}
      {#if error}
        <Hint tone="danger">{error}</Hint>
      {/if}
      {#if !hasRows}
        <p class="text-sm text-muted">{m['settings.account.sso.none_configured']()}</p>
      {:else}
        <div class="flex flex-col gap-3">
          {#each providers as provider (provider.id)}
            <div class="flex items-center justify-between gap-3 rounded border border-border p-3">
              <div class="flex min-w-0 items-center gap-3">
                <span class={['iconify text-lg text-muted', providerIcon(provider.type)]}></span>
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium">{provider.label}</div>
                  <div class="text-xs text-muted">
                    {#if provider.linked}
                      {m['settings.account.sso.linked']()}
                    {:else}
                      {m['settings.account.sso.not_linked']()}
                    {/if}
                  </div>
                </div>
              </div>
              {#if provider.linked}
                {#if provider.linkedIdentitySubjectHash}
                  <Button
                    variant="danger-secondary"
                    size="sm"
                    loading={disconnectingSubjectHash === provider.linkedIdentitySubjectHash}
                    disabled={linkingProviderId !== '' || disconnectingSubjectHash !== ''}
                    onclick={() => openDisconnectProvider(provider)}
                  >
                    <span class="iconify uil--link-broken"></span>
                    {disconnectButtonLabel(provider.linkedIdentitySubjectHash)}
                  </Button>
                {:else}
                  <span class="text-sm text-muted">{m['settings.account.sso.linked']()}</span>
                {/if}
              {:else}
                <Button
                  variant="secondary"
                  size="sm"
                  loading={linkingProviderId === provider.id}
                  disabled={linkingProviderId !== '' || disconnectingSubjectHash !== ''}
                  onclick={() => startProviderLink(provider)}
                >
                  <span class="iconify uil--link"></span>
                  {m['settings.account.sso.link_button']()}
                </Button>
              {/if}
            </div>
          {/each}

          {#each unconfiguredLinkedIdentities as identity (identity.subjectHash)}
            <div class="flex items-center justify-between gap-3 rounded border border-border p-3">
              <div class="flex min-w-0 items-center gap-3">
                <span class={['iconify text-lg text-muted', providerIcon(identity.providerType)]}
                ></span>
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium">{identity.providerLabel}</div>
                  <div class="text-xs text-muted">
                    {m['settings.account.sso.provider_unconfigured']()}
                  </div>
                </div>
              </div>
              <Button
                variant="danger-secondary"
                size="sm"
                loading={disconnectingSubjectHash === identity.subjectHash}
                disabled={linkingProviderId !== '' || disconnectingSubjectHash !== ''}
                onclick={() => openDisconnectIdentity(identity)}
              >
                <span class="iconify uil--link-broken"></span>
                {disconnectButtonLabel(identity.subjectHash)}
              </Button>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
</FormSection>

{#if disconnectTarget}
  <ConfirmDialog
    visible
    title={m['settings.account.sso.disconnect_modal.title']()}
    actionLabel={m['settings.account.sso.disconnect_modal.action']()}
    actionIcon="iconify uil--link-broken"
    loading={disconnectingSubjectHash === disconnectTarget.subjectHash}
    onconfirm={confirmDisconnectIdentity}
    onclose={closeDisconnectDialog}
  >
    {m['settings.account.sso.disconnect_modal.body']({
      provider: disconnectTarget.providerLabel
    })}
  </ConfirmDialog>
{/if}

{#if disconnectFreshAuthTarget}
  <Dialog
    visible
    title={m['settings.account.sso.disconnect_fresh_auth_modal.title']()}
    size="sm"
    onclose={closeDisconnectFreshAuthDialog}
  >
    <form class="flex flex-col gap-4" onsubmit={confirmDisconnectFreshAuth}>
      <p class="text-sm text-muted">
        {m['settings.account.sso.disconnect_fresh_auth_modal.body']({
          provider: disconnectFreshAuthTarget.providerLabel
        })}
      </p>
      <TextInput
        id="sso-disconnect-current-password"
        label={m['settings.account.password.current_label']()}
        type="password"
        bind:value={disconnectCurrentPassword}
        disabled={disconnectingSubjectHash !== ''}
        autocomplete="current-password"
      />
      {#if disconnectFreshAuthError}
        <FormError error={disconnectFreshAuthError} />
      {/if}
      <div class="flex flex-wrap justify-end gap-2">
        <Button
          type="button"
          variant="secondary"
          onclick={closeDisconnectFreshAuthDialog}
          disabled={disconnectingSubjectHash !== ''}
        >
          {m['common.cancel']()}
        </Button>
        <Button
          type="submit"
          loading={disconnectingSubjectHash === disconnectFreshAuthTarget.subjectHash}
          disabled={!disconnectCurrentPassword || disconnectingSubjectHash !== ''}
        >
          <span class="iconify uil--link-broken"></span>
          {m['settings.account.sso.disconnect_fresh_auth_modal.action']()}
        </Button>
      </div>
    </form>
  </Dialog>
{/if}

<Dialog
  visible={showDisconnectBlockedModal}
  title={m['settings.account.sso.disconnect_blocked_modal.title']()}
  size="sm"
  onclose={closeDisconnectBlockedModal}
>
  <div class="flex flex-col gap-4">
    <Hint tone="warning">
      {m['settings.account.sso.disconnect_blocked_modal.body']({
        provider: blockedDisconnectProviderLabel
      })}
    </Hint>
    <div class="flex justify-end">
      <Button variant="secondary" onclick={closeDisconnectBlockedModal}>
        {m['ui.close']()}
      </Button>
    </div>
  </div>
</Dialog>

{#if linkFreshAuthProvider}
  <Dialog
    visible
    title={m['settings.account.sso.fresh_auth_modal.title']()}
    size="sm"
    onclose={closeLinkFreshAuthDialog}
  >
    <form class="flex flex-col gap-4" onsubmit={confirmLinkFreshAuth}>
      <p class="text-sm text-muted">
        {m['settings.account.sso.fresh_auth_modal.body']({
          provider: linkFreshAuthProvider.label
        })}
      </p>
      <TextInput
        id="sso-link-current-password"
        label={m['settings.account.password.current_label']()}
        type="password"
        bind:value={linkCurrentPassword}
        disabled={linkingProviderId !== ''}
        autocomplete="current-password"
      />
      {#if linkFreshAuthError}
        <FormError error={linkFreshAuthError} />
      {/if}
      <div class="flex flex-wrap justify-end gap-2">
        <Button
          type="button"
          variant="secondary"
          onclick={closeLinkFreshAuthDialog}
          disabled={linkingProviderId !== ''}
        >
          {m['common.cancel']()}
        </Button>
        <Button
          type="submit"
          loading={linkingProviderId === linkFreshAuthProvider.id}
          disabled={!linkCurrentPassword || linkingProviderId !== ''}
        >
          <span class="iconify uil--link"></span>
          {m['settings.account.sso.fresh_auth_modal.action']()}
        </Button>
      </div>
    </form>
  </Dialog>
{/if}
