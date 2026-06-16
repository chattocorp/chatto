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
        error = 'Failed to request account deletion';
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
        error = 'Failed to delete account';
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete account';
    } finally {
      isDeleting = false;
    }
  }
</script>

<PaneHeader title="Account" subtitle="Manage your account settings" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Account Information -->
  <FormSection title="Account Information" maxWidth="max-w-md">
    <dl class="flex flex-col gap-3 text-sm">
      <div class="flex items-center justify-between">
        <dt class="text-muted">Username</dt>
        <dd class="font-mono">{currentUser.user?.login}</dd>
      </div>
      <div class="flex items-center justify-between">
        <dt class="text-muted">Display Name</dt>
        <dd>{currentUser.user?.displayName}</dd>
      </div>
    </dl>
  </FormSection>

  <!-- Danger Zone (only shown if user has permission to delete their own account) -->
  {#if canDeleteAccount}
    <div class="max-w-md border-t border-border pt-6">
      <h3 class="mb-2 text-sm font-semibold text-danger">Danger Zone</h3>
      <p class="mb-4 text-sm text-muted">
        Deleting your account is permanent and cannot be undone. All your data will be removed.
      </p>
      <Button variant="danger" onclick={openDeleteModal}>Delete Account</Button>
    </div>
  {/if}
</div>

<!-- Delete Account Confirmation Modal -->
<Dialog visible={showDeleteModal} title="Delete Account" size="sm" onclose={closeDeleteModal}>
  <div class="flex flex-col gap-4">
    <Hint tone="danger">
      <strong>Warning:</strong> This action is permanent and cannot be undone.
    </Hint>

    <p class="text-sm text-muted">This will permanently delete your account and:</p>
    <ul class="list-inside list-disc text-sm text-muted">
      <li>Remove you from all spaces and rooms</li>
      <li>Delete all your messages (content will be unrecoverable)</li>
      <li>Delete your profile, avatar, and associated data</li>
    </ul>

    <TextInput
      id="delete-confirm"
      label="Type DELETE to confirm"
      bind:value={confirmText}
      placeholder="DELETE"
      disabled={isDeleting}
      autocomplete="off"
    />

    {#if error}
      <FormError {error} />
    {/if}

    <div class="flex justify-end gap-3">
      <button
        type="button"
        class="cursor-pointer rounded-lg bg-surface-200 px-4 py-2 text-sm font-medium text-text hover:bg-surface-300"
        onclick={closeDeleteModal}
        disabled={isDeleting}
      >
        Cancel
      </button>
      <button
        type="button"
        class="cursor-pointer rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
        onclick={handleDeleteAccount}
        disabled={!canDelete || isDeleting}
      >
        {#if isDeleting}
          Deleting...
        {:else}
          Delete Account
        {/if}
      </button>
    </div>
  </div>
</Dialog>
