<script lang="ts">
  import type { PresenceStatus } from '$lib/render/types';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import { TextArea } from '$lib/ui/form';
  import * as m from '$lib/i18n/messages';

  type User = {
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string | null;
    presenceStatus: PresenceStatus;
  };

  type Room = {
    id: string;
    name: string;
  };

  let {
    user = null,
    userId,
    room = null,
    roomId,
    submitting = false,
    error = null,
    onconfirm,
    onclose
  }: {
    user?: User | null;
    userId: string;
    room?: Room | null;
    roomId: string;
    submitting?: boolean;
    error?: string | null;
    onconfirm?: (reason: string) => void;
    onclose?: () => void;
  } = $props();

  let visible = $state(true);
  let reason = $state('');

  const displayName = $derived(user?.displayName || user?.login || userId);
  const roomLabel = $derived(room ? `#${room.name}` : roomId);
  const disabled = $derived(reason.trim().length === 0 || submitting);

  function handleSubmit() {
    if (disabled) return;
    onconfirm?.(reason.trim());
  }
</script>

<FormDialog
  bind:visible
  title={m['admin.moderation.unban_title']({ user: displayName })}
  size="sm"
  submitLabel={m['admin.moderation.unban']()}
  submitTone="warning"
  submitIcon="iconify uil--unlock"
  submitLoadingText={m['admin.moderation.unbanning']()}
  loading={submitting}
  {disabled}
  {error}
  onsubmit={handleSubmit}
  onclose={() => onclose?.()}
>
  <div class="flex items-center gap-3 surface-box p-3">
    {#if user}
      <UserAvatar {user} size="md" />
    {:else}
      <div
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-surface-200 text-muted"
      >
        <span class="iconify text-lg uil--user"></span>
      </div>
    {/if}
    <div class="min-w-0 flex-1">
      <div class="truncate font-medium text-text">{displayName}</div>
      <div class="truncate text-sm text-muted">{roomLabel}</div>
    </div>
  </div>

  <TextArea
    id="unban-room-member-reason"
    label={m['admin.common.reason']()}
    bind:value={reason}
    rows={4}
    maxlength={1000}
    required
    disabled={submitting}
  />
</FormDialog>
