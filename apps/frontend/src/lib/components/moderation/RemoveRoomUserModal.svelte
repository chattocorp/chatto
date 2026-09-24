<script lang="ts">
  import { accountNameToken } from '$lib/render/accountName';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getLiveDisplayName, getLiveLogin } from '$lib/state/userProfiles.svelte';
  import { FormDialog } from '$lib/ui';
  import { ExpirySelect, TextArea } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import type { RoomSuspensionChoice } from '$lib/api-client/rooms';

  type User = {
    id: string;
    login: string;
    displayName: string;
    isBot?: boolean;
    deleted?: boolean;
    avatarUrl?: string | null;
    presenceStatus: PresenceStatus;
  };

  let {
    user,
    isUniversal = false,
    submitting = false,
    error = null,
    onconfirm,
    onclose
  }: {
    user: User;
    isUniversal?: boolean;
    submitting?: boolean;
    error?: string | null;
    onconfirm?: (reason: string, suspension: RoomSuspensionChoice) => void;
    onclose?: () => void;
  } = $props();

  let visible = $state(true);
  let reason = $state('');
  let suspend = $state(false);
  let expiresAt = $state<string | null>(null);
  let expiryValid = $state(true);

  const displayName = $derived(getLiveDisplayName(user.id, user.displayName || user.login));
  const login = $derived(getLiveLogin(user.id, user.login));

  const suspensionSelected = $derived(isUniversal || suspend);
  const disabled = $derived(reason.trim().length === 0 || submitting || (suspensionSelected && !expiryValid));

  function handleSubmit() {
    if (disabled) return;
    const suspension: RoomSuspensionChoice = !suspensionSelected
      ? { kind: 'none' }
      : expiresAt
        ? { kind: 'until', expiresAt }
        : { kind: 'indefinite' };
    onconfirm?.(reason.trim(), suspension);
  }
</script>

{#snippet removeTitle()}<AccountNameTokens
  text={m('admin.moderation.remove_title', { user: accountNameToken(0) })}
  accounts={[{ name: displayName, identity: user }]}
/>{/snippet}

<FormDialog
  bind:visible
  title={m('admin.moderation.remove_title', { user: displayName })}
  titleContent={removeTitle}
  size="sm"
  submitLabel={m('admin.moderation.remove_action')}
  submitTone="danger"
  submitIcon="iconify icon-[uil--user-minus]"
  submitLoadingText={m('admin.moderation.removing')}
  loading={submitting}
  {disabled}
  {error}
  onsubmit={handleSubmit}
  onclose={() => onclose?.()}
>
  <div class="flex items-center gap-3 surface-box p-3">
    <UserAvatar {user} size="md" />
    <div class="min-w-0 flex-1">
      <AccountName name={displayName} identity={user} class="font-medium text-text" />
      <div class="truncate text-sm text-muted">@{login}</div>
    </div>
  </div>

  <TextArea
    id="remove-room-user-reason"
    label={m('admin.common.reason')}
    bind:value={reason}
    rows={4}
    maxlength={1000}
    required
    disabled={submitting}
  />

  {#if !isUniversal}
    <div class="flex flex-col gap-2">
      <label class="flex items-center gap-2">
        <input type="radio" bind:group={suspend} value={false} disabled={submitting} />
        {m('admin.moderation.no_suspension')}
      </label>
      <label class="flex items-center gap-2">
        <input type="radio" bind:group={suspend} value={true} disabled={submitting} />
        {m('admin.moderation.suspend_rejoining')}
      </label>
    </div>
  {/if}

  {#if suspensionSelected}
    <ExpirySelect
      id="remove-room-user-suspension-expires-at"
      label={m('admin.moderation.suspension_period')}
      bind:value={expiresAt}
      bind:valid={expiryValid}
      disabled={submitting}
    />
  {/if}
</FormDialog>
