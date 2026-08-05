<script lang="ts">
  import { Panel } from '$lib/components/admin';
  import * as m from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Button, Form, FormError, TextInput, validate, z } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { untrack } from 'svelte';
  import type { AdminManagedUser, AdminMember } from '$lib/api-client/adminUsers';
  import {
    formatCooldownRemaining,
    getLoginChangeCooldownRemaining,
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin
  } from '$lib/validation';

  type Props = {
    member: AdminMember;
    isSelf: boolean;
    updateIdentity: (input: {
      login?: string;
      displayName?: string;
    }) => Promise<AdminManagedUser | null>;
    clearUsernameCooldown: () => Promise<boolean>;
    updatePassword: (password: string) => Promise<AdminMember | null>;
  };

  let {
    member,
    isSelf,
    updateIdentity,
    clearUsernameCooldown,
    updatePassword
  }: Props = $props();

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  // These are edit buffers, not mirrors. The parent unmounts this component
  // while switching members, so capture the current values once per member.
  let editLogin = $state(untrack(() => member.login));
  let editDisplayName = $state(untrack(() => member.displayName));
  let identityError = $state<string | null>(null);
  let savingIdentity = $state(false);
  let clearingCooldown = $state(false);
  let adminPassword = $state('');
  let adminConfirmPassword = $state('');
  let passwordError = $state<string | null>(null);
  let settingPassword = $state(false);

  const loginModified = $derived(!!member && editLogin !== member.login);
  const displayNameModified = $derived(!!member && editDisplayName !== member.displayName);
  const identityModified = $derived(loginModified || displayNameModified);
  const lastLoginChange = $derived(
    member?.lastLoginChange ? new Date(member.lastLoginChange) : null
  );
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const cooldownActive = $derived(cooldownRemaining > 0);
  const passwordSchema = z.string().min(8, m['common.validation.password_min']());
  const adminPasswordValidationError = $derived(
    adminPassword ? validate(passwordSchema, adminPassword) : undefined
  );
  const adminConfirmPasswordError = $derived(
    adminConfirmPassword && adminPassword !== adminConfirmPassword
      ? m['common.validation.passwords_match']()
      : undefined
  );
  const canSetMemberPassword = $derived(
    !!member &&
      adminPassword !== '' &&
      adminConfirmPassword !== '' &&
      !adminPasswordValidationError &&
      !adminConfirmPasswordError &&
      !settingPassword
  );

  async function saveIdentity(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!member || !identityModified || savingIdentity) return;

    identityError = null;
    const input: { login?: string; displayName?: string } = {};

    if (displayNameModified) {
      const result = validateAndNormalizeDisplayName(editDisplayName);
      if (!result.valid || result.normalized === undefined) {
        identityError = result.error ?? 'Invalid display name';
        return;
      }
      input.displayName = result.normalized;
    }

    if (loginModified) {
      const result = validateAndNormalizeLogin(editLogin);
      if (!result.valid || result.normalized === undefined) {
        identityError = result.error ?? 'Invalid username';
        return;
      }
      input.login = result.normalized;
    }

    savingIdentity = true;
    try {
      const updated = await updateIdentity(input);
      if (updated) {
        editLogin = updated.login;
        editDisplayName = updated.displayName;
        toast.success('User updated');
      }
    } catch (error) {
      identityError = error instanceof Error ? error.message : 'Failed to update user';
    } finally {
      savingIdentity = false;
    }
  }

  function resetIdentity(): void {
    if (!member) return;
    editLogin = member.login;
    editDisplayName = member.displayName;
    identityError = null;
  }

  async function clearCooldown(): Promise<void> {
    if (!member || clearingCooldown) return;

    clearingCooldown = true;
    identityError = null;
    try {
      if (await clearUsernameCooldown()) {
        toast.success('Username change cooldown cleared');
      }
    } catch (error) {
      identityError = error instanceof Error ? error.message : 'Failed to clear username cooldown';
    } finally {
      clearingCooldown = false;
    }
  }

  async function setMemberPassword(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!member || !canSetMemberPassword) {
      passwordError =
        adminPasswordValidationError ||
        adminConfirmPasswordError ||
        m['common.validation.fix_errors']();
      return;
    }

    settingPassword = true;
    passwordError = null;
    try {
      if (await updatePassword(adminPassword)) {
        adminPassword = '';
        adminConfirmPassword = '';
        toast.success(m['admin.members.password_set']());
      }
    } catch (error) {
      passwordError =
        error instanceof Error ? error.message : m['admin.members.set_password_failed']();
    } finally {
      settingPassword = false;
    }
  }
</script>

{#if member}
  <Panel title={m['admin.members.identity']()} icon="iconify icon-[uil--edit]">
    <Form onsubmit={saveIdentity} error={identityError}>
      <TextInput
        id="member-login"
        testid="admin-identity-login"
        label={m['common.username']()}
        bind:value={editLogin}
        disabled={savingIdentity}
        description={m['admin.members.admin_rename_description']()}
      />
      <TextInput
        id="member-display-name"
        testid="admin-identity-display-name"
        label={m['settings.profile.display_name.label']()}
        bind:value={editDisplayName}
        disabled={savingIdentity}
      />
      {#snippet footer()}
        <Button
          type="submit"
          disabled={!identityModified || savingIdentity}
          loading={savingIdentity}
          loadingText={m['rbac.role_form.saving']()}
        >
          {m['rbac.role_form.save']()}
        </Button>
        <Button
          type="button"
          variant="ghost"
          onclick={resetIdentity}
          disabled={!identityModified || savingIdentity}
        >
          {m['admin.members.reset']()}
        </Button>
      {/snippet}
      <div class="flex items-center gap-3 surface-box p-3">
        <div class="flex-1 text-sm text-muted">
          {#if cooldownActive}
            {m['admin.members.cooldown_active']({
              remaining: formatCooldownRemaining(cooldownRemaining)
            })}
          {:else if lastLoginChange}
            {m['admin.members.last_self_rename']({
              time: formatDateTime(lastLoginChange, userSettings, activeLocale)
            })}
          {:else}
            {m['admin.members.never_renamed']()}
          {/if}
        </div>
        <Button
          type="button"
          variant="ghost"
          onclick={clearCooldown}
          disabled={!cooldownActive}
          loading={clearingCooldown}
          loadingText={m['admin.members.clearing']()}
        >
          {m['admin.members.reset_cooldown']()}
        </Button>
      </div>
    </Form>

    {#if !isSelf}
      <form
        class="mt-6 flex flex-col gap-4 border-t border-border pt-6"
        onsubmit={setMemberPassword}
      >
        <div>
          <h4 class="text-sm font-semibold">{m['admin.members.set_password']()}</h4>
          <p class="mt-1 text-sm text-muted">
            {m['admin.members.set_password_description']()}
          </p>
        </div>
        <TextInput
          id="admin-member-password"
          label={m['common.new_password']()}
          type="password"
          bind:value={adminPassword}
          placeholder={m['common.password_min_placeholder']()}
          disabled={settingPassword}
          autocomplete="new-password"
          error={adminPasswordValidationError}
        />
        <TextInput
          id="admin-member-password-confirm"
          label={m['common.confirm_password']()}
          type="password"
          bind:value={adminConfirmPassword}
          placeholder={m['common.password_confirm_placeholder']()}
          disabled={settingPassword}
          autocomplete="new-password"
          error={adminConfirmPasswordError}
        />
        {#if passwordError}
          <FormError error={passwordError} />
        {/if}
        <div>
          <Button
            type="submit"
            loading={settingPassword}
            loadingText={m['admin.members.setting_password']()}
            disabled={!canSetMemberPassword}
          >
            <span class="iconify icon-[mdi--key-change]"></span>
            {m['admin.members.set_password']()}
          </Button>
        </div>
      </form>
    {/if}
  </Panel>
{/if}
