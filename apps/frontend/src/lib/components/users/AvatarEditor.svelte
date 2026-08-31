<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { m } from '$lib/i18n/messages';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  type AvatarEditorUser = {
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string | null;
    deleted?: boolean;
    isBot?: boolean;
  };

  let {
    user,
    onupload,
    ondelete
  }: {
    user: AvatarEditorUser;
    onupload: (file: File) => Promise<boolean>;
    ondelete: () => Promise<boolean>;
  } = $props();

  let uploading = $state(false);
  let deleting = $state(false);
  let fileInput = $state<HTMLInputElement>();
  let isDragging = $state(false);
  const avatarUser = $derived({
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl ?? null,
    deleted: user.deleted ?? false,
    isBot: user.isBot ?? false,
    presenceStatus: PresenceStatus.OFFLINE
  });

  async function uploadFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(m('settings.profile.avatar.invalid_type'));
      return;
    }
    if (file.size > 10 * 1024 * 1024) {
      toast.error(m('settings.profile.avatar.too_large'));
      return;
    }

    uploading = true;
    try {
      if (await onupload(file)) toast.success(m('settings.profile.avatar.uploaded'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : m('settings.profile.avatar.upload_failed')
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
    if (!user.avatarUrl) return;
    deleting = true;
    try {
      if (await ondelete()) toast.success(m('settings.profile.avatar.removed'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : m('settings.profile.avatar.delete_failed')
      );
    } finally {
      deleting = false;
    }
  }
</script>

<Panel title={m('settings.profile.avatar.title')} icon="iconify icon-[uil--image]">
  <div
    class="relative flex max-w-md items-start gap-6"
    data-testid="avatar-drop-zone"
    role="group"
    aria-label={`${m('settings.profile.avatar.title')}: ${user.displayName}`}
    {@attach avatarDropZone}
  >
    <DropZoneOverlay
      visible={isDragging}
      title={m('settings.profile.avatar.drop_title')}
      subtitle={m('settings.profile.avatar.drop_subtitle')}
    />

    <UserAvatar user={avatarUser} size="xl" useLiveProfile={false} class="shadow-md" />

    <div class="flex flex-col gap-3">
      <p class="text-sm text-muted">{m('settings.profile.avatar.description')}</p>
      <div class="flex gap-2">
        <input
          type="file"
          accept="image/*"
          class="hidden"
          bind:this={fileInput}
          onchange={handleUpload}
        />
        <Button
          onclick={() => fileInput?.click()}
          label={`${user.avatarUrl ? m('settings.profile.avatar.change') : m('settings.profile.avatar.upload')}: ${user.displayName}`}
          loading={uploading}
          loadingText={m('settings.profile.avatar.uploading')}
        >
          <span class="inline-flex items-center gap-2">
            <span class="iconify icon-[uil--image-upload]" aria-hidden="true"></span>
            {user.avatarUrl
              ? m('settings.profile.avatar.change')
              : m('settings.profile.avatar.upload')}
          </span>
        </Button>
        {#if user.avatarUrl}
          <Button
            variant="danger-secondary"
            onclick={deleteAvatar}
            label={`${m('settings.profile.avatar.remove')}: ${user.displayName}`}
            loading={deleting}
            loadingText={m('settings.profile.avatar.removing')}
          >
            <span class="inline-flex items-center gap-2">
              <span class="iconify icon-[uil--trash-alt]" aria-hidden="true"></span>
              {m('settings.profile.avatar.remove')}
            </span>
          </Button>
        {/if}
      </div>
    </div>
  </div>
</Panel>
