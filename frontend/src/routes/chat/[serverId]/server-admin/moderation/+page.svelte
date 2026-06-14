<script lang="ts">
  import { graphql, useFragment } from '$lib/gql';
  import { useMutation, useQuery } from '$lib/hooks';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import UnbanRoomMemberModal from '$lib/components/moderation/UnbanRoomMemberModal.svelte';
  import UnsuspendUserModal from '$lib/components/moderation/UnsuspendUserModal.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';
  import { toast } from '$lib/ui/toast';

  const userSettings = getUserSettings();

  const ModerationQuery = graphql(`
    query AdminModeration {
      admin {
        userSuspensions {
          id
          userId
          user {
            id
            login
            displayName
            avatarUrl(width: 96, height: 96)
            presenceStatus
          }
          moderatorId
          moderator {
            id
            login
            displayName
            avatarUrl(width: 96, height: 96)
            presenceStatus
          }
          reason
          createdAt
          expiresAt
        }
        roomBans {
          id
          roomId
          room {
            id
            name
          }
          userId
          user {
            ...UserAvatarUser
          }
          reason
          expiresAt
        }
      }
    }
  `);

  const UnsuspendUserMutation = graphql(`
    mutation AdminUnsuspendUser($input: UnsuspendUserInput!) {
      admin {
        unsuspendUser(input: $input)
      }
    }
  `);

  const UnbanRoomMemberMutation = graphql(`
    mutation AdminUnbanRoomMember($input: UnbanRoomMemberInput!) {
      unbanRoomMember(input: $input)
    }
  `);

  const moderationQuery = useQuery(ModerationQuery, () => ({}));
  const unsuspendMutation = useMutation(UnsuspendUserMutation);
  const unbanMutation = useMutation(UnbanRoomMemberMutation);

  let userSuspensions = $derived(moderationQuery.data?.admin?.userSuspensions ?? []);
  let bans = $derived(moderationQuery.data?.admin?.roomBans ?? []);
  let unsuspendingUserId = $state<string | null>(null);
  let unbanningBanId = $state<string | null>(null);
  let unsuspendDialogSuspension = $state<(typeof userSuspensions)[number] | null>(null);
  let unbanDialogBan = $state<(typeof bans)[number] | null>(null);
  let unsuspendError = $state<string | null>(null);
  let unbanError = $state<string | null>(null);
  let loading = $derived(moderationQuery.loading);
  let error = $derived(
    moderationQuery.error ??
      (!moderationQuery.loading && !moderationQuery.data?.admin ? 'Admin access unavailable' : null)
  );

  function formatDate(value: string | null | undefined): string {
    if (!value) return 'No expiry';
    return formatDateUtil(value, userSettings);
  }

  function roomLabel(ban: (typeof bans)[number]): string {
    return ban.room ? `#${ban.room.name}` : ban.roomId;
  }

  function openUnsuspendDialog(suspension: (typeof userSuspensions)[number]) {
    unsuspendDialogSuspension = suspension;
    unsuspendError = null;
  }

  function openUnbanDialog(ban: (typeof bans)[number]) {
    unbanDialogBan = ban;
    unbanError = null;
  }

  async function unsuspend(suspension: (typeof userSuspensions)[number], reason: string) {
    if (unsuspendingUserId) return;
    unsuspendingUserId = suspension.userId;
    unsuspendError = null;
    const result = await unsuspendMutation.execute({
      input: {
        userId: suspension.userId,
        reason
      }
    });
    unsuspendingUserId = null;

    if (result.error) {
      unsuspendError = 'Failed to unsuspend user';
      toast.error(unsuspendError);
      console.error('Failed to unsuspend user:', result.error);
      return;
    }

    toast.success('User unsuspended');
    unsuspendDialogSuspension = null;
    moderationQuery.refetch();
  }

  async function unban(ban: (typeof bans)[number], reason: string) {
    if (unbanningBanId) return;
    unbanningBanId = ban.id;
    unbanError = null;
    const result = await unbanMutation.execute({
      input: {
        roomId: ban.roomId,
        userId: ban.userId,
        reason
      }
    });
    unbanningBanId = null;

    if (result.error) {
      unbanError = 'Failed to unban user';
      toast.error(unbanError);
      console.error('Failed to unban room member:', result.error);
      return;
    }

    toast.success('User unbanned');
    unbanDialogBan = null;
    moderationQuery.refetch();
  }
</script>

<PageTitle title="Moderation | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Moderation"
    subtitle="Review active server suspensions and room bans"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading moderation state...</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      <Panel title="User Suspensions" icon="iconify uil--user-times" noPadding>
        <DataTable
          items={userSuspensions}
          columns={5}
          emptyMessage="No active user suspensions"
        >
          {#snippet header()}
            <th class="px-3 py-2 font-medium">User</th>
            <th class="px-3 py-2 font-medium">Moderator</th>
            <th class="px-3 py-2 font-medium">Reason</th>
            <th class="px-3 py-2 font-medium">Expires</th>
            <th class="px-3 py-2 font-medium"></th>
          {/snippet}
          {#snippet row(suspension)}
            <td class="min-w-48 px-3 py-2">
              <div class="flex items-center gap-2">
                {#if suspension.user}
                  <UserAvatar user={suspension.user} size="sm" />
                {:else}
                  <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-200 text-muted">
                    <span class="iconify text-base uil--user"></span>
                  </div>
                {/if}
                <div class="min-w-0">
                  <div class="truncate font-medium">{suspension.user?.displayName || suspension.userId}</div>
                  <div class="truncate text-xs text-muted">
                    {#if suspension.user}@{suspension.user.login}{/if}
                  </div>
                </div>
              </div>
            </td>
            <td class="min-w-40 px-3 py-2">
              <div class="truncate">{suspension.moderator?.displayName || suspension.moderatorId}</div>
              <div class="truncate text-xs text-muted">
                {#if suspension.moderator}@{suspension.moderator.login}{/if}
              </div>
            </td>
            <td class="min-w-64 px-3 py-2">
              <div class="line-clamp-2 whitespace-pre-wrap break-words">{suspension.reason}</div>
            </td>
            <td class="px-3 py-2 text-muted">
              <div class="whitespace-nowrap">{formatDate(suspension.expiresAt)}</div>
            </td>
            <td class="px-3 py-2 text-right">
              <Button
                variant="secondary"
                size="sm"
                loading={unsuspendingUserId === suspension.userId}
                loadingText="Unsuspending..."
                onclick={() => openUnsuspendDialog(suspension)}
              >
                <span class="iconify uil--user-check"></span>
                <span>Unsuspend</span>
              </Button>
            </td>
          {/snippet}
        </DataTable>
      </Panel>

      <Panel noPadding>
        <DataTable items={bans} columns={5} emptyMessage="No active room bans">
          {#snippet header()}
            <th class="px-3 py-2 font-medium">User</th>
            <th class="px-3 py-2 font-medium">Room</th>
            <th class="px-3 py-2 font-medium">Reason</th>
            <th class="px-3 py-2 font-medium">Expires</th>
            <th class="px-3 py-2 font-medium"></th>
          {/snippet}
          {#snippet row(ban)}
            {@const user = ban.user ? useFragment(UserAvatarFragment, ban.user) : null}
            <td class="min-w-48 px-3 py-2">
              <div class="flex items-center gap-2">
                {#if user}
                  <UserAvatar {user} size="sm" />
                {:else}
                  <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-200 text-muted">
                    <span class="iconify text-base uil--user"></span>
                  </div>
                {/if}
                <div class="min-w-0">
                  <div class="truncate font-medium">{user?.displayName || ban.userId}</div>
                  <div class="truncate text-xs text-muted">
                    {#if user}@{user.login}{/if}
                  </div>
                </div>
              </div>
            </td>
            <td class="max-w-56 px-3 py-2">
              <div class="truncate">{roomLabel(ban)}</div>
            </td>
            <td class="min-w-64 px-3 py-2">
              <div class="line-clamp-2 whitespace-pre-wrap break-words">{ban.reason}</div>
            </td>
            <td class="px-3 py-2 text-muted">
              <div class="whitespace-nowrap">{formatDate(ban.expiresAt)}</div>
            </td>
            <td class="px-3 py-2 text-right">
              <Button
                variant="secondary"
                size="sm"
                loading={unbanningBanId === ban.id}
                loadingText="Unbanning..."
                onclick={() => openUnbanDialog(ban)}
              >
                <span class="iconify uil--unlock"></span>
                <span>Unban</span>
              </Button>
            </td>
          {/snippet}
        </DataTable>
      </Panel>
    {/if}
  </div>
</div>

{#if unsuspendDialogSuspension}
  <UnsuspendUserModal
    user={unsuspendDialogSuspension.user}
    userId={unsuspendDialogSuspension.userId}
    submitting={unsuspendingUserId === unsuspendDialogSuspension.userId}
    error={unsuspendError}
    onconfirm={(reason) => unsuspend(unsuspendDialogSuspension!, reason)}
    onclose={() => (unsuspendDialogSuspension = null)}
  />
{/if}

{#if unbanDialogBan}
  {@const unbanDialogUser = unbanDialogBan.user
    ? useFragment(UserAvatarFragment, unbanDialogBan.user)
    : null}
  <UnbanRoomMemberModal
    user={unbanDialogUser}
    userId={unbanDialogBan.userId}
    room={unbanDialogBan.room}
    roomId={unbanDialogBan.roomId}
    submitting={unbanningBanId === unbanDialogBan.id}
    error={unbanError}
    onconfirm={(reason) => unban(unbanDialogBan!, reason)}
    onclose={() => (unbanDialogBan = null)}
  />
{/if}
