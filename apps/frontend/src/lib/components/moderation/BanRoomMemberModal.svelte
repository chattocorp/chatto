<script lang="ts">
  import type { PresenceStatus } from '$lib/render/types';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getLiveDisplayName, getLiveLogin } from '$lib/state/userProfiles.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import { ExpirySelect, TextArea } from '$lib/ui/form';
  import * as m from '$lib/i18n/messages';

  type User = {
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string | null;
    presenceStatus: PresenceStatus;
  };

  let {
    user,
    submitting = false,
    error = null,
    onconfirm,
    onclose
  }: {
    user: User;
    submitting?: boolean;
    error?: string | null;
    onconfirm?: (reason: string, expiresAt: string | null) => void;
    onclose?: () => void;
  } = $props();

  let visible = $state(true);
  let reason = $state('');
  let expiresAt = $state<string | null>(null);
  let expiryValid = $state(true);

  const displayName = $derived(getLiveDisplayName(user.id, user.displayName || user.login));
  const login = $derived(getLiveLogin(user.id, user.login));

  const disabled = $derived(reason.trim().length === 0 || submitting || !expiryValid);

  function handleSubmit() {
    if (disabled) return;
    onconfirm?.(reason.trim(), expiresAt);
  }
</script>

<FormDialog
  bind:visible
  title={m['admin.moderation.ban_title']({ user: displayName })}
  size="sm"
  submitLabel={m['admin.moderation.ban_action']()}
  submitTone="danger"
  submitIcon="iconify uil--ban"
  submitLoadingText={m['admin.moderation.banning']()}
  loading={submitting}
  {disabled}
  {error}
  onsubmit={handleSubmit}
  onclose={() => onclose?.()}
>
  <div class="flex items-center gap-3 surface-box p-3">
    <UserAvatar {user} size="md" />
    <div class="min-w-0 flex-1">
      <div class="truncate font-medium text-text">{displayName}</div>
      <div class="truncate text-sm text-muted">@{login}</div>
    </div>
  </div>

  <TextArea
    id="ban-room-member-reason"
    label={m['admin.common.reason']()}
    bind:value={reason}
    rows={4}
    maxlength={1000}
    required
    disabled={submitting}
  />

  <ExpirySelect
    id="ban-room-member-expires-at"
    label={m['admin.common.expires']()}
    bind:value={expiresAt}
    bind:valid={expiryValid}
    disabled={submitting}
  />
</FormDialog>
