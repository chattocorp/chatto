<script lang="ts">
  import { passwordValidationError as validateNewPassword } from '$lib/passwordValidation';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { passwordChangeUnavailableMessage } from '$lib/actionAvailability';
  import {
    PasswordChangeAvailability,
    passwordChangeAvailabilityWithFallback
  } from '@chatto/client/api/actionAvailability';
  import { Code, ConnectError } from '@connectrpc/connect';
  import type { AccountAPI } from '@chatto/client/api/account';
  import type { CurrentUserState } from '@chatto/client/auth/currentUser';
  import { Hint, Panel } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { Button, FormError, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  let {
    currentUser,
    directLoginEnabled,
    getAccountAPI,
    onPasswordSet
  }: {
    currentUser: CurrentUserState;
    /** Server policy also controls password self-service. */
    directLoginEnabled: boolean;
    getAccountAPI: () => AccountAPI;
    /** Refresh identity actions after adding a new sign-in method. */
    onPasswordSet?: () => Promise<void>;
  } = $props();

  let currentPassword = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let passwordError = $state('');
  let passwordSubmitting = $state(false);

  const hasPassword = $derived(currentUser.user?.hasPassword ?? false);
  const passwordAvailability = $derived(
    passwordChangeAvailabilityWithFallback(
      currentUser.user?.actions?.changePassword,
      directLoginEnabled
    )
  );
  const passwordValidationError = $derived(password ? validateNewPassword(password) : undefined);
  const currentPasswordError = $derived(
    hasPassword && password && !currentPassword
      ? m('settings.account.password.current_required')
      : undefined
  );
  const confirmPasswordError = $derived(
    confirmPassword && password !== confirmPassword
      ? m('common.validation.passwords_match')
      : undefined
  );
  const canChangePassword = $derived(
    passwordAvailability === PasswordChangeAvailability.ALLOWED &&
      password !== '' &&
      confirmPassword !== '' &&
      (!hasPassword || currentPassword !== '') &&
      !passwordValidationError &&
      !currentPasswordError &&
      !confirmPasswordError &&
      !passwordSubmitting
  );

  async function handleChangePassword(e: Event) {
    e.preventDefault();
    if (passwordAvailability !== PasswordChangeAvailability.ALLOWED) return;
    if (!canChangePassword) {
      passwordError =
        passwordValidationError ||
        currentPasswordError ||
        confirmPasswordError ||
        m('common.validation.fix_errors');
      return;
    }

    const wasChangingPassword = hasPassword;
    passwordSubmitting = true;
    passwordError = '';
    try {
      await getAccountAPI().changePassword({
        password,
        currentPassword: wasChangingPassword ? currentPassword : undefined
      });
      currentPassword = '';
      password = '';
      confirmPassword = '';
      if (!wasChangingPassword) {
        await currentUser.load();
        await onPasswordSet?.();
      }
      toast.success(
        wasChangingPassword
          ? m('settings.account.password.changed')
          : m('settings.account.password.saved')
      );
    } catch (err) {
      if (
        err instanceof ConnectError &&
        (err.code === Code.FailedPrecondition || err.code === Code.PermissionDenied)
      ) {
        await currentUser.load();
      }
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        passwordError = wasChangingPassword
          ? m('settings.account.password.already_set')
          : m('settings.account.password.fresh_auth_required');
      } else {
        passwordError = errorMessage(err, m('settings.account.password.save_failed'));
      }
    } finally {
      passwordSubmitting = false;
    }
  }
</script>

<Panel title={m('settings.account.password.title')} icon="iconify icon-[uil--key-skeleton]">
  <form class="flex max-w-md flex-col gap-4" onsubmit={handleChangePassword}>
    {#if passwordAvailability !== PasswordChangeAvailability.ALLOWED}
      <Hint>{passwordChangeUnavailableMessage(passwordAvailability)}</Hint>
    {:else}
      <p class="text-sm text-muted">
        {hasPassword
          ? m('settings.account.password.change_description')
          : m('settings.account.password.add_description')}
      </p>
    {/if}
    {#if hasPassword}
      <TextInput
        id="current-password"
        label={m('settings.account.password.current_label')}
        type="password"
        bind:value={currentPassword}
        disabled={passwordAvailability !== PasswordChangeAvailability.ALLOWED || passwordSubmitting}
        autocomplete="current-password"
        error={currentPasswordError}
      />
    {/if}
    <TextInput
      id="add-password"
      label={m('common.new_password')}
      type="password"
      bind:value={password}
      placeholder={m('common.password_bytes_placeholder')}
      disabled={passwordAvailability !== PasswordChangeAvailability.ALLOWED || passwordSubmitting}
      autocomplete="new-password"
      error={passwordValidationError}
    />
    <TextInput
      id="add-password-confirm"
      label={m('common.confirm_password')}
      type="password"
      bind:value={confirmPassword}
      placeholder={m('common.password_confirm_placeholder')}
      disabled={passwordAvailability !== PasswordChangeAvailability.ALLOWED || passwordSubmitting}
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
        loadingText={m('settings.account.password.saving')}
        disabled={!canChangePassword}
      >
        <span aria-hidden="true" class="iconify icon-[mdi--key-plus]"></span>
        {hasPassword
          ? m('settings.account.password.change_button')
          : m('settings.account.password.add_button')}
      </Button>
    </div>
  </form>
</Panel>
