<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { AccountAPI } from '$lib/api-client/account';
  import * as m from '$lib/i18n/messages';
  import { Dialog, Hint } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';
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

  let displayName = $state(currentUser.user?.displayName ?? '');
  let login = $state(currentUser.user?.login ?? '');
  let isSaving = $state(false);
  let error = $state('');
  let successMessage = $state('');
  let localLastLoginChange = $state<Date | null>(null);
  let showLoginConfirm = $state(false);
  let pendingDisplayName = $state<string | undefined>(undefined);
  let pendingLogin = $state<string | undefined>(undefined);

  const viewerLastLoginChange = $derived(
    currentUser.user?.lastLoginChange ? new Date(currentUser.user.lastLoginChange) : null
  );
  const lastLoginChange = $derived(localLastLoginChange ?? viewerLastLoginChange);
  const displayNameModified = $derived(displayName !== currentUser.user?.displayName);
  const loginModified = $derived(login !== currentUser.user?.login);
  const isModified = $derived(displayNameModified || loginModified);
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
        error = validation.error ?? m['settings.profile.display_name.invalid']();
        return;
      }
      normalizedDisplayName = validation.normalized;
    }

    let normalizedLogin: string | undefined;
    if (loginModified) {
      if (!canChangeLogin) {
        error = m['settings.profile.username.cooldown_error']({
          remaining: formatCooldownRemaining(cooldownRemaining)
        });
        return;
      }
      const validation = validateAndNormalizeLogin(login);
      if (!validation.valid) {
        error = validation.error ?? m['settings.profile.username.invalid']();
        return;
      }
      normalizedLogin = validation.normalized;
    }

    if (!normalizedDisplayName && !normalizedLogin) return;

    if (normalizedLogin) {
      pendingDisplayName = normalizedDisplayName;
      pendingLogin = normalizedLogin;
      showLoginConfirm = true;
      return;
    }

    await saveProfile(normalizedDisplayName, undefined);
  }

  async function confirmLoginChange() {
    showLoginConfirm = false;
    await saveProfile(pendingDisplayName, pendingLogin);
    pendingDisplayName = undefined;
    pendingLogin = undefined;
  }

  async function saveProfile(
    normalizedDisplayName: string | undefined,
    normalizedLogin: string | undefined
  ) {
    isSaving = true;
    error = '';
    successMessage = '';

    try {
      const updated = await getAccountAPI().updateProfile({
        displayName: normalizedDisplayName,
        login: normalizedLogin
      });

      if (currentUser.user) {
        const lastLoginChange = normalizedLogin
          ? new Date().toISOString()
          : currentUser.user.lastLoginChange;
        currentUser.user = {
          ...currentUser.user,
          displayName: updated.displayName,
          login: updated.login,
          lastLoginChange
        };
      }

      displayName = updated.displayName;
      login = updated.login;

      if (normalizedLogin) {
        localLastLoginChange = new Date();
      }

      successMessage = m['settings.profile.saved']();
    } catch (saveError) {
      error = saveError instanceof Error ? saveError.message : m['settings.profile.save_failed']();
    } finally {
      isSaving = false;
    }
  }
</script>

<Form onsubmit={handleSubmit} maxWidth="max-w-md" bordered {error}>
  <TextInput
    label={m['settings.profile.display_name.label']()}
    bind:value={displayName}
    placeholder={m['settings.profile.display_name.placeholder']()}
    disabled={isSaving}
    oninput={clearMessages}
  />

  <TextInput
    label={m['settings.profile.username.label']()}
    bind:value={login}
    placeholder={m['settings.profile.username.placeholder']()}
    disabled={isSaving || !canChangeLogin}
    testid="settings-username"
    oninput={clearMessages}
  />

  {#if !canChangeLogin}
    <p class="text-sm text-muted">
      {m['settings.profile.username.cooldown_notice']({
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
      {m['settings.profile.save_button']()}
    </Button>
  {/snippet}
</Form>

<Dialog
  bind:visible={showLoginConfirm}
  title={m['settings.profile.username.confirm_title']()}
  size="sm"
>
  <p class="mb-2">
    {m['settings.profile.username.confirm_prompt']({ login: pendingLogin ?? '' })}
  </p>
  <p class="mb-4 text-muted">{m['settings.profile.username.confirm_cooldown']()}</p>

  <div class="flex items-center gap-3">
    <Button onclick={confirmLoginChange}>
      <span class="iconify icon-[uil--check]"></span>
      {m['settings.profile.username.confirm_button']()}
    </Button>
    <Button variant="ghost" onclick={() => (showLoginConfirm = false)}>
      <span class="iconify icon-[uil--times]"></span>
      {m['common.cancel']()}
    </Button>
  </div>
</Dialog>
