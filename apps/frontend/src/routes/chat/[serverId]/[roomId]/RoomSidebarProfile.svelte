<!--
@component

Displays a user's complete public profile in the room sidebar. The header is
the shared user card; right-click, touch long-press, and its menu button open
the shared user menu, which includes bot management for permitted viewers. The
component uses cached user data while it refreshes and updates shared profile
fields as realtime changes arrive.
-->
<script lang="ts">
  import { createUserAPI } from '$lib/api-client/users';
  import BotOwnerRow from '$lib/components/bots/BotOwnerRow.svelte';
  import BotPermissionSummary from '$lib/components/bots/BotPermissionSummary.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import UserMenu from '$lib/components/users/UserMenu.svelte';
  import { UserMenuState } from '$lib/components/users/UserMenuState.svelte';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import UserBio from '$lib/components/users/UserBio.svelte';
  import { m } from '$lib/i18n/messages';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { mapOptionalUserSummary, mapUserPresenceView } from '$lib/api-client/userSummary';
  import {
    getLiveBio,
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin,
    getLiveTimezone
  } from '$lib/state/userProfiles.svelte';
  import { formatAccountName } from '$lib/render/accountName';
  import { Hint, LoadingFog, UserCard } from '$lib/ui';
  import { formatMessageTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

  let {
    userId,
    onSendMessage,
    onOpenProfile
  }: {
    userId: string;
    onSendMessage?: (userId: string) => void;
    onOpenProfile?: (userId: string) => void;
  } = $props();

  const serverScope = useServerScope();
  const viewerTimeSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  let localTimeNow = $state(Date.now());

  const userQuery = createQuery(() => {
    const connection = serverScope.connection;
    return {
      queryKey: [...serverSessionQueryRoot(serverScope.serverId, connection), 'user', userId],
      queryFn: async () => {
        const users = await connection.getAPI(createUserAPI).batchGetUsers([userId]);
        return users[0] ?? null;
      },
      enabled: !!userId,
      staleTime: 30_000,
      refetchInterval: (query) => (query.state.data?.isBot ? 30_000 : false)
    };
  });

  const projectionUser = $derived(serverScope.store.projection.users.get(userId)?.user);
  const baseUser = $derived(mapOptionalUserSummary(projectionUser));
  const loading = $derived(!baseUser && userQuery.isPending);
  const notFound = $derived(!!userId && !loading && !baseUser);
  const displayName = $derived(
    baseUser ? getLiveDisplayName(baseUser.id, baseUser.displayName || baseUser.login) : ''
  );
  const login = $derived(baseUser ? getLiveLogin(baseUser.id, baseUser.login) : '');
  const bio = $derived(baseUser ? getLiveBio(baseUser.id, baseUser.bio ?? null) : null);
  const timezone = $derived(
    baseUser ? getLiveTimezone(baseUser.id, baseUser.timezone ?? null) : null
  );
  const customStatus = $derived(baseUser ? getLiveCustomStatus(baseUser.id, null) : null);
  // Observed presence wins; without it, use the profile's own snapshot value.
  const presence = $derived(
    serverScope.store.presence.get(userId) ?? mapUserPresenceView(projectionUser).presenceStatus
  );
  /** The profile user as the avatar and the shared user menu expect it. */
  const menuUser = $derived(
    baseUser ? { ...baseUser, bio, timezone, presenceStatus: presence, customStatus } : null
  );
  const profileMenu = new UserMenuState<string>();

  function formatLocalTime(zone: string): string | null {
    try {
      return formatMessageTime(new Date(localTimeNow), {
        ...viewerTimeSettings,
        effectiveTimezone: zone
      });
    } catch {
      return null;
    }
  }

  const localTime = $derived(timezone ? formatLocalTime(timezone) : null);
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-y-auto p-4" data-testid="room-sidebar-profile">
  {#if loading}
    <LoadingFog class="h-40 w-full" />
  {:else if notFound || !baseUser || !menuUser}
    <Hint tone="danger">{m('chat.profile.not_found')}</Hint>
  {:else}
    <h2 class="sr-only">{formatAccountName(displayName, baseUser)}</h2>
    <div {@attach profileMenu.trigger(() => userId)}>
      <UserCard
        variant="card"
        name={displayName}
        identity={baseUser}
        username={login}
        testId="profile-user-card"
        menu={{
          label: m('chat.profile.open_card', { name: formatAccountName(displayName, baseUser) }),
          onclick: (event) => profileMenu.toggle(userId, event),
          expanded: profileMenu.target === userId,
          testId: 'profile-user-menu-button'
        }}
      >
        {#snippet avatar()}
          <UserAvatar user={menuUser} {presence} size="sm" showPresence />
        {/snippet}
      </UserCard>
    </div>
    <UserCustomStatusBadge status={customStatus} showText class="mt-3 max-w-full" />

    <UserMenu
      state={profileMenu}
      user={menuUser}
      viewerSettings={serverScope.store.currentUser.user?.settings}
      canSendMessage={!!onSendMessage}
      onSendMessage={() => onSendMessage?.(userId)}
    />

    {#if baseUser.isBot && baseUser.bot?.ownerUserId}
      {#key baseUser.bot?.ownerUserId}
        <BotOwnerRow
          ownerId={baseUser.bot?.ownerUserId}
          {onSendMessage}
          {onOpenProfile}
          viewerSettings={serverScope.store.currentUser.user?.settings}
        />
      {/key}
    {/if}

    {#if bio}
      <div class="-mx-4 mt-6">
        <RoomGroupSection
          label={m('settings.profile.bio.label')}
          persistKey={serverStorageKey(serverScope.serverId, 'profile-bio-collapsed')}
          items={[]}
          testid="profile-bio-heading"
          separated
        >
          {#snippet content()}
            <UserBio {bio} timestampSettings={viewerTimeSettings} class="px-1 pt-2 pb-2" />
          {/snippet}
        </RoomGroupSection>
      </div>
    {/if}

    {#if baseUser.isBot}
      {#key baseUser.id}
        <BotPermissionSummary
          botId={baseUser.id}
          botOwnerId={baseUser.bot?.ownerUserId}
          class={bio ? '' : 'mt-6'}
        />
      {/key}
    {/if}

    {#if timezone && localTime}
      <p class="mt-3 flex items-center gap-1.5 text-muted">
        <span class="iconify icon-[uil--clock-three] shrink-0" aria-hidden="true"></span>
        <span>{m('chat.profile.local_time', { time: localTime, zone: timezone })}</span>
      </p>
      <Interval milliseconds={60_000} ontick={() => (localTimeNow = Date.now())} />
    {/if}
  {/if}
</div>
