<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { PaneHeader, FormSection, Dialog, Hint } from '$lib/ui';
  import { TextInput, Button, Form } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import {
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin,
    getLoginChangeCooldownRemaining,
    formatCooldownRemaining
  } from '$lib/validation';
  import { getAvatarInitials } from '$lib/utils/initials';
  import * as m from '$lib/i18n/messages';

  // Capture the active server's CurrentUserState at init. The settings
  // page is scoped to one server (it lives under `[serverId]/settings`),
  // so we don't need the registry lookup to re-resolve reactively — and
  // the captured CurrentUserState is itself a reactive class (`user` /
  // `loading` are `$state`), so subsequent profile updates flow through.
  // The connection getter resolves to the active server's GraphQL client,
  // so profile/avatar mutations land on the right backend.
  const currentUser = serverRegistry.getStore(getActiveServer()).currentUser;
  const connection = useConnection();

  // Form state seeded once from the user's current profile. After init
  // these are local edit buffers; profile updates from elsewhere
  // (`currentUser.user = ...` after a mutation, cross-tab sync, etc.)
  // intentionally don't re-sync into them.
  let displayName = $state(currentUser.user?.displayName ?? '');
  let login = $state(currentUser.user?.login ?? '');
  let avatarUrl = $state<string | null>(currentUser.user?.avatarUrl ?? null);

  let isSaving = $state(false);
  let error = $state('');
  let successMessage = $state('');

  // Avatar state
  let uploadingAvatar = $state(false);
  let deletingAvatar = $state(false);
  let avatarFileInput = $state<HTMLInputElement>();
  let isDraggingAvatar = $state(false);

  // Cooldown state
  let lastLoginChange = $state<Date | null>(null);
  let cooldownLoaded = $state(false);

  // Confirmation dialog state
  let showLoginConfirm = $state(false);
  let pendingDisplayName = $state<string | undefined>(undefined);
  let pendingLogin = $state<string | undefined>(undefined);

  // Compute initials for avatar fallback
  const initials = $derived(
    getAvatarInitials(currentUser.user?.displayName, currentUser.user?.login)
  );

  // Track if the form has been modified
  const displayNameModified = $derived(displayName !== currentUser.user?.displayName);
  const loginModified = $derived(login !== currentUser.user?.login);
  const isModified = $derived(displayNameModified || loginModified);

  // Cooldown
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const canChangeLogin = $derived(cooldownRemaining === 0);

  // Fetch last login change on mount
  $effect(() => {
    connection()
      .client.query(
        graphql(`
          query GetMyLastLoginChange {
            viewer {
              user {
                id
                lastLoginChange
              }
            }
          }
        `),
        {}
      )
      .toPromise()
      .then((result) => {
        const last = result.data?.viewer?.user.lastLoginChange;
        if (last) {
          lastLoginChange = new Date(last);
        }
        cooldownLoaded = true;
      });
  });

  // Clear messages when the user modifies inputs
  $effect(() => {
    if (displayNameModified || loginModified) {
      error = '';
      successMessage = '';
    }
  });

  async function uploadAvatarFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(m['settings.profile.avatar.invalid_type']());
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error(m['settings.profile.avatar.too_large']());
      return;
    }

    uploadingAvatar = true;

    try {
      const result = await connection()
        .client.mutation(
          graphql(`
            mutation UploadAvatar($input: UploadAvatarInput!) {
              uploadAvatar(input: $input) {
                id
                avatarUrl
              }
            }
          `),
          { input: { userId: currentUser.user!.id, file } }
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      avatarUrl = result.data?.uploadAvatar.avatarUrl ?? null;

      // Update the current user state
      if (currentUser.user && result.data?.uploadAvatar) {
        currentUser.user = {
          ...currentUser.user,
          avatarUrl: result.data.uploadAvatar.avatarUrl
        };
      }

      toast.success(m['settings.profile.avatar.uploaded']());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m['settings.profile.avatar.upload_failed']());
    } finally {
      uploadingAvatar = false;
      if (avatarFileInput) avatarFileInput.value = '';
    }
  }

  function handleAvatarUpload(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (file) uploadAvatarFile(file);
  }

  const avatarDropZone = dropZone({
    onDrop: (files) => uploadAvatarFile(files[0]),
    onDragStateChange: (dragging) => (isDraggingAvatar = dragging),
    acceptedTypes: ['image/*']
  });

  async function handleAvatarDelete() {
    if (!avatarUrl) return;

    deletingAvatar = true;

    try {
      const result = await connection()
        .client.mutation(
          graphql(`
            mutation DeleteAvatar($input: DeleteAvatarInput!) {
              deleteAvatar(input: $input) {
                id
                avatarUrl
              }
            }
          `),
          { input: { userId: currentUser.user!.id } }
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      avatarUrl = null;

      // Update the current user state
      if (currentUser.user) {
        currentUser.user = {
          ...currentUser.user,
          avatarUrl: null
        };
      }

      toast.success(m['settings.profile.avatar.removed']());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m['settings.profile.avatar.delete_failed']());
    } finally {
      deletingAvatar = false;
    }
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();

    // Validate display name if changed
    let normalizedDisplayName: string | undefined;
    if (displayNameModified) {
      const validation = validateAndNormalizeDisplayName(displayName);
      if (!validation.valid) {
        error = validation.error ?? m['settings.profile.display_name.invalid']();
        return;
      }
      normalizedDisplayName = validation.normalized!;
    }

    // Validate login if changed
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
      normalizedLogin = validation.normalized!;
    }

    if (!normalizedDisplayName && !normalizedLogin) {
      return;
    }

    // If login is being changed, show confirmation dialog
    if (normalizedLogin) {
      pendingDisplayName = normalizedDisplayName;
      pendingLogin = normalizedLogin;
      showLoginConfirm = true;
      return;
    }

    // No login change — save directly
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
      const result = await connection()
        .client.mutation(
          graphql(`
            mutation UpdateProfile($input: UpdateProfileInput!) {
              updateProfile(input: $input) {
                id
                displayName
                login
              }
            }
          `),
          {
            input: {
              userId: currentUser.user!.id,
              displayName: normalizedDisplayName ?? null,
              login: normalizedLogin ?? null
            }
          }
        )
        .toPromise();

      if (result.error) {
        error = result.error.message;
        return;
      }

      // Update the current user state
      if (currentUser.user && result.data?.updateProfile) {
        currentUser.user = {
          ...currentUser.user,
          displayName: result.data.updateProfile.displayName,
          login: result.data.updateProfile.login
        };
      }

      // Update local state to match
      displayName = result.data?.updateProfile.displayName ?? displayName;
      login = result.data?.updateProfile.login ?? login;

      // Update cooldown if login was changed
      if (normalizedLogin) {
        lastLoginChange = new Date();
      }

      successMessage = m['settings.profile.saved']();
    } catch (err) {
      error = err instanceof Error ? err.message : m['settings.profile.save_failed']();
    } finally {
      isSaving = false;
    }
  }
</script>

<PaneHeader
  title={m['settings.profile.title']()}
  subtitle={m['settings.profile.subtitle']()}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Avatar Section -->
  <FormSection title={m['settings.profile.avatar.title']()} maxWidth="max-w-md">
    <div
      class="relative flex items-start gap-6"
      data-testid="avatar-drop-zone"
      {@attach avatarDropZone}
    >
      <DropZoneOverlay
        visible={isDraggingAvatar}
        title={m['settings.profile.avatar.drop_title']()}
        subtitle={m['settings.profile.avatar.drop_subtitle']()}
      />
      <!-- Avatar Preview -->
      <div
        class="flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-200 text-4xl font-black text-muted shadow-md"
      >
        {#if avatarUrl}
          <img
            src={avatarUrl}
            alt={m['settings.profile.avatar.alt']()}
            class="h-full w-full object-cover"
          />
        {:else}
          {initials}
        {/if}
      </div>

      <!-- Upload Controls -->
      <div class="flex flex-col gap-3">
        <p class="text-sm text-muted">
          {m['settings.profile.avatar.description']()}
        </p>
        <div class="flex gap-2">
          <input
            type="file"
            accept="image/*"
            class="hidden"
            bind:this={avatarFileInput}
            onchange={handleAvatarUpload}
          />
          <Button
            variant="secondary"
            onclick={() => avatarFileInput?.click()}
            loading={uploadingAvatar}
            loadingText={m['settings.profile.avatar.uploading']()}
          >
            <span class="inline-flex items-center gap-2">
              <span class="iconify uil--image-upload"></span>
              {avatarUrl
                ? m['settings.profile.avatar.change']()
                : m['settings.profile.avatar.upload']()}
            </span>
          </Button>
          {#if avatarUrl}
            <Button
              variant="ghost"
              onclick={handleAvatarDelete}
              loading={deletingAvatar}
              loadingText={m['settings.profile.avatar.removing']()}
            >
              <span class="inline-flex items-center gap-2 text-error">
                <span class="iconify uil--trash-alt"></span>
                {m['settings.profile.avatar.remove']()}
              </span>
            </Button>
          {/if}
        </div>
      </div>
    </div>
  </FormSection>

  <!-- Profile Form -->
  <Form onsubmit={handleSubmit} maxWidth="max-w-md" bordered {error}>
    <TextInput
      label={m['settings.profile.display_name.label']()}
      bind:value={displayName}
      placeholder={m['settings.profile.display_name.placeholder']()}
      disabled={isSaving}
    />

    <TextInput
      label={m['settings.profile.username.label']()}
      bind:value={login}
      placeholder={m['settings.profile.username.placeholder']()}
      disabled={isSaving || !canChangeLogin}
      testid="settings-username"
    />

    {#if cooldownLoaded && !canChangeLogin}
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
        <span class="iconify uil--check"></span>
        {m['settings.profile.save_button']()}
      </Button>
    {/snippet}
  </Form>
</div>

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
      <span class="iconify uil--check"></span>
      {m['settings.profile.username.confirm_button']()}
    </Button>
    <Button variant="ghost" onclick={() => (showLoginConfirm = false)}>
      <span class="iconify uil--times"></span>
      {m['common.cancel']()}
    </Button>
  </div>
</Dialog>
