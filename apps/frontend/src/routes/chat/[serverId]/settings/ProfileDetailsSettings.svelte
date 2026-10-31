<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    UsernameChangeState,
    canChangeUsername,
    legacyUsernameChangeAvailability
  } from '@chatto/client/api/actionAvailability';
  import { usernameChangeUnavailableMessage } from '$lib/actionAvailability';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { Code, ConnectError } from '@connectrpc/connect';
  import type { UpdateUserProfileInput, UserAPI } from '@chatto/client/api/users';
  import UserBioEditor from '$lib/components/users/UserBioEditor.svelte';
  import { profileSaveErrorMessage } from '$lib/components/users/profileSaveError';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { Panel, ConfirmDialog, Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import {
    MAX_BIO_LENGTH,
    startsLoginCooldown,
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
  // The validated patch awaiting username-change confirmation.
  let pendingUpdate = $state<UpdateUserProfileInput | null>(null);
  let now = $state(Date.now());

  const viewerLastLoginChange = $derived(
    currentUser.user?.lastLoginChange ? new Date(currentUser.user.lastLoginChange) : null
  );
  const lastLoginChange = $derived(localLastLoginChange ?? viewerLastLoginChange);
  const displayNameModified = $derived(displayName !== baseline.displayName);
  const loginModified = $derived(login !== baseline.login);
  const bioModified = $derived(bio !== baseline.bio);
  const isModified = $derived(displayNameModified || loginModified || bioModified);
  const canBypassLoginCooldown = $derived(serverScope.store.permissions.canAdminManageAccounts);
  const usernameAvailability = $derived(
    currentUser.user?.actions?.changeUsername ??
      legacyUsernameChangeAvailability(lastLoginChange?.toISOString(), canBypassLoginCooldown)
  );
  const canChangeLogin = $derived(canChangeUsername(usernameAvailability, now));
  const waitingForUsername = $derived(
    usernameAvailability.state === UsernameChangeState.ALLOWED &&
      usernameAvailability.availableAt !== undefined &&
      Date.parse(usernameAvailability.availableAt) > now
  );

  function clearMessages() {
    error = '';
    successMessage = '';
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();

    const input: UpdateUserProfileInput = {};
    if (displayNameModified) {
      const validation = validateAndNormalizeDisplayName(displayName);
      if (!validation.valid) {
        error = validation.error ?? m('settings.profile.display_name.invalid');
        return;
      }
      input.displayName = validation.normalized;
    }

    if (loginModified) {
      if (!canChangeUsername(usernameAvailability)) {
        error = usernameChangeUnavailableMessage(usernameAvailability);
        return;
      }
      const validation = validateAndNormalizeLogin(login);
      if (!validation.valid) {
        error = validation.error ?? m('settings.profile.username.invalid');
        return;
      }
      input.login = validation.normalized;
    }

    if (bioModified) {
      const validation = validateAndNormalizeBio(bio);
      if (!validation.valid) {
        error = validation.error ?? m('settings.profile.save_failed');
        return;
      }
      input.bio = validation.normalized;
    }

    if (!input.displayName && !input.login && input.bio === undefined) return;

    if (input.login) {
      pendingUpdate = input;
      return;
    }

    await saveProfile(input);
  }

  async function confirmLoginChange() {
    if (!pendingUpdate) return;
    const input = pendingUpdate;
    pendingUpdate = null;
    await saveProfile(input);
  }

  async function saveProfile(input: UpdateUserProfileInput) {
    const userId = serverScope.store.accountId;
    if (!userId) return;
    // Case-only renames and account-manager renames do not start the cooldown.
    const startedCooldown =
      !!input.login && !canBypassLoginCooldown && startsLoginCooldown(baseline.login, input.login);
    isSaving = true;
    error = '';
    successMessage = '';

    try {
      const refreshActions = !!input.login && !!currentUser.user?.actions;
      const updated = await getUserAPI().updateUserProfile(userId, input);

      currentUser.update(userId, (user) => ({
        displayName: updated.displayName,
        login: updated.login,
        bio: updated.bio ?? '',
        lastLoginChange: startedCooldown ? new Date().toISOString() : user.lastLoginChange,
        actions: input.login ? undefined : user.actions
      }));

      baseline = {
        displayName: updated.displayName,
        login: updated.login,
        bio: updated.bio ?? ''
      };
      displayName = baseline.displayName;
      login = baseline.login;
      bio = baseline.bio;

      if (startedCooldown) localLastLoginChange = new Date();

      if (refreshActions) await currentUser.load();

      successMessage = m('settings.profile.saved');
    } catch (saveError) {
      if (
        saveError instanceof ConnectError &&
        (saveError.code === Code.FailedPrecondition || saveError.code === Code.PermissionDenied)
      ) {
        await currentUser.load();
      }
      error = profileSaveErrorMessage(saveError, m('settings.profile.save_failed'));
    } finally {
      isSaving = false;
    }
  }
</script>

{#if waitingForUsername}
  <Interval milliseconds={1000} ontick={() => (now = Date.now())} />
{/if}

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

    {#if !canChangeLogin}
      <Hint>{usernameChangeUnavailableMessage(usernameAvailability, now)}</Hint>
    {/if}

    <UserBioEditor
      bind:value={bio}
      editorKind={userPreferences.composerEditor}
      maxlength={MAX_BIO_LENGTH}
      disabled={isSaving}
      oninput={clearMessages}
    />

    {#if successMessage}
      <Hint tone="success">{successMessage}</Hint>
    {/if}

    {#snippet footer()}
      <Button type="submit" disabled={!isModified || isSaving} loading={isSaving}>
        <span aria-hidden="true" class="iconify icon-[uil--check]"></span>
        {m('settings.profile.save_button')}
      </Button>
    {/snippet}
  </Form>
</Panel>

{#if pendingUpdate}
  <ConfirmDialog
    title={m('settings.profile.username.confirm_title')}
    tone="info"
    actionLabel={m('settings.profile.username.confirm_button')}
    actionIcon="iconify icon-[uil--check]"
    onconfirm={confirmLoginChange}
    onclose={() => (pendingUpdate = null)}
  >
    <p>
      {m('settings.profile.username.confirm_prompt', { login: pendingUpdate.login ?? '' })}
    </p>
    {#if !canBypassLoginCooldown}
      <p class="mt-3">{m('settings.profile.username.confirm_cooldown')}</p>
    {/if}
  </ConfirmDialog>
{/if}
