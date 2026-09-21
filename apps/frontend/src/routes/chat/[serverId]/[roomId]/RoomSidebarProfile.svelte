<!--
@component

Displays a user's complete public profile in the room sidebar. The component
uses cached user data while it refreshes and updates shared profile fields as
realtime changes arrive.
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { createQuery } from '@tanstack/svelte-query';
  import { createUserAPI } from '$lib/api-client/users';
  import BotOwnerRow from '$lib/components/bots/BotOwnerRow.svelte';
  import BotPermissionSummary from '$lib/components/bots/BotPermissionSummary.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import UserBio from '$lib/components/users/UserBio.svelte';
  import { m } from '$lib/i18n/messages';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { mapOptionalUserSummary } from '$lib/api-client/userSummary';
  import {
    getLiveBio,
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin,
    getLiveTimezone
  } from '$lib/state/userProfiles.svelte';
  import { Hint } from '$lib/ui';
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

  const userQuery = createQuery(
    () => {
      const connection = serverScope.connection;
      return {
        queryKey: ['user', serverScope.serverId, connection.queryScope, userId],
        queryFn: async () => {
          const users = await connection.getAPI(createUserAPI).batchGetUsers([userId]);
          return users[0] ?? null;
        },
        enabled: !!userId,
        staleTime: 30_000,
        refetchInterval: (query) => (query.state.data?.isBot ? 30_000 : false)
      };
    },
    () => queryClient
  );

  const baseUser = $derived(mapOptionalUserSummary(serverScope.store.projection.users.get(userId)?.user));
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
  const avatarUser = $derived(
    baseUser
      ? {
          id: baseUser.id,
          login: baseUser.login,
          displayName: baseUser.displayName,
          avatarUrl: baseUser.avatarUrl,
          presenceStatus: PresenceStatus.UNSPECIFIED,
          customStatus: null
        }
      : null
  );

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
    <div class="text-muted" aria-busy="true">{m('common.loading')}</div>
  {:else if notFound || !baseUser || !avatarUser}
    <Hint tone="danger">{m('chat.profile.not_found')}</Hint>
  {:else}
    <div class="flex items-center gap-4">
      <UserAvatar user={avatarUser} serverId={serverScope.serverId} size="xl" />
      <div class="min-w-0 flex-1">
        <h2 class="truncate text-lg font-semibold text-text-top">
          <AccountName name={displayName} identity={baseUser} />
        </h2>
        <p class="truncate text-sm text-muted" dir="ltr">@{login}</p>
        <UserCustomStatusBadge status={customStatus} showText class="mt-1 max-w-full" />
      </div>
    </div>

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
