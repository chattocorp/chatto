<script lang="ts">
  import { resolve } from '$app/paths';
  import { createAccountAPI } from '$lib/api/account';
  import { createExternalIdentityAPI, type SSOProvider } from '$lib/api/externalIdentities';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { PaneHeader, Dialog, FormSection, Hint } from '$lib/ui';
  import { TextInput, Button, FormError } from '$lib/ui/form';
  import { notifyLogout } from '$lib/auth/sessionChannel';
  import { csrfFetch } from '$lib/auth/csrf';
  import * as m from '$lib/i18n/messages';

  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const connection = useConnection();
  const serverId = $derived(getActiveServer());
  const accountSettingsPath = $derived(resolve('/chat/[serverId]/settings/account', { serverId }));

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
  let ssoProviders = $state.raw<SSOProvider[]>([]);
  let ssoLoading = $state(true);
  let ssoError = $state('');
  let linkingProviderId = $state('');

  const canDelete = $derived(confirmText === 'DELETE');

  $effect(() => {
    const client = connection();
    void loadExternalIdentities(client.serverId, client.connectBaseUrl, client.bearerToken);
  });

  async function loadExternalIdentities(
    serverId: string | undefined,
    baseUrl: string,
    bearerToken: string | null
  ) {
    ssoLoading = true;
    ssoError = '';
    try {
      const api = createExternalIdentityAPI({
        serverId,
        baseUrl,
        bearerToken
      });
      const result = await api.list();
      ssoProviders = result.providers;
    } catch (err) {
      ssoError = err instanceof Error ? err.message : m['settings.account.sso.load_failed']();
    } finally {
      ssoLoading = false;
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

  async function handleStartProviderLink(provider: SSOProvider) {
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
        redirectPath: accountSettingsPath
      });
      window.location.href = startUrl;
    } catch (err) {
      ssoError = err instanceof Error ? err.message : m['settings.account.sso.load_failed']();
      linkingProviderId = '';
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

  <FormSection title={m['settings.account.sso.title']()} maxWidth="max-w-md">
    <div class="flex flex-col gap-4">
      {#if ssoLoading}
        <p class="text-sm text-muted">{m['settings.account.sso.loading']()}</p>
      {:else if ssoError}
        <Hint tone="danger">{ssoError}</Hint>
      {:else if ssoProviders.length === 0}
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
                <span class="text-sm text-muted">{m['settings.account.sso.linked']()}</span>
              {:else}
                <Button
                  variant="secondary"
                  size="sm"
                  loading={linkingProviderId === provider.id}
                  disabled={linkingProviderId !== ''}
                  onclick={() => handleStartProviderLink(provider)}
                >
                  <span class="iconify uil--link"></span>
                  {m['settings.account.sso.link_button']()}
                </Button>
              {/if}
            </div>
          {/each}
        </div>
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
