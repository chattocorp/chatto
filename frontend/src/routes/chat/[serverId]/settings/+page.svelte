<script lang="ts">
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { graphql } from '$lib/gql';
  import { PaneHeader, FormSection, Dialog } from '$lib/ui';
  import { TextInput, Button, FormError } from '$lib/ui/form';
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

  const currentUser = getCurrentUser();

  // Form state
  let displayName = $state(currentUser.user?.displayName ?? '');
  let login = $state(currentUser.user?.login ?? '');
  let isSaving = $state(false);
  let error = $state('');
  let successMessage = $state('');

  // Avatar state
  let avatarUrl = $state<string | null>(currentUser.user?.avatarUrl ?? null);
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
    graphqlClientManager.originClient.client
      .query(
        graphql(`
          query GetMyLastLoginChange {
            me {
              id
              lastLoginChange
            }
          }
        `),
        {}
      )
      .toPromise()
      .then((result) => {
        if (result.data?.me?.lastLoginChange) {
          lastLoginChange = new Date(result.data.me.lastLoginChange);
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
      toast.error('Please select an image file');
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error('Image must be less than 10MB');
      return;
    }

    uploadingAvatar = true;

    try {
      const result = await graphqlClientManager.originClient.client
        .mutation(
          graphql(`
            mutation UploadMyAvatar($input: UploadMyAvatarInput!) {
              uploadMyAvatar(input: $input) {
                id
                avatarUrl
              }
            }
          `),
          { input: { file } }
        )
        .toPromise();

      if (result.error) {
        throw new Error(result.error.message);
      }

      avatarUrl = result.data?.uploadMyAvatar.avatarUrl ?? null;

      // Update the current user state
      if (currentUser.user && result.data?.uploadMyAvatar) {
        currentUser.user = {
          ...currentUser.user,
          avatarUrl: result.data.uploadMyAvatar.avatarUrl
        };
      }

      toast.success('Avatar uploaded successfully');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to upload avatar');
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
      const result = await graphqlClientManager.originClient.client
        .mutation(
          graphql(`
            mutation DeleteMyAvatar {
              deleteMyAvatar {
                id
                avatarUrl
              }
            }
          `),
          {}
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

      toast.success('Avatar removed');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to delete avatar');
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
        error = validation.error ?? 'Invalid display name';
        return;
      }
      normalizedDisplayName = validation.normalized!;
    }

    // Validate login if changed
    let normalizedLogin: string | undefined;
    if (loginModified) {
      if (!canChangeLogin) {
        error = `You can only change your username once every 30 days. Try again in ${formatCooldownRemaining(cooldownRemaining)}.`;
        return;
      }
      const validation = validateAndNormalizeLogin(login);
      if (!validation.valid) {
        error = validation.error ?? 'Invalid username';
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
      const result = await graphqlClientManager.originClient.client
        .mutation(
          graphql(`
            mutation UpdateMyProfile($input: UpdateMyProfileInput!) {
              updateMyProfile(input: $input) {
                id
                displayName
                login
              }
            }
          `),
          {
            input: {
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
      if (currentUser.user && result.data?.updateMyProfile) {
        currentUser.user = {
          ...currentUser.user,
          displayName: result.data.updateMyProfile.displayName,
          login: result.data.updateMyProfile.login
        };
      }

      // Update local state to match
      displayName = result.data?.updateMyProfile.displayName ?? displayName;
      login = result.data?.updateMyProfile.login ?? login;

      // Update cooldown if login was changed
      if (normalizedLogin) {
        lastLoginChange = new Date();
      }

      successMessage = 'Profile updated successfully';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save profile';
    } finally {
      isSaving = false;
    }
  }
</script>

<PaneHeader title="Profile" subtitle="Manage your profile information" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <!-- Avatar Section -->
  <FormSection title="Avatar" maxWidth="max-w-md">
    <div class="relative flex items-start gap-6" data-testid="avatar-drop-zone" {@attach avatarDropZone}>
      <DropZoneOverlay visible={isDraggingAvatar} title="Drop image" subtitle="Upload as your avatar" />
      <!-- Avatar Preview -->
      <div
        class="flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-200 text-4xl font-black text-muted shadow-md"
      >
        {#if avatarUrl}
          <img src={avatarUrl} alt="Your avatar" class="h-full w-full object-cover" />
        {:else}
          {initials}
        {/if}
      </div>

      <!-- Upload Controls -->
      <div class="flex flex-col gap-3">
        <p class="text-sm text-muted">
          Upload an avatar. Images will be resized to 256x256 pixels.
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
            loadingText="Uploading..."
          >
            <span class="inline-flex items-center gap-2">
              <span class="iconify uil--image-upload"></span>
              {avatarUrl ? 'Change Avatar' : 'Upload Avatar'}
            </span>
          </Button>
          {#if avatarUrl}
            <Button
              variant="ghost"
              onclick={handleAvatarDelete}
              loading={deletingAvatar}
              loadingText="Removing..."
            >
              <span class="inline-flex items-center gap-2 text-error">
                <span class="iconify uil--trash-alt"></span>
                Remove
              </span>
            </Button>
          {/if}
        </div>
      </div>
    </div>
  </FormSection>

  <!-- Profile Form -->
  <form onsubmit={handleSubmit} class="flex max-w-md flex-col gap-4 border-t border-border pt-6">
    <TextInput
      label="Display Name"
      bind:value={displayName}
      placeholder="Enter your display name"
      disabled={isSaving}
    />

    <TextInput
      label="Username"
      bind:value={login}
      placeholder="Enter your username"
      disabled={isSaving || !canChangeLogin}
      testid="settings-username"
    />

    {#if cooldownLoaded && !canChangeLogin}
      <p class="text-sm text-muted">
        You can change your username again in {formatCooldownRemaining(cooldownRemaining)}.
      </p>
    {/if}

    {#if error}
      <FormError {error} />
    {/if}

    {#if successMessage}
      <div
        class="rounded-lg border border-green-500/20 bg-green-500/10 p-3 text-sm text-green-600 dark:text-green-400"
      >
        {successMessage}
      </div>
    {/if}

    <div class="flex gap-2">
      <Button type="submit" disabled={!isModified || isSaving} loading={isSaving}>
        <span class="iconify uil--check"></span>
        Save Changes
      </Button>
    </div>
  </form>
</div>

<Dialog bind:visible={showLoginConfirm} title="Change Username" size="sm">
  <p class="mb-2">
    Are you sure you want to change your username to <strong>@{pendingLogin}</strong>?
  </p>
  <p class="mb-4 text-sm text-muted">You can only change your username once every 30 days.</p>

  <div class="flex items-center gap-3">
    <Button onclick={confirmLoginChange}>
      <span class="iconify uil--check"></span>
      Change Username
    </Button>
    <Button variant="ghost" onclick={() => (showLoginConfirm = false)}>
      <span class="iconify uil--times"></span>
      Cancel
    </Button>
  </div>
</Dialog>
