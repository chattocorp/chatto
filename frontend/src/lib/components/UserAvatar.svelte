<script lang="ts" module>
  import { graphql } from '$lib/gql';

  // Request 96x96 for 2x retina (covers sizes up to lg at 48px CSS pixels)
  export const UserAvatarFragment = graphql(`
    fragment UserAvatarUser on User {
      id
      login
      displayName
      deleted
      avatarUrl(width: 96, height: 96)
      presenceStatus
    }
  `);
</script>

<script lang="ts">
  import type { UserAvatarUserFragment } from '$lib/gql/graphql';
  import { getLiveAvatarUrl } from '$lib/state/userProfiles.svelte';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import { getAvatarInitials } from '$lib/utils/initials';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  type AvatarUser = Omit<UserAvatarUserFragment, 'deleted'> & { deleted?: boolean };
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

  const badgeSizeClasses: Record<Size, string> = {
    xs: 'h-2 w-2',
    sm: 'h-3 w-3',
    md: 'h-3.5 w-3.5',
    lg: 'h-4 w-4',
    xl: 'h-5 w-5'
  };

  let {
    user,
    size = 'md',
    showPresence = true,
    class: className = ''
  }: {
    user: AvatarUser;
    size?: Size;
    showPresence?: boolean;
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

  // Use live presence from global cache if available, otherwise fall back to initial GraphQL value.
  // The global cache is populated by ServerEventProvider, so all UserAvatar instances — including
  // newly-mounted ones like popovers — see the latest presence immediately.
  const presence = $derived(
    user && !user.deleted ? presenceCache.get(user.id, user.presenceStatus) : undefined
  );

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
        class="{badgeSizeClasses[
          size
        ]} absolute right-0 bottom-0 rounded-full border-2 border-surface {badgeColor}"
        aria-label={presenceLabel}
      ></span>
    {/if}
  </div>
{/if}
