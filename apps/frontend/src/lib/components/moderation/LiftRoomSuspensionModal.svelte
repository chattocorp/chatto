<script lang="ts">
  import { accountNameToken } from '$lib/render/accountName';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { FormDialog } from '$lib/ui';
  import { TextArea } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';

  type User = {
    id: string;
    login: string;
    displayName: string;
    isBot?: boolean;
    deleted?: boolean;
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

{#snippet liftTitle()}<AccountNameTokens
    text={m('admin.moderation.lift_title', { user: accountNameToken(0) })}
    accounts={[{ name: displayName, identity: user }]}
  />{/snippet}

<FormDialog
  bind:visible
  title={m('admin.moderation.lift_title', { user: displayName })}
  titleContent={liftTitle}
  size="sm"
  submitLabel={m('admin.moderation.lift')}
  submitTone="warning"
  submitIcon="iconify icon-[uil--unlock]"
  submitLoadingText={m('admin.moderation.lifting')}
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
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-muted"
      >
        <span class="iconify icon-[uil--user] text-lg"></span>
      </div>
    {/if}
    <div class="min-w-0 flex-1">
      <AccountName name={displayName} identity={user} class="font-medium text-text" />
      <div class="truncate text-sm text-muted">{roomLabel}</div>
    </div>
  </div>

  <TextArea
    id="lift-room-suspension-reason"
    label={m('admin.common.reason')}
    bind:value={reason}
    rows={4}
    maxlength={1000}
    required
    disabled={submitting}
  />
</FormDialog>
