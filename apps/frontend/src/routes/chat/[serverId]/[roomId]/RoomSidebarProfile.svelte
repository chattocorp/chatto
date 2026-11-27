<!--
@component

Displays a user's complete public profile in the room sidebar: the shared
profile header, quick actions, bio, details, verified identities, and bot
information. Right-click, touch long-press, and the header's menu button open
the shared user menu, which includes bot management for permitted viewers. The
component uses cached user data while it refreshes and updates shared profile
fields as realtime changes arrive.
-->
<script lang="ts">
  import ProfileVerifiedIdentities from '$lib/components/users/ProfileVerifiedIdentities.svelte';
  import type { Snippet } from 'svelte';
  import ProfileDetails from '$lib/components/users/ProfileDetails.svelte';
  import ProfileRoles from '$lib/components/users/ProfileRoles.svelte';
  import ProfileRecentMessages from './ProfileRecentMessages.svelte';
  import { useProfileDetails } from '$lib/components/users/profileDetailsQuery.svelte';
  import ProfileHeader from '$lib/components/users/ProfileHeader.svelte';
  import ProfileQuickActions from '$lib/components/users/ProfileQuickActions.svelte';
  import { searchUserMessages } from '$lib/components/users/searchUserMessages';
  import { createUserAPI } from '@chatto/client/api/users';
  import BotOwnerRow from '$lib/components/bots/BotOwnerRow.svelte';
  import BotPermissionSummary from '$lib/components/bots/BotPermissionSummary.svelte';
  import UserMenu from '$lib/components/users/UserMenu.svelte';
  import { UserMenuState } from '$lib/components/users/UserMenuState.svelte';
  import UserBio from '$lib/components/users/UserBio.svelte';
  import { m } from '$lib/i18n/messages';
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { mapOptionalUserSummary, mapUserPresenceView } from '@chatto/client/api/userSummary';
  import {
    getLiveBio,
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin,
    getLiveTimezone
  } from '$lib/state/userProfiles.svelte';
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { CompactActionButton, Hint, LoadingDots } from '$lib/ui';
  import { timeFormatSettingsFor } from '$lib/utils/formatTime';

  let {
    userId,
    onSendMessage,
    onMention,
    onOpenProfile
  }: {
    userId: string;
    onSendMessage?: (userId: string) => void;
    /** Mention the user in the room composer. Hosts pass it only while posting is allowed. */
    onMention?: (login: string) => void;
    onOpenProfile?: (userId: string) => void;
  } = $props();

  const serverScope = useServerScope();
  const viewerSettings = $derived(serverScope.store.currentUser.user?.settings);
  const viewerTimeSettings = $derived(timeFormatSettingsFor(viewerSettings));

  const userQuery = createQuery(() => {
    const connection = serverScope.connection;
    return {
      queryKey: [...serverSessionQueryRoot(serverScope), 'user', userId],
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
  // Shares the cache entry that ProfileDetails reads, so the section appears with its rows.
  const details = useProfileDetails(() => userId);
  const hasDetails = $derived(
    !!timezone ||
      !!details.data?.createdAt ||
      (!baseUser?.isBot && !!details.data?.verifiedIdentities.length)
  );
  const hasRoles = $derived(!baseUser?.deleted && !!details.data?.roles.length);
</script>

{#snippet section(label: string, testid: string, content: Snippet)}
  <section class="flex flex-col gap-2" data-testid={testid}>
    <h3 class="section-label text-muted">{label}</h3>
    <div class="tile p-3">{@render content()}</div>
  </section>
{/snippet}

{#snippet bioContent()}
  <UserBio bio={bio ?? ''} timestampSettings={viewerTimeSettings} />
{/snippet}

{#snippet detailsContent()}
  <div class="flex flex-col gap-3">
    <ProfileDetails {userId} {timezone} {viewerSettings} showRoles={false} />
    {#if !baseUser?.isBot}
      <ProfileVerifiedIdentities {userId} plain />
    {/if}
  </div>
{/snippet}

<div class="flex min-h-0 flex-1 flex-col overflow-y-auto" data-testid="room-sidebar-profile">
  {#if loading}
    <LoadingDots class="m-4 h-40" />
  {:else if notFound || !baseUser || !menuUser}
    <div class="p-4"><Hint tone="danger">{m('chat.profile.not_found')}</Hint></div>
  {:else}
    <div {@attach profileMenu.trigger(() => userId)} data-testid="profile-user-card">
      <ProfileHeader
        user={menuUser}
        name={displayName}
        {login}
        {presence}
        {customStatus}
        size="view"
        heading
      >
        {#snippet actions()}
          <CompactActionButton
            label={m('chat.profile.open_card', { name: formatAccountName(displayName, baseUser) })}
            data-testid="profile-user-menu-button"
            onclick={(event: MouseEvent) => profileMenu.toggle(userId, event)}
            aria-haspopup="dialog"
            aria-expanded={profileMenu.target === userId}
          >
            <span class="iconify icon-[uil--ellipsis-v]" aria-hidden="true"></span>
          </CompactActionButton>
        {/snippet}
      </ProfileHeader>
    </div>

    <UserMenu
      state={profileMenu}
      user={menuUser}
      {viewerSettings}
      canSendMessage={!!onSendMessage}
      onSendMessage={() => onSendMessage?.(userId)}
      onMention={onMention && !baseUser.deleted && userId !== serverScope.store.accountId
        ? onMention
        : undefined}
    />

    <div class="flex flex-col gap-6 px-4 pt-4 pb-6">
      {#if !baseUser.deleted}
        <ProfileQuickActions
          onSendMessage={onSendMessage ? () => onSendMessage(userId) : undefined}
          onMention={onMention && userId !== serverScope.store.accountId
            ? () => onMention(login)
            : undefined}
          onSearchMessages={() =>
            searchUserMessages(
              serverScope.serverId,
              serverUi(serverScope.store).messageSearch,
              login
            )}
        />
      {/if}

      {#if baseUser.isBot && baseUser.bot?.ownerUserId}
        {#key baseUser.bot?.ownerUserId}
          <BotOwnerRow
            ownerId={baseUser.bot?.ownerUserId}
            {onSendMessage}
            {onOpenProfile}
            {viewerSettings}
          />
        {/key}
      {/if}

      {#if bio}
        {@render section(m('settings.profile.bio.label'), 'profile-bio', bioContent)}
      {/if}

      {#if hasDetails && !baseUser.deleted}
        {@render section(m('chat.profile.details'), 'profile-details-section', detailsContent)}
      {/if}

      {#if hasRoles}
        <section class="flex flex-col gap-2">
          <h3 class="section-label text-muted">{m('chat.profile.roles')}</h3>
          <ProfileRoles {userId} />
        </section>
      {/if}

      {#if !baseUser.deleted}
        <ProfileRecentMessages {userId} {login} />
      {/if}

      {#if baseUser.isBot}
        {#key baseUser.id}
          <BotPermissionSummary botId={baseUser.id} botOwnerId={baseUser.bot?.ownerUserId} />
        {/key}
      {/if}
    </div>
  {/if}
</div>
