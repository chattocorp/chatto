<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { untrack } from 'svelte';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { m } from '$lib/i18n/messages';
  import { getLiveAvatarUrl, getLiveCustomStatus } from '$lib/state/userProfiles.svelte';
  import { getAvatarColour, getAvatarLabel } from '$lib/utils/initials';
  import UserCustomStatusBadge from './UserCustomStatusBadge.svelte';

  type AvatarUser = Omit<UserAvatarUserView, 'deleted'> & { deleted?: boolean };
  type Size = 'xs' | 'sm' | 'md' | 'message' | 'lg' | 'xl';

  const sizeClasses: Record<Size, string> = {
    xs: 'h-5 w-5',
    sm: 'h-8 w-8',
    md: 'h-10 w-10',
    message: 'h-11 w-11',
    lg: 'h-12 w-12',
    xl: 'h-16 w-16'
  };

  const textSizeClasses: Record<Size, string> = {
    xs: 'text-xs',
    sm: 'text-sm',
    md: 'text-base',
    message: 'text-base',
    lg: 'text-lg',
    xl: 'text-xl'
  };

  const presenceDotColorClasses: Record<PresenceStatus, string> = {
    [PresenceStatus.UNSPECIFIED]: 'bg-presence-offline',
    [PresenceStatus.ONLINE]: 'bg-presence-online',
    [PresenceStatus.AWAY]: 'bg-presence-away',
    [PresenceStatus.DO_NOT_DISTURB]: 'bg-presence-do-not-disturb',
    [PresenceStatus.OFFLINE]: 'bg-presence-offline'
  };

  const presenceDotSizeClasses: Record<Size, string> = {
    xs: '',
    sm: 'h-2 w-2',
    md: 'h-2.5 w-2.5',
    message: 'h-2.5 w-2.5',
    lg: 'h-3 w-3',
    xl: 'h-3.5 w-3.5'
  };

  const presenceDotShellSizeClasses: Record<Size, string> = {
    xs: '',
    sm: 'h-3.5 w-3.5',
    md: 'h-4 w-4',
    message: 'h-4 w-4',
    lg: 'h-[18px] w-[18px]',
    xl: 'h-5 w-5'
  };

  const customStatusTextSizeClasses: Record<Size, string> = {
    xs: 'text-[10px]',
    sm: 'text-xs',
    md: 'text-sm',
    message: 'text-sm',
    lg: 'text-base',
    xl: 'text-lg'
  };
  let {
    user,
    presence: livePresence,
    size = 'md',
    showPresence = false,
    showStatus = false,
    useLiveProfile = true,
    class: className = ''
  }: {
    user: AvatarUser;
    /** Current presence from the owner's server store. Default: the presence in `user`. */
    presence?: PresenceStatus;
    size?: Size;
    showPresence?: boolean;
    showStatus?: boolean;
    /** Disable app-context profile lookups for static directory renderers. */
    useLiveProfile?: boolean;
    class?: string;
  } = $props();

  // Context capture is an initialization concern; callers do not switch one
  // mounted avatar between static and live modes.
  const liveProfileEnabled = untrack(() => useLiveProfile);
  // Guard all derived computations against null user — during tab resume/reconnect,
  // fragment data can be transiently null. An unguarded crash here poisons Svelte 5's
  // reactive graph and deadlocks the entire UI.
  const label = $derived(
    user ? getAvatarLabel(user.displayName, user.login) : { kind: 'icon' as const }
  );
  const colour = $derived(user ? getAvatarColour(user.id) : 'blue');

  const avatarUrl = $derived(
    user && !user.deleted
      ? liveProfileEnabled
        ? getLiveAvatarUrl(user.id, user.avatarUrl ?? null)
        : (user.avatarUrl ?? null)
      : null
  );
  let failedAvatarUrl = $state<string | null>(null);

  const presence = $derived(
    !user || user.deleted ? undefined : (livePresence ?? user.presenceStatus)
  );

  const customStatus = $derived(
    user && !user.deleted
      ? liveProfileEnabled
        ? getLiveCustomStatus(user.id, user.customStatus)
        : (user.customStatus ?? null)
      : null
  );
  const showCustomStatusBadge = $derived(!!user && showStatus && !user.deleted);
  // A bot's public Offline state does not reveal whether it set a presence choice.
  // Show a bot dot only while it has an active public presence state.
  const showPresenceDot = $derived(
    !!presence &&
      showPresence &&
      size !== 'xs' &&
      (!user?.isBot ||
        presence === PresenceStatus.ONLINE ||
        presence === PresenceStatus.AWAY ||
        presence === PresenceStatus.DO_NOT_DISTURB)
  );
  const hasOverlay = $derived(showCustomStatusBadge || showPresenceDot);
  const wrapperClass = $derived(
    [sizeClasses[size], 'inline-grid shrink-0 rounded-full', hasOverlay && 'relative', className]
      .filter(Boolean)
      .join(' ')
  );
  const avatarClass = $derived('h-full w-full overflow-hidden rounded-full');
  const placeholderClass = $derived(
    [
      avatarClass,
      textSizeClasses[size],
      'flex items-center justify-center font-semibold ring-1 ring-inset ring-muted/15',
      user?.deleted ? 'bg-surface-emphasized text-muted' : 'avatar-placeholder'
    ]
      .filter(Boolean)
      .join(' ')
  );

  const presenceLabel = $derived(
    presence === PresenceStatus.ONLINE
      ? 'Online'
      : presence === PresenceStatus.AWAY
        ? 'Away'
        : presence === PresenceStatus.DO_NOT_DISTURB
          ? 'Do not disturb'
          : 'Offline'
  );
</script>

{#if user}
  <div class={wrapperClass}>
    {#if avatarUrl && failedAvatarUrl !== avatarUrl}
      <img
        loading="lazy"
        src={avatarUrl}
        alt={user.login}
        class="{avatarClass} object-cover"
        onerror={() => (failedAvatarUrl = avatarUrl)}
      />
    {:else if user.deleted}
      <div class={placeholderClass} role="img" aria-label={m('common.deleted_user')}>
        <span class="iconify icon-[uil--user-times]" aria-hidden="true"></span>
      </div>
    {:else}
      <div class={placeholderClass} data-avatar-colour={colour} role="img" aria-label={user.login}>
        {#if label.kind === 'icon'}
          <span class="icon-[uil--user]" aria-hidden="true"></span>
        {:else}
          <bdi>{label.text}</bdi>
        {/if}
      </div>
    {/if}
    {#if showCustomStatusBadge}
      <UserCustomStatusBadge
        status={customStatus}
        class="{customStatusTextSizeClasses[
          size
        ]} pointer-events-none absolute end-0 top-0 translate-x-1/4 -translate-y-1/4 [text-shadow:0_1px_2px_rgb(0_0_0_/_0.9),0_0_1px_rgb(0_0_0_/_0.95)] rtl:-translate-x-1/4"
      />
    {/if}
    {#if showPresenceDot && presence}
      <span
        class={[
          presenceDotShellSizeClasses[size],
          'pointer-events-none absolute end-0 bottom-0 grid translate-x-0.5 translate-y-0.5 place-items-center rounded-full border-2 border-surface bg-surface rtl:-translate-x-0.5'
        ]}
        role="img"
        aria-label={presenceLabel}
      >
        <span
          class={[
            presenceDotSizeClasses[size],
            presenceDotColorClasses[presence],
            'presence-dot rounded-full'
          ]}
          data-testid="presence-dot"
          aria-hidden="true"
        ></span>
      </span>
    {/if}
  </div>
{/if}
