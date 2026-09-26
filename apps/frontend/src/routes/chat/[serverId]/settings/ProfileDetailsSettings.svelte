<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { UserAPI } from '$lib/api-client/users';
  import UserBioEditor from '$lib/components/users/UserBioEditor.svelte';
  import { profileSaveErrorMessage } from '$lib/components/users/profileSaveError';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { m } from '$lib/i18n/messages';
  import { ConfirmDialog, Hint } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import {
    formatCooldownRemaining,
    getLoginChangeCooldownRemaining,
    MAX_BIO_LENGTH,
    validateAndNormalizeBio,
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin
  } from '$lib/validation';

  // The server route keys its subtree by server. Seed the local edit buffers
  // once so profile updates elsewhere cannot overwrite an in-progress edit.
  const serverScope = useServerScope();
  const currentUser = serverScope.store.currentUser;

  let { getUserAPI }: { getUserAPI: () => UserAPI } = $props();

  // Dirty checks compare against the seeded values, so a concurrent change to
  // an untouched field is not sent back with its stale value.
  const seed = {
    displayName: currentUser.user?.displayName ?? '',
    login: currentUser.user?.login ?? '',
    bio: currentUser.user?.bio ?? ''
  };
  let baseline = $state(seed);
  let displayName = $state(seed.displayName);
  let login = $state(seed.login);
  let bio = $state(seed.bio);
  let isSaving = $state(false);
  let error = $state('');
  let successMessage = $state('');
  let localLastLoginChange = $state<Date | null>(null);
  let showLoginConfirm = $state(false);
  let pendingDisplayName = $state<string | undefined>(undefined);
  let pendingLogin = $state<string | undefined>(undefined);
  let pendingBio = $state<string | undefined>(undefined);

  const viewerLastLoginChange = $derived(
    currentUser.user?.lastLoginChange ? new Date(currentUser.user.lastLoginChange) : null
  );
  const lastLoginChange = $derived(localLastLoginChange ?? viewerLastLoginChange);
  const displayNameModified = $derived(displayName !== baseline.displayName);
  const loginModified = $derived(login !== baseline.login);
  const bioModified = $derived(bio !== baseline.bio);
  const isModified = $derived(displayNameModified || loginModified || bioModified);
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const canBypassLoginCooldown = $derived(serverScope.store.permissions.canAdminManageAccounts);
  const canChangeLogin = $derived(canBypassLoginCooldown || cooldownRemaining === 0);

  function clearMessages() {
    error = '';
    successMessage = '';
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();

    let normalizedDisplayName: string | undefined;
    if (displayNameModified) {
      const validation = validateAndNormalizeDisplayName(displayName);
      if (!validation.valid) {
        error = validation.error ?? m('settings.profile.display_name.invalid');
        return;
      }
      normalizedDisplayName = validation.normalized;
    }

    let normalizedLogin: string | undefined;
    if (loginModified) {
      if (!canChangeLogin) {
        error = m('settings.profile.username.cooldown_error', {
          remaining: formatCooldownRemaining(cooldownRemaining)
        });
        return;
      }
      const validation = validateAndNormalizeLogin(login);
      if (!validation.valid) {
        error = validation.error ?? m('settings.profile.username.invalid');
        return;
      }
      normalizedLogin = validation.normalized;
    }

    let normalizedBio: string | undefined;
    if (bioModified) {
      const validation = validateAndNormalizeBio(bio);
      if (!validation.valid) {
        error = validation.error ?? m('settings.profile.save_failed');
        return;
      }
      normalizedBio = validation.normalized;
    }

    if (!normalizedDisplayName && !normalizedLogin && normalizedBio === undefined) return;

    if (normalizedLogin) {
      pendingDisplayName = normalizedDisplayName;
      pendingLogin = normalizedLogin;
      pendingBio = normalizedBio;
      showLoginConfirm = true;
      return;
    }

    await saveProfile(normalizedDisplayName, undefined, normalizedBio);
  }

  async function confirmLoginChange() {
    showLoginConfirm = false;
    await saveProfile(pendingDisplayName, pendingLogin, pendingBio);
    pendingDisplayName = undefined;
    pendingLogin = undefined;
  }

  async function saveProfile(
    normalizedDisplayName: string | undefined,
    normalizedLogin: string | undefined,
    normalizedBio?: string
  ) {
    const userId = currentUser.user?.id;
    if (!userId) return;
    isSaving = true;
    error = '';
    successMessage = '';

    try {
      const updated = await getUserAPI().updateUserProfile(userId, {
        displayName: normalizedDisplayName,
        login: normalizedLogin,
        bio: normalizedBio
      });

      if (currentUser.user) {
        const lastLoginChange =
          normalizedLogin && !canBypassLoginCooldown
            ? new Date().toISOString()
            : currentUser.user.lastLoginChange;
        currentUser.user = {
          ...currentUser.user,
          displayName: updated.displayName,
          login: updated.login,
          bio: updated.bio ?? '',
          lastLoginChange
        };
      }

      baseline = {
        displayName: updated.displayName,
        login: updated.login,
        bio: updated.bio ?? ''
      };
      displayName = baseline.displayName;
      login = baseline.login;
      bio = baseline.bio;

      if (normalizedLogin && !canBypassLoginCooldown) {
        localLastLoginChange = new Date();
      }

      successMessage = m('settings.profile.saved');
    } catch (saveError) {
      error = profileSaveErrorMessage(saveError, m('settings.profile.save_failed'));
    } finally {
      isSaving = false;
    }
  }
</script>

<Panel title={m('settings.profile.title')} icon="iconify icon-[uil--user]">
  <Form onsubmit={handleSubmit} maxWidth="max-w-md" {error}>
    <TextInput
      label={m('settings.profile.display_name.label')}
      bind:value={displayName}
      placeholder={m('settings.profile.display_name.placeholder')}
      disabled={isSaving}
      oninput={clearMessages}
    />

    <TextInput
      label={m('settings.profile.username.label')}
      bind:value={login}
      placeholder={m('settings.profile.username.placeholder')}
      disabled={isSaving || !canChangeLogin}
      testid="settings-username"
      oninput={clearMessages}
    />

    <UserBioEditor
      bind:value={bio}
      editorKind={userPreferences.composerEditor}
      maxlength={MAX_BIO_LENGTH}
      disabled={isSaving}
      oninput={clearMessages}
    />

    {#if !canChangeLogin}
      <p class="text-sm text-muted">
        {m('settings.profile.username.cooldown_notice', {
          remaining: formatCooldownRemaining(cooldownRemaining)
        })}
      </p>
    {/if}

    {#if successMessage}
      <Hint tone="success">{successMessage}</Hint>
    {/if}

    {#snippet footer()}
      <Button type="submit" disabled={!isModified || isSaving} loading={isSaving}>
        <span class="iconify icon-[uil--check]"></span>
        {m('settings.profile.save_button')}
      </Button>
    {/snippet}
  </Form>
</Panel>

<ConfirmDialog
  bind:visible={showLoginConfirm}
  title={m('settings.profile.username.confirm_title')}
  tone="info"
  actionLabel={m('settings.profile.username.confirm_button')}
  actionIcon="iconify icon-[uil--check]"
  onconfirm={confirmLoginChange}
  onclose={() => (showLoginConfirm = false)}
>
  <p>
    {m('settings.profile.username.confirm_prompt', { login: pendingLogin ?? '' })}
  </p>
  {#if !canBypassLoginCooldown}
    <p class="mt-3">{m('settings.profile.username.confirm_cooldown')}</p>
  {/if}
</ConfirmDialog>
