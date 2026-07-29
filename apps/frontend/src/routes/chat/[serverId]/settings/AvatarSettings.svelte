<script lang="ts">
  import type { AccountAPI } from '$lib/api-client/account';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import * as m from '$lib/i18n/messages';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { FormSection } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { getAvatarInitials } from '$lib/utils/initials';

  // This route is scoped to one server, so the current-user store is stable
  // for the component lifetime while its fields remain reactive.
  const currentUser = serverRegistry.getStore(getActiveServer()).currentUser;

  let { getAccountAPI }: { getAccountAPI: () => AccountAPI } = $props();

  let avatarUrl = $state<string | null>(currentUser.user?.avatarUrl ?? null);
  let uploading = $state(false);
  let deleting = $state(false);
  let fileInput = $state<HTMLInputElement>();
  let isDragging = $state(false);

  const initials = $derived(
    getAvatarInitials(currentUser.user?.displayName, currentUser.user?.login)
  );

  async function uploadFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(m['settings.profile.avatar.invalid_type']());
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      toast.error(m['settings.profile.avatar.too_large']());
      return;
    }

    uploading = true;

    try {
      const updated = await getAccountAPI().uploadAvatar(file);
      avatarUrl = updated.avatarUrl ?? null;

      if (currentUser.user) {
        currentUser.user = {
          ...currentUser.user,
          avatarUrl: updated.avatarUrl ?? null
        };
      }

      toast.success(m['settings.profile.avatar.uploaded']());
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : m['settings.profile.avatar.upload_failed']()
      );
    } finally {
      uploading = false;
      if (fileInput) fileInput.value = '';
    }
  }

  function handleUpload(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (file) void uploadFile(file);
  }

  const avatarDropZone = dropZone({
    onDrop: (files) => uploadFile(files[0]),
    onDragStateChange: (dragging) => (isDragging = dragging),
    acceptedTypes: ['image/*']
  });

  async function deleteAvatar() {
    if (!avatarUrl) return;

    deleting = true;

    try {
      const updated = await getAccountAPI().deleteAvatar();
      avatarUrl = updated.avatarUrl ?? null;

      if (currentUser.user) {
        currentUser.user = {
          ...currentUser.user,
          avatarUrl: updated.avatarUrl ?? null
        };
      }

      toast.success(m['settings.profile.avatar.removed']());
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : m['settings.profile.avatar.delete_failed']()
      );
    } finally {
      deleting = false;
    }
  }
</script>

<FormSection title={m['settings.profile.avatar.title']()} maxWidth="max-w-md">
  <div
    class="relative flex items-start gap-6"
    data-testid="avatar-drop-zone"
    {@attach avatarDropZone}
  >
    <DropZoneOverlay
      visible={isDragging}
      title={m['settings.profile.avatar.drop_title']()}
      subtitle={m['settings.profile.avatar.drop_subtitle']()}
    />

    <div
      class="flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-emphasized text-4xl font-black text-muted shadow-md"
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

    <div class="flex flex-col gap-3">
      <p class="text-sm text-muted">
        {m['settings.profile.avatar.description']()}
      </p>
      <div class="flex gap-2">
        <input
          type="file"
          accept="image/*"
          class="hidden"
          bind:this={fileInput}
          onchange={handleUpload}
        />
        <Button
          variant="secondary"
          onclick={() => fileInput?.click()}
          loading={uploading}
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
            onclick={deleteAvatar}
            loading={deleting}
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
