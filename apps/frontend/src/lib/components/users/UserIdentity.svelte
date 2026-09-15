<!--
@component

Renders a compact user identity with the shared avatar and display name. Native
right-click and stationary touch long-press open the shared user profile menu.
With openOnClick, a keyboard-accessible button also opens it on click or tap.
-->
<script module lang="ts">
  type UserContextMenuModule = typeof import('$lib/components/menus/UserContextMenu.svelte');

  let userContextMenuModule: Promise<UserContextMenuModule> | null = null;

  function loadUserContextMenu() {
    userContextMenuModule ??= import('$lib/components/menus/UserContextMenu.svelte');
    return userContextMenuModule;
  }
</script>

<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { m } from '$lib/i18n/messages';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import type { UserAvatarUserView } from '$lib/render/users';
  import type { ViewerTimeSettings } from '$lib/utils/formatTime';
  import {
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui/contextMenuTrigger.svelte';

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
    userContextMenuLoader = loadUserContextMenu
  }: {
    user: IdentityUser;
    size?: 'xs' | 'sm' | 'md';
    /** Render a button that opens the shared profile card on click or tap. */
    openOnClick?: boolean;
    class?: string;
    viewerSettings?: ViewerTimeSettings | null;
    userContextMenuLoader?: () => Promise<UserContextMenuModule>;
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
  let profileMenu = $state<ContextMenuTriggerDetails | null>(null);
  const profileMenuTrigger = contextMenuTrigger((details) => {
    profileMenu = details;
  });
  function openProfile(event: MouseEvent): void {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    profileMenu = { position: { x: rect.left, y: rect.bottom }, presentation: 'auto' };
  }
</script>

{#snippet identity()}
  <UserAvatar user={profileUser} {size} useLiveProfile={false} />
  <bdi class="min-w-0 truncate font-medium text-text-top">
    {profileUser.displayName || profileUser.login}
  </bdi>
{/snippet}

{#if openOnClick}
  <button
    type="button"
    class={[
      'inline-flex min-h-10 min-w-0 cursor-pointer items-center gap-2 rounded text-start hover:underline focus-visible:outline-2 focus-visible:outline-action',
      className
    ]}
    data-testid="user-identity"
    aria-haspopup="dialog"
    aria-label={m('room.sidebar.view_profile', {
      name: profileUser.displayName || profileUser.login
    })}
    onclick={openProfile}
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

{#if profileMenu}
  {#await userContextMenuLoader() then { default: UserContextMenu }}
    <UserContextMenu
      user={profileUser}
      position={profileMenu.position}
      presentation={profileMenu.presentation}
      {viewerSettings}
      onClose={() => (profileMenu = null)}
    />
  {/await}
{/if}
