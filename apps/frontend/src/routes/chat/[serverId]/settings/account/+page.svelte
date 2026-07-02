<script lang="ts">
  import { Code, ConnectError } from '@connectrpc/connect';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import {
    createExternalIdentityAPI,
    type ExternalIdentityProviderInfo,
    type LinkedExternalIdentityInfo
  } from '$lib/api-client/externalIdentities';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createAccountAPI } from '$lib/api-client/account';
  import { PaneHeader, ConfirmDialog, Dialog, FormSection, Hint } from '$lib/ui';
  import { TextInput, Button, FormError, z, validate } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast/toastState.svelte';
  import { notifyLogout } from '$lib/auth/sessionChannel';
  import { csrfFetch } from '$lib/auth/csrf';
  import * as m from '$lib/i18n/messages';

  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const connection = useConnection();
  const serverId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(serverId));
  const accountSettingsPath = $derived(
    resolve('/chat/[serverId]/settings/account', { serverId: serverSegment })
  );
  let ssoLoadSerial = 0;

  const canDeleteAccount = $derived(currentUser.user?.viewerCanDeleteAccount ?? false);

  function accountAPI() {
    const conn = connection();
    return createAccountAPI({
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  }

  // Modal state
  let showDeleteModal = $state(false);
  let confirmText = $state('');
  let isDeleting = $state(false);
  let error = $state('');
  let ssoProviders = $state.raw<ExternalIdentityProviderInfo[]>([]);
  let linkedSSOIdentities = $state.raw<LinkedExternalIdentityInfo[]>([]);
  let ssoLoading = $state(true);
  let ssoError = $state('');
  let linkingProviderId = $state('');
  let linkFreshAuthProvider = $state<ExternalIdentityProviderInfo | null>(null);
  let linkCurrentPassword = $state('');
  let linkFreshAuthError = $state('');
  let disconnectingSubjectHash = $state('');
  let disconnectTarget = $state<{ subjectHash: string; providerLabel: string } | null>(null);
  let disconnectFreshAuthTarget = $state<{ subjectHash: string; providerLabel: string } | null>(null);
  let disconnectCurrentPassword = $state('');
  let disconnectFreshAuthError = $state('');
  let blockedDisconnectProviderLabel = $state('');
  let showDisconnectBlockedModal = $state(false);
  let currentPassword = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let passwordError = $state('');
  let passwordSubmitting = $state(false);

  const canDelete = $derived(confirmText === 'DELETE');
  const hasPassword = $derived(currentUser.user?.hasPassword ?? false);
  const passwordSchema = z.string().min(8, m['common.validation.password_min']());
  const passwordValidationError = $derived(
    password ? validate(passwordSchema, password) : undefined
  );
  const currentPasswordError = $derived(
    hasPassword && password && !currentPassword
      ? m['settings.account.password.current_required']()
      : undefined
  );
  const confirmPasswordError = $derived(
    confirmPassword && password !== confirmPassword
      ? m['common.validation.passwords_match']()
      : undefined
  );
  const canUpdatePassword = $derived(
    password !== '' &&
      confirmPassword !== '' &&
      (!hasPassword || currentPassword !== '') &&
      !passwordValidationError &&
      !currentPasswordError &&
      !confirmPasswordError &&
      !passwordSubmitting
  );
  const unconfiguredLinkedIdentities = $derived(
    linkedSSOIdentities.filter(
      (identity) =>
        !ssoProviders.some(
          (provider) => provider.linkedIdentitySubjectHash === identity.subjectHash
        )
    )
  );
  const hasSSORows = $derived(
    ssoProviders.length > 0 || unconfiguredLinkedIdentities.length > 0
  );
  const disconnectWouldRemoveLastMethod = $derived(
    !hasPassword && linkedSSOIdentities.length <= 1
  );

  $effect(() => {
    void refreshExternalIdentities();
  });

  async function refreshExternalIdentities() {
    const activeServerId = serverId;
    const client = connection();
    const loadSerial = ++ssoLoadSerial;
    await loadExternalIdentities(
      loadSerial,
      activeServerId,
      client.serverId,
      client.connectBaseUrl,
      client.bearerToken
    );
  }

  async function loadExternalIdentities(
    loadSerial: number,
    activeServerId: string,
    apiServerId: string | undefined,
    baseUrl: string,
    bearerToken: string | null
  ) {
    ssoLoading = true;
    ssoError = '';
    try {
      const api = createExternalIdentityAPI({
        serverId: apiServerId,
        baseUrl,
        bearerToken
      });
      const result = await api.list();
      if (loadSerial !== ssoLoadSerial || activeServerId !== getActiveServer()) {
        return;
      }
      ssoProviders = result.providers;
      linkedSSOIdentities = result.linkedIdentities;
    } catch (err) {
      if (loadSerial !== ssoLoadSerial || activeServerId !== getActiveServer()) {
        return;
      }
      ssoError = err instanceof Error ? err.message : m['settings.account.sso.load_failed']();
    } finally {
      if (loadSerial === ssoLoadSerial && activeServerId === getActiveServer()) {
        ssoLoading = false;
      }
    }
  }

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

  async function handleStartProviderLink(provider: ExternalIdentityProviderInfo) {
    await startProviderLink(provider);
  }

  async function startProviderLink(
    provider: ExternalIdentityProviderInfo,
    currentPassword?: string
  ) {
    const client = connection();
    linkingProviderId = provider.id;
    ssoError = '';
    try {
      const api = createExternalIdentityAPI({
        serverId: client.serverId,
        baseUrl: client.connectBaseUrl,
        bearerToken: client.bearerToken
      });
      const startUrl = await api.startLink({
        providerId: provider.id,
        redirectPath: accountSettingsPath,
        currentPassword
      });
      window.location.href = startUrl;
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition && hasPassword) {
        linkFreshAuthProvider = provider;
        linkCurrentPassword = '';
        linkFreshAuthError = '';
      } else if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        ssoError = m['settings.account.sso.fresh_auth_required']();
      } else if (currentPassword !== undefined) {
        linkFreshAuthError =
          err instanceof Error ? err.message : m['settings.account.sso.link_failed']();
      } else {
        ssoError = err instanceof Error ? err.message : m['settings.account.sso.link_failed']();
      }
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
    ssoError = '';
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
    const client = connection();
    disconnectingSubjectHash = subjectHash;
    ssoError = '';
    try {
      const api = createExternalIdentityAPI({
        serverId: client.serverId,
        baseUrl: client.connectBaseUrl,
        bearerToken: client.bearerToken
      });
      await api.disconnect(subjectHash, currentPassword);
      disconnectTarget = null;
      disconnectFreshAuthTarget = null;
      disconnectCurrentPassword = '';
      disconnectFreshAuthError = '';
      serverRegistry.handleAuthenticationRequired(client.serverId ?? serverId);
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        disconnectTarget = null;
        if (hasPassword) {
          disconnectFreshAuthTarget = { subjectHash, providerLabel };
          disconnectCurrentPassword = '';
          disconnectFreshAuthError = '';
        } else {
          ssoError = m['settings.account.sso.disconnect_fresh_auth_required']();
        }
      } else if (currentPassword !== undefined) {
        disconnectFreshAuthError =
          err instanceof Error ? err.message : m['settings.account.sso.disconnect_failed']();
      } else {
        ssoError =
          err instanceof Error ? err.message : m['settings.account.sso.disconnect_failed']();
        disconnectTarget = null;
      }
    } finally {
      disconnectingSubjectHash = '';
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

  async function handleUpdatePassword(e: Event) {
    e.preventDefault();
    if (!canUpdatePassword) {
      passwordError =
        passwordValidationError ||
        currentPasswordError ||
        confirmPasswordError ||
        m['common.validation.fix_errors']();
      return;
    }

    const wasChangingPassword = hasPassword;
    passwordSubmitting = true;
    passwordError = '';
    try {
      await accountAPI().updatePassword({
        password,
        currentPassword: wasChangingPassword ? currentPassword : undefined
      });
      currentPassword = '';
      password = '';
      confirmPassword = '';
      if (!wasChangingPassword) {
        await currentUser.load();
      }
      toast.success(
        wasChangingPassword
          ? m['settings.account.password.changed']()
          : m['settings.account.password.saved']()
      );
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        passwordError = wasChangingPassword
          ? m['settings.account.password.already_set']()
          : m['settings.account.password.fresh_auth_required']();
      } else {
        passwordError =
          err instanceof Error ? err.message : m['settings.account.password.save_failed']();
      }
    } finally {
      passwordSubmitting = false;
    }
  }

  function openDeleteModal() {
    confirmText = '';
    error = '';
    showDeleteModal = true;
  }

  function closeDeleteModal() {
    showDeleteModal = false;
    confirmText = '';
    error = '';
  }

  async function handleDeleteAccount() {
    if (!canDelete) return;

    isDeleting = true;
    error = '';

    try {
      // Step 1: Request a confirmation token (XSS protection)
      const confirmationToken = await accountAPI().requestAccountDeletion();
      if (!confirmationToken) {
        error = m['settings.account.delete_request_failed']();
        return;
      }

      // Step 2: Delete account with the confirmation token
      if (await accountAPI().deleteMyAccount(confirmationToken)) {
        // Log out and redirect to home
        const originToken = serverRegistry.originServer?.token;
        await csrfFetch('/auth/logout', {
          method: 'POST',
          headers: originToken ? { Authorization: `Bearer ${originToken}` } : undefined
        });
        notifyLogout();
        window.location.href = '/';
      } else {
        error = m['settings.account.delete_failed']();
      }
    } catch (err) {
      error = err instanceof Error ? err.message : m['settings.account.delete_failed']();
    } finally {
      isDeleting = false;
    }
  }
</script>

<PaneHeader
  title={m['settings.account.title']()}
  subtitle={m['settings.account.subtitle']()}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Account Information -->
  <FormSection title={m['settings.account.info_title']()} maxWidth="max-w-md">
    <dl class="flex flex-col gap-3 text-sm">
      <div class="flex items-center justify-between">
        <dt class="text-muted">{m['settings.account.username']()}</dt>
        <dd class="font-mono">{currentUser.user?.login}</dd>
      </div>
      <div class="flex items-center justify-between">
        <dt class="text-muted">{m['settings.account.display_name']()}</dt>
        <dd>{currentUser.user?.displayName}</dd>
      </div>
    </dl>
  </FormSection>

  <FormSection title={m['settings.account.password.title']()} maxWidth="max-w-md">
    <form class="flex flex-col gap-4" onsubmit={handleUpdatePassword}>
      <p class="text-sm text-muted">
        {hasPassword
          ? m['settings.account.password.change_description']()
          : m['settings.account.password.add_description']()}
      </p>
      {#if hasPassword}
        <TextInput
          id="current-password"
          label={m['settings.account.password.current_label']()}
          type="password"
          bind:value={currentPassword}
          disabled={passwordSubmitting}
          autocomplete="current-password"
          error={currentPasswordError}
        />
      {/if}
      <TextInput
        id="add-password"
        label={m['common.new_password']()}
        type="password"
        bind:value={password}
        placeholder={m['common.password_min_placeholder']()}
        disabled={passwordSubmitting}
        autocomplete="new-password"
        error={passwordValidationError}
      />
      <TextInput
        id="add-password-confirm"
        label={m['common.confirm_password']()}
        type="password"
        bind:value={confirmPassword}
        placeholder={m['common.password_confirm_placeholder']()}
        disabled={passwordSubmitting}
        autocomplete="new-password"
        error={confirmPasswordError}
      />
      {#if passwordError}
        <FormError error={passwordError} />
      {/if}
      <div>
        <Button
          type="submit"
          loading={passwordSubmitting}
          loadingText={m['settings.account.password.saving']()}
          disabled={!canUpdatePassword}
        >
          <span class="iconify mdi--key-plus"></span>
          {hasPassword
            ? m['settings.account.password.change_button']()
            : m['settings.account.password.add_button']()}
        </Button>
      </div>
    </form>
  </FormSection>

  <FormSection title={m['settings.account.sso.title']()} maxWidth="max-w-md">
    <div class="flex flex-col gap-4">
      {#if ssoLoading}
        <p class="text-sm text-muted">{m['settings.account.sso.loading']()}</p>
      {:else}
        {#if ssoError}
          <Hint tone="danger">{ssoError}</Hint>
        {/if}
        {#if !hasSSORows}
          <p class="text-sm text-muted">{m['settings.account.sso.none_configured']()}</p>
        {:else}
          <div class="flex flex-col gap-3">
            {#each ssoProviders as provider (provider.id)}
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
                    <button
                      type="button"
                      class="btn-secondary btn-sm hover:!from-danger/65 hover:!to-danger/95 hover:!text-white hover:!ring-danger/30"
                      aria-busy={
                        disconnectingSubjectHash === provider.linkedIdentitySubjectHash ||
                        undefined
                      }
                      disabled={linkingProviderId !== '' || disconnectingSubjectHash !== ''}
                      onclick={() => openDisconnectProvider(provider)}
                    >
                      <span class="iconify uil--link-broken"></span>
                      {disconnectButtonLabel(provider.linkedIdentitySubjectHash)}
                    </button>
                  {:else}
                    <span class="text-sm text-muted">{m['settings.account.sso.linked']()}</span>
                  {/if}
                {:else}
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={linkingProviderId === provider.id}
                    disabled={linkingProviderId !== '' || disconnectingSubjectHash !== ''}
                    onclick={() => handleStartProviderLink(provider)}
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
                  <span
                    class={['iconify text-lg text-muted', providerIcon(identity.providerType)]}
                  ></span>
                  <div class="min-w-0">
                    <div class="truncate text-sm font-medium">{identity.providerLabel}</div>
                    <div class="text-xs text-muted">
                      {m['settings.account.sso.provider_unconfigured']()}
                    </div>
                  </div>
                </div>
                <button
                  type="button"
                  class="btn-secondary btn-sm hover:!from-danger/65 hover:!to-danger/95 hover:!text-white hover:!ring-danger/30"
                  aria-busy={disconnectingSubjectHash === identity.subjectHash || undefined}
                  disabled={linkingProviderId !== '' || disconnectingSubjectHash !== ''}
                  onclick={() => openDisconnectIdentity(identity)}
                >
                  <span class="iconify uil--link-broken"></span>
                  {disconnectButtonLabel(identity.subjectHash)}
                </button>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  </FormSection>

  <!-- Danger Zone (only shown if user has permission to delete their own account) -->
  {#if canDeleteAccount}
    <div class="max-w-md border-t border-border pt-6">
      <h3 class="mb-2 text-sm font-semibold text-danger">{m['settings.account.danger_title']()}</h3>
      <p class="mb-4 text-sm text-muted">
        {m['settings.account.danger_description']()}
      </p>
      <Button variant="danger" onclick={openDeleteModal}>
        {m['settings.account.delete_button']()}
      </Button>
    </div>
  {/if}
</div>

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

<!-- Delete Account Confirmation Modal -->
<Dialog
  visible={showDeleteModal}
  title={m['settings.account.delete_modal.title']()}
  size="sm"
  onclose={closeDeleteModal}
>
  <div class="flex flex-col gap-4">
    <Hint tone="danger">
      <strong>{m['settings.account.delete_modal.warning_label']()}</strong>
      {m['settings.account.delete_modal.warning_text']()}
    </Hint>

    <p class="text-sm text-muted">{m['settings.account.delete_modal.intro']()}</p>
    <ul class="list-inside list-disc text-sm text-muted">
      <li>{m['settings.account.delete_modal.remove_from_rooms']()}</li>
      <li>{m['settings.account.delete_modal.delete_messages']()}</li>
      <li>{m['settings.account.delete_modal.delete_profile']()}</li>
    </ul>

    <TextInput
      id="delete-confirm"
      label={m['settings.account.delete_modal.confirm_label']()}
      bind:value={confirmText}
      placeholder={m['settings.account.delete_modal.confirm_placeholder']()}
      disabled={isDeleting}
      autocomplete="off"
    />

    {#if error}
      <FormError {error} />
    {/if}

    <div class="flex flex-wrap justify-end gap-2">
      <Button variant="secondary" onclick={closeDeleteModal} disabled={isDeleting}>
        {m['common.cancel']()}
      </Button>
      <Button
        variant="danger"
        onclick={handleDeleteAccount}
        disabled={!canDelete || isDeleting}
        loading={isDeleting}
        loadingText={m['settings.account.delete_modal.deleting']()}
      >
        {m['settings.account.delete_button']()}
      </Button>
    </div>
  </div>
</Dialog>
