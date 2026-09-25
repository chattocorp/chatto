<script lang="ts">
  import { accountNameToken } from '$lib/render/accountName';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { getLiveDisplayName, getLiveLogin } from '$lib/state/userProfiles.svelte';
  import { FormDialog, UserCard } from '$lib/ui';
  import { FormField, Select, TextArea } from '$lib/ui/form';
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
  let suspensionPreset = $state('none');
  let customExpiry = $state('');

  const displayName = $derived(getLiveDisplayName(user.id, user.displayName || user.login));
  const login = $derived(getLiveLogin(user.id, user.login));
  const selectedPreset = $derived(
    isUniversal && suspensionPreset === 'none' ? 'indefinite' : suspensionPreset
  );

  const suspensionOptions = $derived([
    ...(!isUniversal ? [{ value: 'none', label: m('admin.moderation.no_suspension') }] : []),
    { value: '24h', label: m('ui.expiry.24h') },
    { value: '7d', label: m('ui.expiry.7d') },
    { value: '30d', label: m('ui.expiry.30d') },
    { value: 'indefinite', label: m('admin.moderation.indefinite') },
    { value: 'custom', label: m('ui.expiry.custom') }
  ]);

  function customExpiryError(value: string): string | null {
    if (!value) return m('ui.expiry.error_required');
    const expiry = new Date(value);
    if (Number.isNaN(expiry.getTime())) return m('ui.expiry.error_invalid');
    if (expiry <= new Date()) return m('ui.expiry.error_future');
    return null;
  }

  const expiryError = $derived(
    selectedPreset === 'custom' ? customExpiryError(customExpiry) : null
  );
  const disabled = $derived(reason.trim().length === 0 || submitting || !!expiryError);

  function handleSubmit() {
    if (disabled) return;
    let suspension: RoomSuspensionChoice;
    switch (selectedPreset) {
      case 'none':
        suspension = { kind: 'none' };
        break;
      case 'indefinite':
        suspension = { kind: 'indefinite' };
        break;
      case 'custom':
        if (customExpiryError(customExpiry)) return;
        suspension = { kind: 'until', expiresAt: new Date(customExpiry).toISOString() };
        break;
      default: {
        const days = { '24h': 1, '7d': 7, '30d': 30 }[selectedPreset];
        if (!days) return;
        suspension = {
          kind: 'until',
          expiresAt: new Date(Date.now() + days * 86_400_000).toISOString()
        };
      }
    }
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
  <UserCard
    variant="card"
    name={displayName}
    identity={user}
    username={login}
    testId="remove-room-user-card"
  >
    {#snippet avatar()}<UserAvatar {user} size="sm" />{/snippet}
  </UserCard>

  <TextArea
    id="remove-room-user-reason"
    label={m('admin.common.reason')}
    bind:value={reason}
    rows={4}
    maxlength={1000}
    required
    disabled={submitting}
  />

  <Select
    id="remove-room-user-suspension"
    label={m('admin.moderation.suspension_period')}
    options={suspensionOptions}
    value={selectedPreset}
    onValueChange={(value) => {
      suspensionPreset = value;
    }}
    disabled={submitting}
  />

  {#if selectedPreset === 'custom'}
    <FormField
      id="remove-room-user-suspension-expires-at"
      label={m('ui.expiry.custom_label')}
      error={expiryError ?? undefined}
    >
      <input
        id="remove-room-user-suspension-expires-at"
        class="input"
        type="datetime-local"
        bind:value={customExpiry}
        disabled={submitting}
        aria-invalid={expiryError ? 'true' : undefined}
        aria-describedby={expiryError ? 'remove-room-user-suspension-expires-at-error' : undefined}
      />
    </FormField>
  {/if}
</FormDialog>
