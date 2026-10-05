<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { Code, ConnectError } from '@connectrpc/connect';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import {
    openAuthorizationWindow,
    authorizationWindowFeatures,
    type AuthorizationWindow
  } from '$lib/oauth/authorizationWindow';
  import IdentityLinkContinuation from './IdentityLinkContinuation.svelte';
  import type { CurrentUserState } from '@chatto/client/auth/currentUser';
  import {
    createExternalIdentityAPI,
    type ExternalIdentityProviderInfo,
    type LinkedExternalIdentityInfo
  } from '$lib/api/externalIdentities';
  import { Panel, LoadingFog, ConfirmDialog, Dialog, FormDialog, Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { createQuery } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { serverRegistry } from '$lib/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { Button, TextInput } from '$lib/ui/form';

  let {
    currentUser,
    accountSettingsPath
  }: {
    currentUser: CurrentUserState;
    accountSettingsPath: string;
  } = $props();

  const serverScope = useServerScope();
  // Private identity data and provider links belong to the accepted account only.
  const accountId = $derived(serverScope.store.accountId);
  const session = createSessionGuard(serverScope, 'server-session');

  type LinkVariables = SessionSnapshot & {
    provider: ExternalIdentityProviderInfo;
    currentPassword?: string;
    redirectPath: string;
  };
  type DisconnectVariables = SessionSnapshot & {
    subjectHash: string;
    providerLabel: string;
    currentPassword?: string;
  };

  const identitiesQuery = createQuery(() => {
    const activeServerId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    const activeUserId = accountId ?? '';
    return {
      queryKey: settingsQueryKeys.externalIdentities(
        activeServerId,
        activeConnection,
        activeUserId
      ),
      queryFn: ({ signal }) => activeConnection.getAPI(createExternalIdentityAPI).list({ signal }),
      enabled: activeUserId !== '',
      // A provider callback returns to this route and must not reuse the pre-link snapshot.
      refetchOnMount: 'always' as const
    };
  });

  const providers = $derived(accountId ? (identitiesQuery.data?.providers ?? []) : []);
  const linkedIdentities = $derived(
    accountId ? (identitiesQuery.data?.linkedIdentities ?? []) : []
  );
  const loading = $derived(identitiesQuery.isPending && !identitiesQuery.data);
  let actionError = $state('');
  // Keep refreshes bound to the account and session that opened this window.
  let providerLinkWindow = $state.raw<{
    window: AuthorizationWindow;
    scope: SessionSnapshot;
    userId: string;
    providerId: string;
  } | null>(null);
  let checkingWindow = false;
  let checkingLink = false;

  function providerLinkIsCurrent() {
    return (
      providerLinkWindow !== null &&
      session.isCurrent(providerLinkWindow.scope) &&
      providerLinkWindow.userId === accountId
    );
  }

  function refreshAfterProviderLink() {
    if (providerLinkIsCurrent()) void identitiesQuery.refetch();
  }

  // The provider window has no opener and can be on another origin. Confirm
  // completion through the account API, without waiting for a focus event.
  async function checkProviderLinkResult() {
    const pending = providerLinkWindow;
    if (!pending || checkingLink || !providerLinkIsCurrent()) return;
    checkingLink = true;
    try {
      const result = await identitiesQuery.refetch({ cancelRefetch: false });
      if (providerLinkWindow !== pending || !providerLinkIsCurrent()) return;
      if (
        result.isSuccess &&
        result.data.providers.some(
          (provider) => provider.id === pending.providerId && provider.linked
        )
      ) {
        providerLinkWindow = null;
        // The remote window closes itself. A detached cross-origin window
        // cannot be closed by its former opener in Chromium.
      }
    } finally {
      checkingLink = false;
    }
  }

  async function checkProviderLinkWindow() {
    const pending = providerLinkWindow;
    if (!pending || checkingWindow) return;
    if (!providerLinkIsCurrent()) {
      providerLinkWindow = null;
      return;
    }
    checkingWindow = true;
    try {
      if ((await pending.window.isClosed()) && providerLinkWindow === pending) {
        refreshAfterProviderLink();
        providerLinkWindow = null;
      }
    } finally {
      checkingWindow = false;
    }
  }

  function openProviderLink(provider: ExternalIdentityProviderInfo) {
    actionError = '';
    const userId = accountId;
    if (!userId) return;
    const url = new URL('/chat/-/settings/account', serverScope.connection.connectBaseUrl);
    url.searchParams.set('link_provider', provider.id);
    url.searchParams.set('link_user', userId);
    const authorizationWindow = openAuthorizationWindow(
      '_blank',
      authorizationWindowFeatures(window)
    );
    if (!authorizationWindow) {
      actionError = m('settings.account.sso.popup_blocked');
      return;
    }
    authorizationWindow.detachOpener();
    const pending = {
      window: authorizationWindow,
      scope: session.snapshot(),
      userId,
      providerId: provider.id
    };
    providerLinkWindow = pending;
    void authorizationWindow.navigate(url.href).catch(() => {
      if (providerLinkWindow !== pending) return;
      if (providerLinkIsCurrent()) actionError = m('settings.account.sso.link_failed');
      providerLinkWindow = null;
      void authorizationWindow.close();
    });
  }

  function continueProviderLink(providerId: string, userId: string) {
    if (!userId || userId !== accountId) {
      actionError = m('settings.account.sso.account_mismatch');
      return;
    }
    const provider = providers.find((provider) => provider.id === providerId);
    if (!provider) {
      actionError = m('settings.account.sso.provider_unavailable');
      return;
    }
    if (provider.linked) window.close();
    else void startProviderLink(provider);
  }

  function completeProviderLink(providerId: string, userId: string) {
    if (!userId || userId !== accountId) {
      actionError = m('settings.account.sso.account_mismatch');
      return;
    }
    if (providers.some((provider) => provider.id === providerId && provider.linked)) {
      window.close();
    }
  }

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

  // Track requests in flight here, not with mutation state: an observer reports
  // the settled state a tick after the request promise, so a dialog opened in
  // the rejection handler would first render disabled.
  let linkingProviderId = $state('');
  let disconnectingSubjectHash = $state('');
  const error = $derived.by(() => {
    if (actionError) return actionError;
    const queryError = identitiesQuery.error;
    return queryError ? errorMessage(queryError, m('settings.account.sso.load_failed')) : '';
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
        return 'icon-[mdi--github]';
      case 'gitlab':
        return 'icon-[mdi--gitlab]';
      case 'google':
        return 'icon-[mdi--google]';
      case 'discord':
        return 'icon-[mdi--discord]';
      default:
        return 'icon-[mdi--shield-account]';
    }
  }

  async function startProviderLink(
    provider: ExternalIdentityProviderInfo,
    currentPassword?: string
  ) {
    const returnURL = new URL(accountSettingsPath, window.location.origin);
    returnURL.searchParams.set('link_provider', provider.id);
    returnURL.searchParams.set('link_user', accountId ?? '');
    returnURL.searchParams.set('link_complete', '1');
    const variables: LinkVariables = {
      ...session.snapshot(),
      provider,
      currentPassword,
      redirectPath: returnURL.pathname + returnURL.search + returnURL.hash
    };
    actionError = '';
    linkingProviderId = provider.id;
    try {
      const startUrl = await variables.connection.getAPI(createExternalIdentityAPI).startLink({
        providerId: provider.id,
        redirectPath: variables.redirectPath,
        currentPassword
      });
      if (!session.isCurrent(variables)) return;
      window.location.href = startUrl;
    } catch (err) {
      if (!session.isCurrent(variables)) return;
      if (
        err instanceof ConnectError &&
        err.code === Code.FailedPrecondition &&
        hasPassword &&
        currentPassword === undefined
      ) {
        linkFreshAuthProvider = provider;
        linkCurrentPassword = '';
        linkFreshAuthError = '';
      } else if (
        err instanceof ConnectError &&
        err.code === Code.FailedPrecondition &&
        currentPassword === undefined
      ) {
        actionError = m('settings.account.sso.fresh_auth_required');
      } else if (currentPassword !== undefined) {
        linkFreshAuthError = errorMessage(err, m('settings.account.sso.link_failed'));
      } else {
        actionError = errorMessage(err, m('settings.account.sso.link_failed'));
      }
    } finally {
      linkingProviderId = '';
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
      linkFreshAuthError = m('settings.account.password.current_required');
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

  async function disconnectIdentity(
    target: { subjectHash: string; providerLabel: string },
    currentPassword?: string
  ) {
    const { subjectHash, providerLabel } = target;
    const variables: DisconnectVariables = {
      ...session.snapshot(),
      subjectHash,
      providerLabel,
      currentPassword
    };
    actionError = '';
    disconnectingSubjectHash = subjectHash;
    try {
      await variables.connection
        .getAPI(createExternalIdentityAPI)
        .disconnect(subjectHash, currentPassword);
      if (!session.isCurrent(variables)) {
        return;
      }
      disconnectTarget = null;
      disconnectFreshAuthTarget = null;
      disconnectCurrentPassword = '';
      disconnectFreshAuthError = '';
      await identitiesQuery.refetch();
    } catch (err) {
      if (!session.isCurrent(variables)) {
        return;
      }
      if (
        err instanceof ConnectError &&
        err.code === Code.FailedPrecondition &&
        currentPassword === undefined
      ) {
        disconnectTarget = null;
        if (hasPassword) {
          disconnectFreshAuthTarget = { subjectHash, providerLabel };
          disconnectCurrentPassword = '';
          disconnectFreshAuthError = '';
        } else {
          actionError = m('settings.account.sso.disconnect_fresh_auth_required');
        }
      } else if (currentPassword !== undefined) {
        disconnectFreshAuthError = errorMessage(err, m('settings.account.sso.disconnect_failed'));
      } else {
        actionError = errorMessage(err, m('settings.account.sso.disconnect_failed'));
        disconnectTarget = null;
      }
    } finally {
      disconnectingSubjectHash = '';
    }
  }

  async function confirmDisconnectFreshAuth(e: Event) {
    e.preventDefault();
    if (!disconnectFreshAuthTarget || !disconnectCurrentPassword) {
      disconnectFreshAuthError = m('settings.account.password.current_required');
      return;
    }
    disconnectFreshAuthError = '';
    await disconnectIdentity(disconnectFreshAuthTarget, disconnectCurrentPassword);
  }

  function disconnectButtonLabel(subjectHash: string) {
    return disconnectingSubjectHash === subjectHash
      ? m('settings.account.sso.disconnecting')
      : m('settings.account.sso.disconnect_button');
  }
</script>

<svelte:window onfocus={refreshAfterProviderLink} />

{#if providerLinkWindow}
  <Interval milliseconds={500} ontick={checkProviderLinkWindow} />
  <Interval milliseconds={2000} ontick={checkProviderLinkResult} />
{/if}

{#if serverRegistry.isOriginServer(serverScope.serverId) && accountId && identitiesQuery.isSuccess && !identitiesQuery.isFetching}
  <IdentityLinkContinuation oncontinue={continueProviderLink} oncomplete={completeProviderLink} />
{/if}

<Panel title={m('settings.account.sso.title')} icon="iconify icon-[uil--link]">
  <div class="flex max-w-md flex-col gap-4">
    {#if loading}
      <LoadingFog class="h-32 w-full" label={m('settings.account.sso.loading')} />
    {:else}
      {#if error}
        <Hint tone="danger">{error}</Hint>
      {/if}
      {#if !hasRows}
        <p class="text-sm text-muted">{m('settings.account.sso.none_configured')}</p>
      {:else}
        <div class="flex flex-col gap-3">
          {#each providers as provider (provider.id)}
            <div class="flex items-center justify-between gap-3 rounded border border-border p-3">
              <div class="flex min-w-0 items-center gap-3">
                <span
                  aria-hidden="true"
                  class={['iconify text-lg text-muted', providerIcon(provider.type)]}
                ></span>
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium">{provider.label}</div>
                  <div class="text-xs text-muted">
                    {#if provider.linked}
                      {m('settings.account.sso.linked')}
                    {:else}
                      {m('settings.account.sso.not_linked')}
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
                    <span aria-hidden="true" class="iconify icon-[uil--link-broken]"></span>
                    {disconnectButtonLabel(provider.linkedIdentitySubjectHash)}
                  </Button>
                {:else}
                  <span class="text-sm text-muted">{m('settings.account.sso.linked')}</span>
                {/if}
              {:else}
                <Button
                  variant="secondary"
                  size="sm"
                  loading={linkingProviderId === provider.id}
                  disabled={linkingProviderId !== '' ||
                    disconnectingSubjectHash !== '' ||
                    providerLinkWindow !== null}
                  onclick={() => openProviderLink(provider)}
                >
                  <span aria-hidden="true" class="iconify icon-[uil--link]"></span>
                  {m('settings.account.sso.link_button')}
                </Button>
              {/if}
            </div>
          {/each}

          {#each unconfiguredLinkedIdentities as identity (identity.subjectHash)}
            <div class="flex items-center justify-between gap-3 rounded border border-border p-3">
              <div class="flex min-w-0 items-center gap-3">
                <span
                  aria-hidden="true"
                  class={['iconify text-lg text-muted', providerIcon(identity.providerType)]}
                ></span>
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium">{identity.providerLabel}</div>
                  <div class="text-xs text-muted">
                    {m('settings.account.sso.provider_unconfigured')}
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
                <span aria-hidden="true" class="iconify icon-[uil--link-broken]"></span>
                {disconnectButtonLabel(identity.subjectHash)}
              </Button>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
</Panel>

{#if disconnectTarget}
  <ConfirmDialog
    visible
    title={m('settings.account.sso.disconnect_modal.title')}
    actionLabel={m('settings.account.sso.disconnect_modal.action')}
    actionIcon="iconify icon-[uil--link-broken]"
    loading={disconnectingSubjectHash === disconnectTarget.subjectHash}
    onconfirm={confirmDisconnectIdentity}
    onclose={closeDisconnectDialog}
  >
    {m('settings.account.sso.disconnect_modal.body', {
      provider: disconnectTarget.providerLabel
    })}
  </ConfirmDialog>
{/if}

{#if disconnectFreshAuthTarget}
  {@const freshAuthTarget = disconnectFreshAuthTarget}
  <FormDialog
    visible
    title={m('settings.account.sso.disconnect_fresh_auth_modal.title')}
    size="sm"
    submitLabel={m('settings.account.sso.disconnect_fresh_auth_modal.action')}
    submitIcon="iconify icon-[uil--link-broken]"
    loading={disconnectingSubjectHash === freshAuthTarget.subjectHash}
    disabled={!disconnectCurrentPassword || disconnectingSubjectHash !== ''}
    error={disconnectFreshAuthError}
    onsubmit={confirmDisconnectFreshAuth}
    onclose={closeDisconnectFreshAuthDialog}
  >
    {#snippet description()}
      <p>
        {m('settings.account.sso.disconnect_fresh_auth_modal.body', {
          provider: freshAuthTarget.providerLabel
        })}
      </p>
    {/snippet}

    <TextInput
      id="sso-disconnect-current-password"
      label={m('settings.account.password.current_label')}
      type="password"
      bind:value={disconnectCurrentPassword}
      disabled={disconnectingSubjectHash !== ''}
      autocomplete="current-password"
    />
  </FormDialog>
{/if}

<Dialog
  visible={showDisconnectBlockedModal}
  title={m('settings.account.sso.disconnect_blocked_modal.title')}
  size="sm"
  onclose={closeDisconnectBlockedModal}
>
  <Hint tone="warning">
    {m('settings.account.sso.disconnect_blocked_modal.body', {
      provider: blockedDisconnectProviderLabel
    })}
  </Hint>

  {#snippet dismissAction()}
    <Button defaultAction variant="secondary" onclick={closeDisconnectBlockedModal}>
      {m('ui.close')}
    </Button>
  {/snippet}
</Dialog>

{#if linkFreshAuthProvider}
  {@const freshAuthProvider = linkFreshAuthProvider}
  <FormDialog
    visible
    title={m('settings.account.sso.fresh_auth_modal.title')}
    size="sm"
    submitLabel={m('settings.account.sso.fresh_auth_modal.action')}
    submitIcon="iconify icon-[uil--link]"
    loading={linkingProviderId === freshAuthProvider.id}
    disabled={!linkCurrentPassword || linkingProviderId !== ''}
    error={linkFreshAuthError}
    onsubmit={confirmLinkFreshAuth}
    onclose={closeLinkFreshAuthDialog}
  >
    {#snippet description()}
      <p>
        {m('settings.account.sso.fresh_auth_modal.body', {
          provider: freshAuthProvider.label
        })}
      </p>
    {/snippet}

    <TextInput
      id="sso-link-current-password"
      label={m('settings.account.password.current_label')}
      type="password"
      bind:value={linkCurrentPassword}
      disabled={linkingProviderId !== ''}
      autocomplete="current-password"
    />
  </FormDialog>
{/if}
