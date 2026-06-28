<script lang="ts" module>
  import { UserAvatarUserViewDocument } from '$lib/render/types';

  export const UserAvatarViewData = UserAvatarUserViewDocument;
</script>

<script lang="ts">
  import type { UserAvatarUserView } from '$lib/render/types';
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

  const presenceRingSizeClasses: Record<Size, string> = {
    xs: 'h-3 w-3',
    sm: 'h-4 w-4',
    md: 'h-4 w-4',
    lg: 'h-5 w-5',
    xl: 'h-5 w-5'
  };

  const presenceDotSizeClasses: Record<Size, string> = {
    xs: 'h-1.5 w-1.5',
    sm: 'h-2.5 w-2.5',
    md: 'h-2.5 w-2.5',
    lg: 'h-3 w-3',
    xl: 'h-3.5 w-3.5'
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
    showStatus = false,
    class: className = ''
  }: {
    user: AvatarUser;
    size?: Size;
    showPresence?: boolean;
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
  const presence = $derived(
    user && !user.deleted ? presenceCache.get(user.id, user.presenceStatus) : undefined
  );

  const customStatus = $derived(
    user && !user.deleted ? getLiveCustomStatus(user.id, user.customStatus) : null
  );
  const showCustomStatusBadge = $derived(showStatus && !user.deleted);

  const badgeColor = $derived(
    presence === 'ONLINE'
      ? 'bg-green-500'
      : presence === 'AWAY'
        ? 'bg-yellow-500'
        : presence === 'DO_NOT_DISTURB'
          ? 'bg-red-500'
          : 'bg-gray-400'
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
  <div class="relative inline-block">
    {#if avatarUrl}
      <SkeletonImg
        loading="lazy"
        src={avatarUrl}
        alt={user.login}
        class="{sizeClasses[size]} rounded-full object-cover {className}"
      />
    {:else}
      <div
        class="{sizeClasses[size]} {textSizeClasses[
          size
        ]} flex items-center justify-center rounded-full bg-surface-200 font-semibold text-muted {className}"
        aria-label={user.login}
      >
        {initials}
      </div>
    {/if}
    {#if showPresence && !user.deleted}
      <span
        class="{presenceRingSizeClasses[
          size
        ]} absolute right-0 bottom-0 grid translate-x-1/4 translate-y-1/4 place-items-center rounded-full border-2 border-surface bg-surface"
        aria-label={presenceLabel}
      >
        <span class="{presenceDotSizeClasses[size]} rounded-full {badgeColor}"></span>
      </span>
    {/if}
    {#if showCustomStatusBadge}
      <UserCustomStatusBadge
        status={customStatus}
        class="{customStatusTextSizeClasses[
          size
        ]} pointer-events-none absolute top-0 right-0 translate-x-1/4 -translate-y-1/4 [text-shadow:0_1px_2px_rgb(0_0_0_/_0.9),0_0_1px_rgb(0_0_0_/_0.95)]"
      />
    {/if}
  </div>
{/if}
