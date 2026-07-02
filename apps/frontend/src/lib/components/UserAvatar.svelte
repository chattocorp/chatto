<script lang="ts" module>
  import { UserAvatarUserViewDocument } from '$lib/render/types';

  export const UserAvatarViewData = UserAvatarUserViewDocument;
</script>

<script lang="ts">
  import { PresenceStatus, type UserAvatarUserView } from '$lib/render/types';
  import { getLiveAvatarUrl, getLiveCustomStatus } from '$lib/state/userProfiles.svelte';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import { getAvatarInitials } from '$lib/utils/initials';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import UserCustomStatusBadge from './UserCustomStatusBadge.svelte';

  type AvatarUser = Omit<UserAvatarUserView, 'deleted'> & { deleted?: boolean };
  type Size = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

  const sizeClasses: Record<Size, string> = {
    xs: 'h-5 w-5',
    sm: 'h-8 w-8',
    md: 'h-10 w-10',
    lg: 'h-12 w-12',
    xl: 'h-16 w-16'
  };

  const textSizeClasses: Record<Size, string> = {
    xs: 'text-xs',
    sm: 'text-sm',
    md: 'text-base',
    lg: 'text-lg',
    xl: 'text-xl'
  };

  const presenceDotColorClasses: Record<PresenceStatus, string> = {
    [PresenceStatus.Online]: 'bg-presence-online',
    [PresenceStatus.Away]: 'bg-presence-away',
    [PresenceStatus.DoNotDisturb]: 'bg-presence-do-not-disturb',
    [PresenceStatus.Offline]: 'bg-presence-offline'
  };

  const presenceDotSizeClasses: Record<Size, string> = {
    xs: '',
    sm: 'h-2 w-2',
    md: 'h-2.5 w-2.5',
    lg: 'h-3 w-3',
    xl: 'h-3.5 w-3.5'
  };

  const presenceDotShellSizeClasses: Record<Size, string> = {
    xs: '',
    sm: 'h-3.5 w-3.5',
    md: 'h-4 w-4',
    lg: 'h-[18px] w-[18px]',
    xl: 'h-5 w-5'
  };

  const customStatusTextSizeClasses: Record<Size, string> = {
    xs: 'text-[10px]',
    sm: 'text-xs',
    md: 'text-sm',
    lg: 'text-base',
    xl: 'text-lg'
  };
  let {
    user,
    size = 'md',
    showPresence = false,
    presenceOverride = null,
    showStatus = false,
    class: className = ''
  }: {
    user: AvatarUser;
    size?: Size;
    showPresence?: boolean;
    presenceOverride?: PresenceStatus | null;
    showStatus?: boolean;
    class?: string;
  } = $props();

  const presenceCache = getPresenceCache();

  // Guard all derived computations against null user — during tab resume/reconnect,
  // fragment data can be transiently null. An unguarded crash here poisons Svelte 5's
  // reactive graph and deadlocks the entire UI.
  const initials = $derived(user ? getAvatarInitials(user.displayName, user.login) : '');

  const avatarUrl = $derived(
    user && !user.deleted ? getLiveAvatarUrl(user.id, user.avatarUrl ?? null) : null
  );

  // Use live presence from global cache if available, otherwise fall back to the initial value.
  // The global cache is populated by ServerEventProvider, so all UserAvatar instances — including
  // newly-mounted ones like popovers — see the latest presence immediately.
  const presence = $derived.by(() => {
    if (!user || user.deleted) return undefined;
    return presenceOverride ?? presenceCache.get(user.id, user.presenceStatus);
  });

  const customStatus = $derived(
    user && !user.deleted ? getLiveCustomStatus(user.id, user.customStatus) : null
  );
  const showCustomStatusBadge = $derived(!!user && showStatus && !user.deleted);
  const showPresenceDot = $derived(!!presence && showPresence && size !== 'xs');
  const hasOverlay = $derived(showCustomStatusBadge || showPresenceDot);
  const wrapperClass = $derived(
    [
      sizeClasses[size],
      'inline-grid shrink-0 rounded-full',
      hasOverlay && 'relative',
      className
    ]
      .filter(Boolean)
      .join(' ')
  );
  const avatarClass = $derived('h-full w-full overflow-hidden rounded-full');
  const placeholderClass = $derived(
    [
      avatarClass,
      textSizeClasses[size],
      'flex items-center justify-center bg-surface-200 font-semibold text-muted'
    ]
      .filter(Boolean)
      .join(' ')
  );

  const presenceLabel = $derived(
    presence === 'ONLINE'
      ? 'Online'
      : presence === 'AWAY'
        ? 'Away'
        : presence === 'DO_NOT_DISTURB'
          ? 'Do not disturb'
          : 'Offline'
  );
</script>

{#if user}
  <div class={wrapperClass}>
    {#if avatarUrl}
      <SkeletonImg
        loading="lazy"
        src={avatarUrl}
        alt={user.login}
        class="{avatarClass} object-cover"
      />
    {:else}
      <div class={placeholderClass} aria-label={user.login}>
        {initials}
      </div>
    {/if}
    {#if showCustomStatusBadge}
      <UserCustomStatusBadge
        status={customStatus}
        class="{customStatusTextSizeClasses[
          size
        ]} pointer-events-none absolute top-0 right-0 translate-x-1/4 -translate-y-1/4 [text-shadow:0_1px_2px_rgb(0_0_0_/_0.9),0_0_1px_rgb(0_0_0_/_0.95)]"
      />
    {/if}
    {#if showPresenceDot && presence}
      <span
        class={[
          presenceDotShellSizeClasses[size],
          'pointer-events-none absolute right-0 bottom-0 grid translate-x-0.5 translate-y-0.5 place-items-center rounded-full border-2 border-surface bg-surface'
        ]}
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
