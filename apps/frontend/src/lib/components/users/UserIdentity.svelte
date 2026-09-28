<!--
@component

Renders a compact user identity with the shared avatar and display name. Native
right-click and stationary touch long-press open the shared user profile menu.
With openOnClick, a keyboard-accessible button also opens it on click or tap.
-->
<script lang="ts">
  import AccountName from './AccountName.svelte';
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { m } from '$lib/i18n/messages';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import type { ViewerTimeSettings } from '$lib/utils/formatTime';
  import UserMenu, { type UserContextMenuLoader } from './UserMenu.svelte';
  import { UserMenuState } from './UserMenuState.svelte';

  type IdentityUser = Omit<UserAvatarUserView, 'deleted' | 'presenceStatus'> & {
    deleted?: boolean;
    presenceStatus?: PresenceStatus;
    bio?: string | null;
    timezone?: string | null;
  };

  let {
    user,
    size = 'sm',
    openOnClick = false,
    class: className,
    viewerSettings,
    onSendMessage,
    onOpenProfile,
    userContextMenuLoader
  }: {
    user: IdentityUser;
    size?: 'xs' | 'sm' | 'md';
    /** Render a button that opens the shared profile card on click or tap. */
    openOnClick?: boolean;
    class?: string;
    viewerSettings?: ViewerTimeSettings | null;
    /** Host-provided room actions; absent when unavailable in this surface. */
    onSendMessage?: (userId: string) => void;
    onOpenProfile?: (userId: string) => void;
    userContextMenuLoader?: UserContextMenuLoader;
  } = $props();

  const profileUser = $derived<IdentityUser & { deleted: boolean; presenceStatus: PresenceStatus }>(
    {
      ...user,
      bio: user.bio,
      timezone: user.timezone,
      deleted: user.deleted ?? false,
      presenceStatus: user.presenceStatus ?? PresenceStatus.OFFLINE
    }
  );
  const profileMenu = new UserMenuState<string>();
  const profileMenuTrigger = profileMenu.trigger(() => user.id);
</script>

{#snippet identity()}
  <UserAvatar user={profileUser} {size} useLiveProfile={false} />
  <AccountName
    name={profileUser.displayName || profileUser.login}
    identity={profileUser}
    class="font-medium text-text-top"
  />
{/snippet}

{#if openOnClick}
  <button
    type="button"
    class={[
      '-mx-1 inline-flex min-h-10 min-w-0 cursor-pointer items-center gap-2 rounded-md px-1 text-start focus-visible:outline-2 focus-visible:outline-action',
      className
    ]}
    data-testid="user-identity"
    aria-haspopup="dialog"
    aria-label={m('room.sidebar.view_profile', {
      name: formatAccountName(profileUser.displayName || profileUser.login, profileUser)
    })}
    onclick={(event) => profileMenu.open(user.id, event)}
    {@attach profileMenuTrigger}
  >
    {@render identity()}
  </button>
{:else}
  <span
    class={['inline-flex min-w-0 items-center gap-2', className]}
    data-testid="user-identity"
    {@attach profileMenuTrigger}
  >
    {@render identity()}
  </span>
{/if}

<UserMenu
  state={profileMenu}
  user={profileUser}
  loader={userContextMenuLoader}
  {viewerSettings}
  canSendMessage={!!onSendMessage}
  onSendMessage={() => onSendMessage?.(user.id)}
  {onOpenProfile}
/>
