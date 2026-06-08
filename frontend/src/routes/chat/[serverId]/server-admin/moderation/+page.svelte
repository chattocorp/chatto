<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useMutation, useQuery } from '$lib/hooks';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UnbanRoomMemberModal from '$lib/components/moderation/UnbanRoomMemberModal.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';
  import { toast } from '$lib/ui/toast';

  const userSettings = getUserSettings();

  const RoomBansQuery = graphql(`
    query AdminRoomBans {
      admin {
        roomBans {
          id
          roomId
          room {
            id
            name
          }
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
      }
    }
  `);

  const UnbanRoomMemberMutation = graphql(`
    mutation AdminUnbanRoomMember($input: UnbanRoomMemberInput!) {
      unbanRoomMember(input: $input)
    }
  `);

  const roomBansQuery = useQuery(RoomBansQuery, () => ({}));
  const unbanMutation = useMutation(UnbanRoomMemberMutation);

  let bans = $derived(roomBansQuery.data?.admin?.roomBans ?? []);
  let unbanningBanId = $state<string | null>(null);
  let unbanDialogBan = $state<(typeof bans)[number] | null>(null);
  let unbanError = $state<string | null>(null);
  let loading = $derived(roomBansQuery.loading);
  let error = $derived(
    roomBansQuery.error ??
      (!roomBansQuery.loading && !roomBansQuery.data?.admin ? 'Admin access unavailable' : null)
  );

  function formatDate(value: string | null | undefined): string {
    if (!value) return 'Indefinite';
    return formatDateUtil(value, userSettings);
  }

  function openUnbanDialog(ban: (typeof bans)[number]) {
    unbanDialogBan = ban;
    unbanError = null;
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
    roomBansQuery.refetch();
  }
</script>

<PageTitle title="Moderation | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Moderation" subtitle="Review and remove active room bans" showMobileNav />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading room bans...</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      <Panel noPadding>
        <DataTable items={bans} columns={6} emptyMessage="No active room bans">
          {#snippet header()}
            <th class="px-4 py-3 font-medium">User</th>
            <th class="px-4 py-3 font-medium">Room</th>
            <th class="px-4 py-3 font-medium">Moderator</th>
            <th class="px-4 py-3 font-medium">Reason</th>
            <th class="px-4 py-3 font-medium">Expires</th>
            <th class="px-4 py-3 font-medium"></th>
          {/snippet}
          {#snippet row(ban)}
            <td class="px-4 py-3">
              {#if ban.user}
                <div class="flex items-center gap-2">
                  <UserAvatar user={ban.user} size="sm" />
                  <div class="min-w-0">
                    <div class="truncate">{ban.user.displayName}</div>
                    <div class="truncate text-xs text-muted">@{ban.user.login}</div>
                  </div>
                </div>
              {:else}
                <span class="text-muted">{ban.userId}</span>
              {/if}
            </td>
            <td class="px-4 py-3">
              {#if ban.room}
                #{ban.room.name}
              {:else}
                <span class="text-muted">{ban.roomId}</span>
              {/if}
            </td>
            <td class="px-4 py-3">
              {#if ban.moderator}
                <div class="flex items-center gap-2">
                  <UserAvatar user={ban.moderator} size="xs" />
                  <span>{ban.moderator.displayName}</span>
                </div>
              {:else}
                <span class="text-muted">{ban.moderatorId}</span>
              {/if}
            </td>
            <td class="max-w-sm px-4 py-3">
              <div class="line-clamp-3 whitespace-pre-wrap">{ban.reason}</div>
              <div class="mt-1 text-xs text-muted">Created {formatDate(ban.createdAt)}</div>
            </td>
            <td class="px-4 py-3 text-muted">{formatDate(ban.expiresAt)}</td>
            <td class="px-4 py-3 text-right">
              <button
                type="button"
                class="btn btn-ghost"
                disabled={unbanningBanId === ban.id}
                onclick={() => openUnbanDialog(ban)}
              >
                {unbanningBanId === ban.id ? 'Unbanning...' : 'Unban'}
              </button>
            </td>
          {/snippet}
        </DataTable>
      </Panel>
    {/if}
  </div>
</div>

{#if unbanDialogBan}
  <UnbanRoomMemberModal
    user={unbanDialogBan.user}
    userId={unbanDialogBan.userId}
    room={unbanDialogBan.room}
    roomId={unbanDialogBan.roomId}
    submitting={unbanningBanId === unbanDialogBan.id}
    error={unbanError}
    onconfirm={(reason) => unban(unbanDialogBan!, reason)}
    onclose={() => (unbanDialogBan = null)}
  />
{/if}
