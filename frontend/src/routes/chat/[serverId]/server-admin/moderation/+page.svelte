<script lang="ts">
  import { onMount } from 'svelte';
  import type { UserAvatarUserFragment } from '$lib/chatTypes';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UnbanRoomMemberModal from '$lib/components/moderation/UnbanRoomMemberModal.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';
  import { toast } from '$lib/ui/toast';
  import {
    CurrentUserPresenceStatus,
    ListRoomBansRequest,
    UnbanRoomMemberRequest,
    type AdminRoomInfoView,
    type RoomBanView,
    type UserAvatarView
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';

  const userSettings = getUserSettings();

  type ModerationRoom = {
    id: string;
    name: string;
  };

  type ModerationRoomBan = {
    id: string;
    roomId: string;
    room: ModerationRoom | null;
    userId: string;
    user: UserAvatarUserFragment | null;
    reason: string;
    expiresAt: string | null;
  };

  let bans = $state.raw<ModerationRoomBan[]>([]);
  let unbanningBanId = $state<string | null>(null);
  let unbanDialogBan = $state<ModerationRoomBan | null>(null);
  let unbanError = $state<string | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  onMount(() => {
    void loadBans();
  });

  function formatDate(value: string | null | undefined): string {
    if (!value) return 'No expiry';
    return formatDateUtil(value, userSettings);
  }

  function roomLabel(ban: ModerationRoomBan): string {
    return ban.room ? `#${ban.room.name}` : ban.roomId;
  }

  function openUnbanDialog(ban: ModerationRoomBan) {
    unbanDialogBan = ban;
    unbanError = null;
  }

  async function loadBans() {
    loading = true;
    error = null;

    try {
      const response = await withActiveServerWireClient((client) =>
        client.listRoomBans(new ListRoomBansRequest())
      );
      bans = response.bans.map(roomBanFromWire);
    } catch (err) {
      error = 'Admin access unavailable';
      console.error('Failed to load room bans:', err);
    } finally {
      loading = false;
    }
  }

  async function unban(ban: ModerationRoomBan, reason: string) {
    if (unbanningBanId) return;
    unbanningBanId = ban.id;
    unbanError = null;

    try {
      await withActiveServerWireClient((client) =>
        client.unbanRoomMember(
          new UnbanRoomMemberRequest({
            roomId: ban.roomId,
            userId: ban.userId,
            reason
          })
        )
      );
    } catch (err) {
      unbanError = 'Failed to unban user';
      toast.error(unbanError);
      console.error('Failed to unban room member:', err);
      return;
    } finally {
      unbanningBanId = null;
    }

    toast.success('User unbanned');
    unbanDialogBan = null;
    await loadBans();
  }

  function roomBanFromWire(ban: RoomBanView): ModerationRoomBan {
    return {
      id: ban.id,
      roomId: ban.roomId,
      room: roomFromWire(ban.room),
      userId: ban.userId,
      user: userAvatarFromWire(ban.user),
      reason: ban.reason,
      expiresAt: ban.expiresAt?.toDate().toISOString() ?? null
    };
  }

  function roomFromWire(room: AdminRoomInfoView | undefined): ModerationRoom | null {
    if (!room) return null;
    return {
      id: room.id,
      name: room.name
    };
  }

  function userAvatarFromWire(view: UserAvatarView | undefined): UserAvatarUserFragment | null {
    const user = view?.user;
    if (!user) return null;
    return {
      id: user.id,
      login: user.login,
      displayName: user.displayName,
      avatarUrl: view.avatarUrl || null,
      presenceStatus: presenceStatusFromWire(view.presenceStatus)
    };
  }

  function presenceStatusFromWire(
    status: CurrentUserPresenceStatus
  ): UserAvatarUserFragment['presenceStatus'] {
    switch (status) {
      case CurrentUserPresenceStatus.ONLINE:
        return 'ONLINE' as UserAvatarUserFragment['presenceStatus'];
      case CurrentUserPresenceStatus.AWAY:
        return 'AWAY' as UserAvatarUserFragment['presenceStatus'];
      case CurrentUserPresenceStatus.DO_NOT_DISTURB:
        return 'DO_NOT_DISTURB' as UserAvatarUserFragment['presenceStatus'];
      case CurrentUserPresenceStatus.OFFLINE:
      case CurrentUserPresenceStatus.UNSPECIFIED:
      default:
        return 'OFFLINE' as UserAvatarUserFragment['presenceStatus'];
    }
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
        <DataTable items={bans} columns={5} emptyMessage="No active room bans">
          {#snippet header()}
            <th class="px-3 py-2 font-medium">User</th>
            <th class="px-3 py-2 font-medium">Room</th>
            <th class="px-3 py-2 font-medium">Reason</th>
            <th class="px-3 py-2 font-medium">Expires</th>
            <th class="px-3 py-2 font-medium"></th>
          {/snippet}
          {#snippet row(ban)}
            {@const user = ban.user}
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

{#if unbanDialogBan}
  {@const unbanDialogUser = unbanDialogBan.user}
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
