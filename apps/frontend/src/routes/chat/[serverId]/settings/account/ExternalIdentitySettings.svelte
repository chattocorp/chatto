<script lang="ts">
  import { onMount } from 'svelte';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { Code, ConnectError } from '@connectrpc/connect';
  import { page } from '$app/state';
  import {
    reverificationErrorMessage,
    reverificationProviders,
    reverificationStartUrl
  } from '$lib/auth/freshAuthReverification';
  import ReverificationChoice from '$lib/components/auth/ReverificationChoice.svelte';
  import { leavePage } from '$lib/navigation/leavePage';
  import DisconnectContinuation, {
    disconnectContinuationPath
  } from './DisconnectContinuation.svelte';
  import {
    identityLinkPath,
    identityLinkReturnPath,
    linkStartFailure,
    onProviderLinked,
    remoteIdentityLinkUrl
  } from '$lib/auth/identityLink';
  import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';
  import { disconnectEligibilityWithFallback } from '@chatto/client/api/actionAvailability';
  import { identityDisconnectUnavailableMessage } from '$lib/actionAvailability';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import {
    authorizationWindowFeatures,
    openAuthorizationWindow
  } from '$lib/oauth/authorizationWindow';
  import type { CurrentUserState } from '@chatto/client/auth/currentUser';
  import { randomOAuthValue } from '@chatto/client/auth/oauth';
  import {
    createExternalIdentityAPI,
    type ExternalIdentityProviderInfo,
    type LinkedExternalIdentityInfo
  } from '$lib/api/externalIdentities';
  import { Panel, LoadingDots, ConfirmDialog, Dialog, FormDialog, Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { providerIcon } from '$lib/auth/providerIcon';
  import { profileVerifiedIdentitiesKey } from '$lib/query/keys';
  import { createQuery, queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { serverRegistry } from '$lib/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { Button, TextInput } from '$lib/ui/form';

  let { currentUser }: { currentUser: CurrentUserState } = $props();

  const serverScope = useServerScope();
  // Private identity data and provider links belong to the accepted account only.
  const accountId = $derived(serverScope.store.accountId);
  const session = createSessionGuard(serverScope, 'server-session');

  type DisconnectVariables = SessionSnapshot & {
    subjectHash: string;
    providerLabel: string;
    currentPassword?: string;
  };

  const identitiesQuery = createQuery(() => {
    const activeConnection = serverScope.connection;
    const activeUserId = accountId ?? '';
    return {
      queryKey: settingsQueryKeys.externalIdentities(serverScope, activeUserId),
      queryFn: ({ signal }) => activeConnection.getAPI(createExternalIdentityAPI).list({ signal }),
      enabled: activeUserId !== '',
      // The link window can link a provider while this page is not mounted.
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
    isClosed: () => Promise<boolean>;
    scope: SessionSnapshot;
    userId: string;
    providerId: string;
  } | null>(null);
  // A browser can require a new click after the asynchronous link-start request.
  let blockedLaunch = $state.raw<{
    url: string;
    provider: ExternalIdentityProviderInfo;
    scope: SessionSnapshot;
    userId: string;
  } | null>(null);
  let checkingLink = false;

  // Provider isolation can make a live popup report closed. The link window
  // page on this origin can still ask this page to read the final state.
  onMount(() => {
    if (!serverRegistry.isOriginServer(serverScope.serverId)) return;
    return onProviderLinked((userId) => {
      if (!accountId || userId !== accountId) return;
      resetProfileIdentities(accountId);
      void identitiesQuery.refetch();
    });
  });

  function providerLinkIsCurrent() {
    return (
      providerLinkWindow !== null &&
      session.isCurrent(providerLinkWindow.scope) &&
      providerLinkWindow.userId === accountId
    );
  }

  function resetProfileIdentities(userId: string) {
    void queryClient.resetQueries({
      queryKey: profileVerifiedIdentitiesKey(serverScope, userId),
      exact: true
    });
  }

  // A closed window may have completed the link just before the final poll.
  async function refreshAfterProviderLink() {
    const pending = providerLinkWindow;
    if (pending && providerLinkIsCurrent())
      await readProviderLink(pending, { cancelRefetch: true });
  }

  async function readProviderLink(
    pending: NonNullable<typeof providerLinkWindow>,
    options: { cancelRefetch: boolean }
  ) {
    const result = await identitiesQuery.refetch(options);
    if (
      !session.isCurrent(pending.scope) ||
      pending.userId !== accountId ||
      !result.isSuccess ||
      !result.data.providers.some(
        (provider) => provider.id === pending.providerId && provider.linked
      )
    )
      return false;
    resetProfileIdentities(pending.userId);
    return true;
  }

  // The provider window has no opener and can be on another origin. Confirm
  // completion through the account API, without waiting for a focus event.
  async function checkProviderLinkResult() {
    const pending = providerLinkWindow;
    if (!pending || checkingLink || !providerLinkIsCurrent()) return;
    checkingLink = true;
    try {
      if (
        (await readProviderLink(pending, { cancelRefetch: false })) &&
        providerLinkWindow === pending &&
        providerLinkIsCurrent()
      ) {
        providerLinkWindow = null;
        await currentUser.load();
        // The remote window closes itself. A detached cross-origin window
        // cannot be closed by its former opener in Chromium.
      }
    } finally {
      checkingLink = false;
    }
  }

  async function checkProviderLinkWindow() {
    const pending = providerLinkWindow;
    if (!pending) return;
    if (
      (!providerLinkIsCurrent() || (await pending.isClosed())) &&
      pending === providerLinkWindow
    ) {
      refreshAfterProviderLink();
      providerLinkWindow = null;
    }
  }

  function launchProviderWindow(provider: ExternalIdentityProviderInfo, url: string) {
    const userId = accountId;
    if (!userId) return;
    const popup = window.open(url, '_blank', authorizationWindowFeatures(window));
    if (!popup) {
      blockedLaunch = { url, provider, scope: session.snapshot(), userId };
      return;
    }
    popup.opener = null;
    blockedLaunch = null;
    providerLinkWindow = {
      isClosed: async () => popup.closed,
      scope: session.snapshot(),
      userId,
      providerId: provider.id
    };
  }

  function retryBlockedLaunch() {
    const pending = blockedLaunch;
    blockedLaunch = null;
    if (pending && session.isCurrent(pending.scope) && accountId === pending.userId) {
      launchProviderWindow(pending.provider, pending.url);
    }
  }

  function openProviderLink(provider: ExternalIdentityProviderInfo) {
    actionError = '';
    if (!accountId) return;
    if (serverRegistry.isOriginServer(serverScope.serverId)) {
      void startProviderLink(provider);
      return;
    }
    // Remote passwords belong on the account server's origin.
    const url = remoteIdentityLinkUrl(
      serverScope.connection.connectBaseUrl,
      provider.id,
      accountId
    );
    const popup = openAuthorizationWindow(
      `chatto-identity-link-${randomOAuthValue(12)}`,
      authorizationWindowFeatures(window)
    );
    if (!popup) {
      actionError = m('settings.account.sso.popup_blocked');
      return;
    }
    popup.detachOpener();
    const pending = {
      isClosed: popup.isClosed,
      scope: session.snapshot(),
      userId: accountId,
      providerId: provider.id
    };
    providerLinkWindow = pending;
    void popup.navigate(url).catch(() => {
      if (providerLinkWindow !== pending) return;
      if (providerLinkIsCurrent()) actionError = m('settings.account.sso.link_failed');
      providerLinkWindow = null;
      void popup.close();
    });
  }

  let linkConfirmation = $state.raw<{ provider: ExternalIdentityProviderInfo } | null>(null);
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
  let blockedDisconnectSubjectHash = $state('');
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
  const directLoginEnabled = $derived(serverScope.store.serverInfo.directLoginEnabled);
  const canUsePasswordProof = $derived(hasPassword && directLoginEnabled);
  const unconfiguredLinkedIdentities = $derived(
    linkedIdentities.filter(
      (identity) =>
        !providers.some((provider) => provider.linkedIdentitySubjectHash === identity.subjectHash)
    )
  );
  const hasRows = $derived(providers.length > 0 || unconfiguredLinkedIdentities.length > 0);
  // Older servers lack the hint, so retain their count-based fallback.
  // Explicit decisions remain authoritative until the list refreshes.
  function canDisconnectIdentity(subjectHash: string | null) {
    return disconnectAvailability(subjectHash) === DisconnectEligibility.ALLOWED;
  }

  /** Name of the linked account at the provider, when the server resolved one. */
  function linkedAccountLabel(subjectHash: string | null) {
    return linkedIdentities.find((identity) => identity.subjectHash === subjectHash)?.accountLabel;
  }

  function disconnectAvailability(subjectHash: string | null) {
    const identity = linkedIdentities.find((identity) => identity.subjectHash === subjectHash);
    return disconnectEligibilityWithFallback(
      identity?.disconnectEligibility,
      canUsePasswordProof,
      linkedIdentities.length
    );
  }

  async function startProviderLink(
    provider: ExternalIdentityProviderInfo,
    currentPassword?: string
  ) {
    const scope = session.snapshot();
    const userId = accountId;
    actionError = '';
    linkingProviderId = provider.id;
    try {
      const startUrl = await scope.connection.getAPI(createExternalIdentityAPI).startLink({
        providerId: provider.id,
        // The provider callback opens the link window in the popup.
        redirectPath: identityLinkReturnPath(provider.id, accountId ?? ''),
        currentPassword
      });
      if (!session.isCurrent(scope) || userId !== accountId) return;
      linkConfirmation = null;
      linkCurrentPassword = '';
      launchProviderWindow(provider, startUrl);
    } catch (err) {
      if (!session.isCurrent(scope) || userId !== accountId) return;
      const failure = linkStartFailure(err, {
        passwordSent: currentPassword !== undefined,
        canUsePasswordProof,
        directLoginEnabled,
        canReverify: reverifiers.length > 0
      });
      if (failure.kind === 'reverification_required') {
        requireReverification({ kind: 'link', provider });
      } else if (failure.kind === 'password_required') {
        linkConfirmation = { provider };
        linkCurrentPassword = '';
        linkFreshAuthError = '';
      } else if (failure.kind === 'password_rejected') {
        linkFreshAuthError = failure.message;
      } else {
        actionError = failure.message;
      }
    } finally {
      linkingProviderId = '';
    }
  }

  function closeLinkFreshAuthDialog() {
    if (linkingProviderId) return;
    linkConfirmation = null;
    linkCurrentPassword = '';
    linkFreshAuthError = '';
  }

  async function confirmLinkFreshAuth(e: Event) {
    e.preventDefault();
    if (!linkConfirmation || !linkCurrentPassword) {
      linkFreshAuthError = m('settings.account.password.current_required');
      return;
    }
    const { provider } = linkConfirmation;
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
    if (!canDisconnectIdentity(subjectHash)) {
      blockedDisconnectProviderLabel = providerLabel;
      blockedDisconnectSubjectHash = subjectHash;
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
    blockedDisconnectSubjectHash = '';
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
      if (accountId) resetProfileIdentities(accountId);
      disconnectTarget = null;
      disconnectFreshAuthTarget = null;
      disconnectCurrentPassword = '';
      disconnectFreshAuthError = '';
      await identitiesQuery.refetch();
      await currentUser.load();
    } catch (err) {
      if (!session.isCurrent(variables)) {
        return;
      }
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        disconnectTarget = null;
        // Another session can remove a method after the list was loaded. Read
        // the authoritative hint again before treating this as a freshness error.
        const refreshed = await identitiesQuery.refetch();
        if (!session.isCurrent(variables)) return;
        const identity = refreshed.data?.linkedIdentities.find(
          (identity) => identity.subjectHash === subjectHash
        );
        if (
          identity &&
          disconnectEligibilityWithFallback(
            identity.disconnectEligibility,
            canUsePasswordProof,
            refreshed.data?.linkedIdentities.length ?? 0
          ) !== DisconnectEligibility.ALLOWED
        ) {
          disconnectFreshAuthTarget = null;
          disconnectCurrentPassword = '';
          disconnectFreshAuthError = '';
          blockedDisconnectProviderLabel = providerLabel;
          blockedDisconnectSubjectHash = subjectHash;
          showDisconnectBlockedModal = true;
          return;
        }
        if (currentPassword !== undefined) {
          disconnectFreshAuthError = errorMessage(err, m('settings.account.sso.disconnect_failed'));
          return;
        }
        if (canUsePasswordProof) {
          disconnectFreshAuthTarget = { subjectHash, providerLabel };
          disconnectCurrentPassword = '';
          disconnectFreshAuthError = '';
        } else if (
          !disconnectReverified &&
          reverifiers.length > 0 &&
          serverRegistry.isOriginServer(serverScope.serverId)
        ) {
          requireReverification({ kind: 'disconnect', subjectHash, providerLabel });
        } else {
          actionError = directLoginEnabled
            ? m('settings.account.sso.disconnect_fresh_auth_required')
            : m('settings.account.sso.sso_fresh_auth_required');
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

  /** An action that the server protects with a recent sign-in. */
  type ProtectedAction =
    | { kind: 'link'; provider: ExternalIdentityProviderInfo }
    | { kind: 'disconnect'; subjectHash: string; providerLabel: string };

  // Linked providers that can confirm a recent sign-in without a password.
  const reverifiers = $derived(reverificationProviders(providers));
  // This page returned from a re-verification for a disconnect. A repeated
  // freshness failure then shows the fallback instead of another round trip.
  let disconnectReverified = false;
  let reverificationPrompt = $state.raw<{ action: ProtectedAction; error: string } | null>(null);
  let reverifyingProviderId = $state('');

  // Re-verify through the only provider that can, or let the user choose.
  function requireReverification(action: ProtectedAction) {
    if (reverifiers.length === 1) startReverification(reverifiers[0], action);
    else reverificationPrompt = { action, error: '' };
  }

  function startReverification(provider: ExternalIdentityProviderInfo, action: ProtectedAction) {
    const userId = accountId;
    if (!userId) return;
    actionError = '';
    reverificationPrompt = null;
    if (action.kind === 'link') {
      // The provider returns to the link window, which starts the link.
      launchProviderWindow(
        action.provider,
        reverificationStartUrl(provider, identityLinkPath(action.provider.id, userId))
      );
    } else {
      reverifyingProviderId = provider.id;
      leavePage(
        reverificationStartUrl(
          provider,
          disconnectContinuationPath(page.url.pathname, action.subjectHash, userId)
        )
      );
    }
  }

  function continueDisconnect(subjectHash: string, userId: string, errorCode: string) {
    if (!userId || userId !== accountId) {
      actionError = m('settings.account.sso.account_mismatch');
      return;
    }
    const identity = linkedIdentities.find((identity) => identity.subjectHash === subjectHash);
    if (!identity) return;
    const action: ProtectedAction = {
      kind: 'disconnect',
      subjectHash,
      providerLabel: identity.providerLabel
    };
    if (errorCode) {
      const message = reverificationErrorMessage(errorCode);
      if (reverifiers.length === 0) actionError = message;
      else reverificationPrompt = { action, error: message };
      return;
    }
    disconnectReverified = true;
    openDisconnectDialog(subjectHash, identity.providerLabel);
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
  <DisconnectContinuation ondisconnect={continueDisconnect} />
{/if}

<Panel title={m('settings.account.sso.title')} icon="iconify icon-[uil--link]" noPadding>
  {#if loading}
    <LoadingDots class="h-32 w-full" label={m('settings.account.sso.loading')} />
  {:else}
    {#if error}
      <div class="p-4">
        <Hint tone="danger">{error}</Hint>
      </div>
    {/if}
    {#if !hasRows}
      <p class="p-4 text-sm text-muted">{m('settings.account.sso.none_configured')}</p>
    {:else}
      <ul class="divide-y divide-border">
        {#each providers as provider (provider.id)}
          <li
            data-testid="sso-provider-row"
            class="flex items-center justify-between gap-4 px-4 py-3"
          >
            {@render identity(
              provider.type,
              provider.label,
              provider.linked,
              provider.linked
                ? m('settings.account.sso.linked')
                : m('settings.account.sso.not_linked'),
              provider.linkedIdentitySubjectHash
            )}
            {#if provider.linked}
              {#if provider.linkedIdentitySubjectHash}
                <Button
                  variant="danger-secondary"
                  size="sm"
                  loading={disconnectingSubjectHash === provider.linkedIdentitySubjectHash}
                  disabled={!canDisconnectIdentity(provider.linkedIdentitySubjectHash) ||
                    linkingProviderId !== '' ||
                    disconnectingSubjectHash !== ''}
                  onclick={() => openDisconnectProvider(provider)}
                >
                  <span aria-hidden="true" class="iconify icon-[uil--link-broken]"></span>
                  {disconnectButtonLabel(provider.linkedIdentitySubjectHash)}
                </Button>
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
          </li>
        {/each}

        {#each unconfiguredLinkedIdentities as linkedIdentity (linkedIdentity.subjectHash)}
          <li
            data-testid="sso-provider-row"
            class="flex items-center justify-between gap-4 px-4 py-3"
          >
            {@render identity(
              linkedIdentity.providerType,
              linkedIdentity.providerLabel,
              false,
              m('settings.account.sso.provider_unconfigured'),
              linkedIdentity.subjectHash
            )}
            <Button
              variant="danger-secondary"
              size="sm"
              loading={disconnectingSubjectHash === linkedIdentity.subjectHash}
              disabled={!canDisconnectIdentity(linkedIdentity.subjectHash) ||
                linkingProviderId !== '' ||
                disconnectingSubjectHash !== ''}
              onclick={() => openDisconnectIdentity(linkedIdentity)}
            >
              <span aria-hidden="true" class="iconify icon-[uil--link-broken]"></span>
              {disconnectButtonLabel(linkedIdentity.subjectHash)}
            </Button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</Panel>

<!-- Provider icon, name, link status, and any reason the identity cannot be disconnected. -->
{#snippet identity(
  type: string,
  label: string,
  linked: boolean,
  status: string,
  subjectHash: string | null
)}
  {@const account = linkedAccountLabel(subjectHash)}
  <div class="flex min-w-0 items-center gap-3">
    <span
      class={[
        'flex size-10 shrink-0 items-center justify-center rounded-md bg-surface text-xl',
        linked ? 'text-text' : 'text-muted'
      ]}
    >
      <span aria-hidden="true" class={['iconify', providerIcon(type)]}></span>
    </span>
    <div class="min-w-0">
      <div class="truncate font-medium">{label}</div>
      <div class="flex min-w-0 items-center gap-1 text-sm text-muted">
        <span class={['flex shrink-0 items-center gap-1', linked && 'text-success']}>
          {#if linked}
            <span aria-hidden="true" class="iconify icon-[uil--check-circle]"></span>
          {/if}
          {status}
        </span>
        {#if account}
          <span aria-hidden="true">·</span>
          <bdi class="truncate" dir="ltr" title={account}>{account}</bdi>
        {/if}
      </div>
      {#if subjectHash && !canDisconnectIdentity(subjectHash)}
        <p class="text-sm text-muted">
          {identityDisconnectUnavailableMessage(disconnectAvailability(subjectHash), label)}
        </p>
      {/if}
    </div>
  </div>
{/snippet}

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
    {identityDisconnectUnavailableMessage(
      disconnectAvailability(blockedDisconnectSubjectHash),
      blockedDisconnectProviderLabel
    )}
  </Hint>

  {#snippet dismissAction()}
    <Button defaultAction variant="secondary" onclick={closeDisconnectBlockedModal}>
      {m('ui.close')}
    </Button>
  {/snippet}
</Dialog>

{#if reverificationPrompt}
  {@const prompt = reverificationPrompt}
  <Dialog
    visible
    title={m('settings.account.sso.reverify_modal.title')}
    size="sm"
    onclose={() => {
      if (!reverifyingProviderId) reverificationPrompt = null;
    }}
  >
    <ReverificationChoice
      providers={reverifiers}
      error={prompt.error}
      busyProviderId={reverifyingProviderId}
      onselect={(provider) => startReverification(provider, prompt.action)}
    />
  </Dialog>
{/if}

{#if blockedLaunch}
  <ConfirmDialog
    visible
    tone="info"
    title={blockedLaunch.provider.label}
    actionLabel={m('auth.login.continue_with_provider', { provider: blockedLaunch.provider.label })}
    onconfirm={retryBlockedLaunch}
    onclose={() => (blockedLaunch = null)}
  >
    {m('settings.account.sso.popup_blocked')}
  </ConfirmDialog>
{/if}

{#if linkConfirmation}
  {@const freshAuthProvider = linkConfirmation.provider}
  <FormDialog
    visible
    title={m('settings.account.sso.fresh_auth_modal.title', { provider: freshAuthProvider.label })}
    size="sm"
    submitLabel={m('auth.login.continue_with_provider', { provider: freshAuthProvider.label })}
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
      label={m('settings.account.sso.fresh_auth_modal.password')}
      type="password"
      bind:value={linkCurrentPassword}
      disabled={linkingProviderId !== ''}
      autocomplete="current-password"
    />
  </FormDialog>
{/if}
