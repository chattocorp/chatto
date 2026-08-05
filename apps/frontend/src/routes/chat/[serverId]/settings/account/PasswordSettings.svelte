<script lang="ts">
  import { Code, ConnectError } from '@connectrpc/connect';
  import type { AccountAPI } from '$lib/api-client/account';
  import type { CurrentUserState } from '$lib/auth/currentUser.svelte';
  import * as m from '$lib/i18n/messages';
  import { FormSection } from '$lib/ui';
  import { Button, FormError, TextInput, validate, z } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast/toastState.svelte';

  let {
    currentUser,
    getAccountAPI
  }: {
    currentUser: CurrentUserState;
    getAccountAPI: () => AccountAPI;
  } = $props();

  let currentPassword = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let passwordError = $state('');
  let passwordSubmitting = $state(false);

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
      await getAccountAPI().updatePassword({
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
</script>

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
        <span class="iconify icon-[mdi--key-plus]"></span>
        {hasPassword
          ? m['settings.account.password.change_button']()
          : m['settings.account.password.add_button']()}
      </Button>
    </div>
  </form>
</FormSection>
