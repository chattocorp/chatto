<!--
@component

Edits a bot's public profile: display name, username, and bio. The parent owns
the save operation and must key this component by bot so the edit buffers are
seeded once per bot. Only changed fields are sent. A failed save keeps the
draft.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import type { UpdateUserProfileInput, UserSummary } from '$lib/api-client/users';
  import UserBioEditor from '$lib/components/users/UserBioEditor.svelte';
  import { profileSaveErrorMessage } from '$lib/components/users/profileSaveError';
  import { m } from '$lib/i18n/messages';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import {
    MAX_BIO_LENGTH,
    validateAndNormalizeBio,
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin
  } from '$lib/validation';

  let {
    bot,
    onsave
  }: {
    bot: { login: string; displayName: string; bio: string | null };
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

  const displayNameModified = $derived(displayName !== baseline.displayName);
  const loginModified = $derived(login !== baseline.login);
  const bioModified = $derived(bio !== baseline.bio);
  const modified = $derived(displayNameModified || loginModified || bioModified);

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

    saving = true;
    try {
      const updated = await onsave(input);
      if (!updated) return;
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
      disabled={saving}
      testid="bot-profile-login"
      oninput={() => (error = null)}
    />
    <UserBioEditor
      bind:value={bio}
      editorKind={userPreferences.composerEditor}
      maxlength={MAX_BIO_LENGTH}
      disabled={saving}
      oninput={() => (error = null)}
    />
    {#snippet footer()}
      <Button type="submit" disabled={!modified || saving} loading={saving}>
        <span class="iconify icon-[uil--check]"></span>
        {m('settings.profile.save_button')}
      </Button>
    {/snippet}
  </Form>
</Panel>
