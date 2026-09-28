<!--
@component

Edits a bot's public profile: display name, username, and bio. The parent owns
the save operation and must key this component by bot so the edit buffers are
seeded once per bot. Only changed fields are sent. A failed save keeps the
draft. A username change starts the bot's 30-day username cooldown unless the
viewer can bypass it, so the form asks for confirmation first and locks the
username field while the cooldown runs.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import type { UpdateUserProfileInput, UserSummary } from '$lib/api-client/users';
  import UserBioEditor from '$lib/components/users/UserBioEditor.svelte';
  import { profileSaveErrorMessage } from '$lib/components/users/profileSaveError';
  import { m } from '$lib/i18n/messages';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { Panel, ConfirmDialog } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import {
    formatCooldownRemaining,
    getLoginChangeCooldownRemaining,
    MAX_BIO_LENGTH,
    startsLoginCooldown,
    validateAndNormalizeBio,
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin
  } from '$lib/validation';

  let {
    bot,
    canBypassLoginCooldown,
    onsave
  }: {
    bot: { login: string; displayName: string; bio: string | null; lastLoginChange: Date | null };
    /** True when the viewer has user.manage-accounts and ignores the cooldown. */
    canBypassLoginCooldown: boolean;
    /** Saves the changed fields. Resolves to null when the result is stale. */
    onsave: (input: UpdateUserProfileInput) => Promise<UserSummary | null>;
  } = $props();

  // Edit buffers, not mirrors: realtime profile updates must not overwrite an
  // in-progress edit. Dirty checks compare against the values the form was
  // seeded with, so a concurrent change to an untouched field is not sent back
  // with its stale value.
  const seed = untrack(() => ({
    displayName: bot.displayName,
    login: bot.login,
    bio: bot.bio ?? ''
  }));
  let baseline = $state(seed);
  let displayName = $state(seed.displayName);
  let login = $state(seed.login);
  let bio = $state(seed.bio);
  let saving = $state(false);
  let error = $state<string | null>(null);
  let localLastLoginChange = $state<Date | null>(null);
  let pendingInput = $state<UpdateUserProfileInput | null>(null);

  const displayNameModified = $derived(displayName !== baseline.displayName);
  const loginModified = $derived(login !== baseline.login);
  const bioModified = $derived(bio !== baseline.bio);
  const modified = $derived(displayNameModified || loginModified || bioModified);
  const cooldownRemaining = $derived(
    getLoginChangeCooldownRemaining(localLastLoginChange ?? bot.lastLoginChange)
  );
  const canChangeLogin = $derived(canBypassLoginCooldown || cooldownRemaining === 0);

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!modified || saving) return;
    error = null;
    const input: UpdateUserProfileInput = {};

    if (displayNameModified) {
      const result = validateAndNormalizeDisplayName(displayName);
      if (!result.valid || result.normalized === undefined) {
        error = result.error ?? m('settings.profile.display_name.invalid');
        return;
      }
      input.displayName = result.normalized;
    }
    if (loginModified) {
      const result = validateAndNormalizeLogin(login);
      if (!result.valid || result.normalized === undefined) {
        error = result.error ?? m('settings.profile.username.invalid');
        return;
      }
      if (!canChangeLogin && startsLoginCooldown(baseline.login, result.normalized)) {
        error = m('settings.bots.username_cooldown_notice', {
          remaining: formatCooldownRemaining(cooldownRemaining)
        });
        return;
      }
      input.login = result.normalized;
    }
    if (bioModified) {
      const result = validateAndNormalizeBio(bio);
      if (!result.valid || result.normalized === undefined) {
        error = result.error ?? m('settings.bots.profile_save_failed');
        return;
      }
      input.bio = result.normalized;
    }

    if (startsCooldown(input)) {
      pendingInput = input;
      return;
    }
    await submit(input);
  }

  /** True when saving `input` starts the bot's username cooldown for this viewer. */
  function startsCooldown(input: UpdateUserProfileInput): boolean {
    return (
      input.login !== undefined &&
      !canBypassLoginCooldown &&
      startsLoginCooldown(baseline.login, input.login)
    );
  }

  async function confirmLoginChange() {
    const input = pendingInput;
    pendingInput = null;
    if (input) await submit(input);
  }

  async function submit(input: UpdateUserProfileInput) {
    saving = true;
    try {
      const startedCooldown = startsCooldown(input);
      const updated = await onsave(input);
      if (!updated) return;
      if (startedCooldown) localLastLoginChange = new Date();
      baseline = { displayName: updated.displayName, login: updated.login, bio: updated.bio ?? '' };
      displayName = baseline.displayName;
      login = baseline.login;
      bio = baseline.bio;
      toast.success(m('settings.bots.profile_saved'));
    } catch (saveError) {
      error = profileSaveErrorMessage(saveError, m('settings.bots.profile_save_failed'));
    } finally {
      saving = false;
    }
  }
</script>

<Panel
  title={m('settings.bots.profile_title')}
  subtitle={m('settings.bots.profile_description')}
  icon="iconify icon-[uil--user]"
>
  <Form onsubmit={save} maxWidth="max-w-md" {error}>
    <TextInput
      id="bot-profile-display-name"
      label={m('settings.bots.display_name')}
      bind:value={displayName}
      disabled={saving}
      testid="bot-profile-display-name"
      oninput={() => (error = null)}
    />
    <TextInput
      id="bot-profile-login"
      label={m('settings.bots.username')}
      bind:value={login}
      disabled={saving || !canChangeLogin}
      testid="bot-profile-login"
      oninput={() => (error = null)}
    />
    {#if !canChangeLogin}
      <p class="text-sm text-muted" data-testid="bot-profile-login-cooldown">
        {m('settings.bots.username_cooldown_notice', {
          remaining: formatCooldownRemaining(cooldownRemaining)
        })}
      </p>
    {/if}
    <UserBioEditor
      bind:value={bio}
      editorKind={userPreferences.composerEditor}
      maxlength={MAX_BIO_LENGTH}
      placeholder={m('settings.bots.bio_placeholder')}
      description={m('settings.bots.bio_description', { max: MAX_BIO_LENGTH })}
      disabled={saving}
      oninput={() => (error = null)}
    />
    {#snippet footer()}
      <Button type="submit" disabled={!modified || saving} loading={saving}>
        <span class="iconify icon-[uil--check]" aria-hidden="true"></span>
        {m('settings.profile.save_button')}
      </Button>
    {/snippet}
  </Form>
</Panel>

<ConfirmDialog
  visible={pendingInput !== null}
  title={m('settings.bots.username_confirm_title')}
  tone="info"
  actionLabel={m('settings.profile.username.confirm_button')}
  actionIcon="iconify icon-[uil--check]"
  onconfirm={confirmLoginChange}
  onclose={() => (pendingInput = null)}
>
  <p>{m('settings.bots.username_confirm_prompt', { login: pendingInput?.login ?? '' })}</p>
  <p class="mt-3">{m('settings.bots.username_confirm_cooldown')}</p>
</ConfirmDialog>
