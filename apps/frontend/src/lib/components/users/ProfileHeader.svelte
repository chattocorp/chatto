<!--
@component

Shared header of the Profile Card and the Profile View. A band in a stable,
user-specific colour sits above the avatar, followed by the name, login, and
custom status. Callers supply live values; `actions` renders at the end of the
avatar row, for example the Profile View's menu button.
-->
<script lang="ts">
  import type { ComponentProps, Snippet } from 'svelte';
  import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { CustomUserStatus } from '@chatto/client/api/userSummary';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import AccountName from './AccountName.svelte';
  import { profileBandColor } from './profileBand';

  let {
    user,
    name,
    login,
    presence,
    customStatus,
    size = 'card',
    heading = false,
    actions,
    class: className
  }: {
    user: ComponentProps<typeof UserAvatar>['user'] &
      ComponentProps<typeof AccountName>['identity'];
    /** Live display name, with the login as fallback. */
    name: string;
    /** Live login, shown with an `@` prefix. */
    login: string;
    presence?: PresenceStatus;
    customStatus?: CustomUserStatus | null;
    /** `card` for popovers and sheets; `view` for the taller sidebar header. */
    size?: 'card' | 'view';
    /** Render the name as the page's `h2`. */
    heading?: boolean;
    actions?: Snippet;
    class?: string;
  } = $props();
</script>

<div class={['min-w-0 text-start', className]} data-testid="profile-header">
  <div
    class={['w-full', size === 'view' ? 'h-24' : 'h-14 rounded-t-md']}
    style:background-color={profileBandColor(user.id)}
    aria-hidden="true"
    data-testid="profile-band"
  ></div>
  <div class={size === 'view' ? 'px-4' : 'px-3 pb-3'}>
    <div class="-mt-9 flex items-end justify-between gap-2">
      <div class="rounded-full bg-background p-1">
        <UserAvatar {user} {presence} size="xl" showPresence={presence !== undefined} />
      </div>
      {@render actions?.()}
    </div>
    <div class="mt-1.5 min-w-0">
      <svelte:element this={heading ? 'h2' : 'div'} class="min-w-0" data-testid="profile-name">
        <AccountName
          {name}
          identity={user}
          class={['font-semibold', size === 'view' && 'text-xl']}
        />
      </svelte:element>
      <!-- Popovers keep one text size; hierarchy comes from weight and colour. -->
      <div class={['truncate text-muted', size === 'view' && 'text-sm']} dir="auto">@{login}</div>
    </div>
    <UserCustomStatusBadge status={customStatus} showText class="mt-2 max-w-full" />
  </div>
</div>
