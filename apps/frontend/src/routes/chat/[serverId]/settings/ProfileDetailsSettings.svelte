<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { AccountAPI } from '$lib/api-client/account';
  import { Panel } from '$lib/components/admin';
  import { m } from '$lib/i18n/messages';
  import { Dialog, Hint } from '$lib/ui';
  import { Button, Form, TextArea, TextInput } from '$lib/ui/form';
  import {
    formatCooldownRemaining,
    getLoginChangeCooldownRemaining,
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin
  } from '$lib/validation';

  // The server route keys its subtree by server. Seed the local edit buffers
  // once so profile updates elsewhere cannot overwrite an in-progress edit.
  const serverScope = useServerScope();
  const currentUser = serverScope.store.currentUser;

  let { getAccountAPI }: { getAccountAPI: () => AccountAPI } = $props();

  // Keep in sync with the server-side bio length cap.
  const MAX_BIO_LENGTH = 1000;

  let displayName = $state(currentUser.user?.displayName ?? '');
  let login = $state(currentUser.user?.login ?? '');
  let bio = $state(currentUser.user?.bio ?? '');
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
  const displayNameModified = $derived(displayName !== currentUser.user?.displayName);
  const loginModified = $derived(login !== currentUser.user?.login);
  const bioModified = $derived((bio || '') !== (currentUser.user?.bio ?? ''));
  const isModified = $derived(displayNameModified || loginModified || bioModified);
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const canChangeLogin = $derived(cooldownRemaining === 0);

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
      const trimmed = bio.trim();
      if ([...trimmed].length > MAX_BIO_LENGTH) {
        error = m('settings.profile.bio.too_long', { max: MAX_BIO_LENGTH });
        return;
      }
      normalizedBio = trimmed;
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
    isSaving = true;
    error = '';
    successMessage = '';

    try {
      const updated = await getAccountAPI().updateProfile({
        displayName: normalizedDisplayName,
        login: normalizedLogin,
        bio: normalizedBio
      });

      if (currentUser.user) {
        const lastLoginChange = normalizedLogin
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

      displayName = updated.displayName;
      login = updated.login;
      bio = updated.bio ?? '';

      if (normalizedLogin) {
        localLastLoginChange = new Date();
      }

      successMessage = m('settings.profile.saved');
    } catch (saveError) {
      error = saveError instanceof Error ? saveError.message : m('settings.profile.save_failed');
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

    <TextArea
      id="settings-bio"
      label={m('settings.profile.bio.label')}
      description={m('settings.profile.bio.description', { max: MAX_BIO_LENGTH })}
      bind:value={bio}
      placeholder={m('settings.profile.bio.placeholder')}
      rows={4}
      maxlength={MAX_BIO_LENGTH}
      disabled={isSaving}
      testid="settings-bio"
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

<Dialog
  bind:visible={showLoginConfirm}
  title={m('settings.profile.username.confirm_title')}
  size="sm"
>
  <p class="mb-2">
    {m('settings.profile.username.confirm_prompt', { login: pendingLogin ?? '' })}
  </p>
  <p class="mb-4 text-muted">{m('settings.profile.username.confirm_cooldown')}</p>

  <div class="flex items-center gap-3">
    <Button defaultAction onclick={confirmLoginChange}>
      <span class="iconify icon-[uil--check]"></span>
      {m('settings.profile.username.confirm_button')}
    </Button>
    <Button variant="ghost" onclick={() => (showLoginConfirm = false)}>
      <span class="iconify icon-[uil--times]"></span>
      {m('common.cancel')}
    </Button>
  </div>
</Dialog>
