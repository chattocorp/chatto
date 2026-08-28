<script lang="ts">
  import type { AccountAPI } from '$lib/api-client/account';
  import { browserCookieAuthenticationHeaders } from '$lib/auth/authenticationMode';
  import { csrfFetch } from '$lib/auth/csrf';
  import { notifyLogout } from '$lib/auth/sessionChannel';
  import Panel from '$lib/ui/Panel.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { Dialog, Hint } from '$lib/ui';
  import { Button, FormError, TextInput } from '$lib/ui/form';

  let {
    canDeleteAccount,
    getAccountAPI
  }: {
    canDeleteAccount: boolean;
    getAccountAPI: () => AccountAPI;
  } = $props();

  let showDeleteModal = $state(false);
  let confirmText = $state('');
  let isDeleting = $state(false);
  let error = $state('');

  const canDelete = $derived(confirmText === 'DELETE');

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
      const confirmationToken = await getAccountAPI().requestAccountDeletion();
      if (!confirmationToken) {
        error = m('settings.account.delete_request_failed');
        return;
      }

      if (await getAccountAPI().deleteMyAccount(confirmationToken)) {
        const originToken = serverRegistry.originServer?.token;
        await csrfFetch('/auth/browser/logout', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...browserCookieAuthenticationHeaders,
            ...(originToken ? { Authorization: `Bearer ${originToken}` } : {})
          },
          body: '{}'
        });
        notifyLogout();
        window.location.href = '/';
      } else {
        error = m('settings.account.delete_failed');
      }
    } catch (err) {
      error = err instanceof Error ? err.message : m('settings.account.delete_failed');
    } finally {
      isDeleting = false;
    }
  }
</script>

{#if canDeleteAccount}
  <Panel title={m('settings.account.danger_title')} icon="iconify icon-[uil--exclamation-triangle]">
    <div class="max-w-md">
      <p class="mb-4 text-sm text-muted">
        {m('settings.account.danger_description')}
      </p>
      <Button variant="danger" onclick={openDeleteModal}>
        {m('settings.account.delete_button')}
      </Button>
    </div>
  </Panel>
{/if}

<Dialog
  visible={showDeleteModal}
  title={m('settings.account.delete_modal.title')}
  size="sm"
  onclose={closeDeleteModal}
>
  <div class="flex flex-col gap-4">
    <Hint tone="danger">
      <strong>{m('settings.account.delete_modal.warning_label')}</strong>
      {m('settings.account.delete_modal.warning_text')}
    </Hint>

    <p class="text-sm text-muted">{m('settings.account.delete_modal.intro')}</p>
    <ul class="list-inside list-disc text-sm text-muted">
      <li>{m('settings.account.delete_modal.remove_from_rooms')}</li>
      <li>{m('settings.account.delete_modal.delete_messages')}</li>
      <li>{m('settings.account.delete_modal.delete_profile')}</li>
    </ul>

    <TextInput
      id="delete-confirm"
      label={m('settings.account.delete_modal.confirm_label')}
      bind:value={confirmText}
      placeholder={m('settings.account.delete_modal.confirm_placeholder')}
      disabled={isDeleting}
      autocomplete="off"
    />

    {#if error}
      <FormError {error} />
    {/if}

    <div class="flex flex-wrap justify-end gap-2">
      <Button variant="secondary" onclick={closeDeleteModal} disabled={isDeleting}>
        {m('common.cancel')}
      </Button>
      <Button
        defaultAction
        variant="danger"
        onclick={handleDeleteAccount}
        disabled={!canDelete || isDeleting}
        loading={isDeleting}
        loadingText={m('settings.account.delete_modal.deleting')}
      >
        {m('settings.account.delete_button')}
      </Button>
    </div>
  </div>
</Dialog>
