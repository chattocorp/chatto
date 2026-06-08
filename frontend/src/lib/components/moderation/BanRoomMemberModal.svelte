<script lang="ts">
  import type { PresenceStatus } from '$lib/gql/graphql';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getLiveDisplayName, getLiveLogin } from '$lib/state/userProfiles.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import { FormField, TextArea } from '$lib/ui/form';

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
  let expiresAtLocal = $state('');

  const displayName = $derived(getLiveDisplayName(user.id, user.displayName || user.login));
  const login = $derived(getLiveLogin(user.id, user.login));

  const expiresAtError = $derived.by(() => {
    if (!expiresAtLocal) return null;
    const date = new Date(expiresAtLocal);
    if (Number.isNaN(date.getTime())) return 'Enter a valid expiry.';
    if (date <= new Date()) return 'Expiry must be in the future.';
    return null;
  });

  const disabled = $derived(reason.trim().length === 0 || submitting || !!expiresAtError);

  function handleSubmit() {
    if (disabled) return;
    const expiresAt = expiresAtLocal ? new Date(expiresAtLocal).toISOString() : null;
    onconfirm?.(reason.trim(), expiresAt);
  }
</script>

<FormDialog
  bind:visible
  title={`Ban ${displayName}`}
  size="sm"
  submitLabel="Ban from room"
  submitTone="danger"
  submitIcon="iconify uil--ban"
  submitLoadingText="Banning..."
  loading={submitting}
  {disabled}
  {error}
  onsubmit={handleSubmit}
  onclose={() => onclose?.()}
>
  <div class="flex items-center gap-3 rounded-md border border-border bg-surface-100 p-3">
    <UserAvatar {user} size="md" />
    <div class="min-w-0 flex-1">
      <div class="truncate font-medium text-text">{displayName}</div>
      <div class="truncate text-sm text-muted">@{login}</div>
    </div>
  </div>

  <TextArea
    id="ban-room-member-reason"
    label="Reason"
    bind:value={reason}
    rows={4}
    maxlength={1000}
    required
    disabled={submitting}
  />

  <FormField
    id="ban-room-member-expires-at"
    label="Expires at"
    error={expiresAtError ?? undefined}
    description="Blank means indefinite."
  >
    <input
      id="ban-room-member-expires-at"
      class="input"
      type="datetime-local"
      bind:value={expiresAtLocal}
      disabled={submitting}
      aria-invalid={expiresAtError ? 'true' : undefined}
      aria-describedby={expiresAtError
        ? 'ban-room-member-expires-at-error'
        : 'ban-room-member-expires-at-description'}
    />
  </FormField>
</FormDialog>
