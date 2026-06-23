<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { PaneHeader, Dialog, FormSection, Hint } from '$lib/ui';
  import { TextInput, Button, FormError } from '$lib/ui/form';
  import { useQuery } from '$lib/hooks';
  import { notifyLogout } from '$lib/auth/sessionChannel';
  import { csrfFetch } from '$lib/auth/csrf';
  import * as m from '$lib/i18n/messages';

  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const connection = useConnection();

  // Check if the user has permission to delete their own account
  const permQuery = useQuery(
    graphql(`
      query AccountPermissions {
        viewer {
          user {
            viewerCanDeleteAccount
          }
        }
      }
    `),
    () => ({})
  );
  let canDeleteAccount = $derived(permQuery.data?.viewer?.user.viewerCanDeleteAccount ?? false);

  // Modal state
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
      // Step 1: Request a confirmation token (XSS protection)
      const tokenResult = await connection()
        .client.mutation(
          graphql(`
            mutation RequestAccountDeletion {
              requestAccountDeletion
            }
          `),
          {}
        )
        .toPromise();

      if (tokenResult.error) {
        error = tokenResult.error.message;
        return;
      }

      const confirmationToken = tokenResult.data?.requestAccountDeletion;
      if (!confirmationToken) {
        error = m['settings.account.delete_request_failed']();
        return;
      }

      // Step 2: Delete account with the confirmation token
      const result = await connection()
        .client.mutation(
          graphql(`
            mutation DeleteMyAccount($input: DeleteMyAccountInput!) {
              deleteMyAccount(input: $input)
            }
          `),
          { input: { confirmationToken } }
        )
        .toPromise();

      if (result.error) {
        error = result.error.message;
        return;
      }

      if (result.data?.deleteMyAccount) {
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
