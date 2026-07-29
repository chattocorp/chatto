<script lang="ts">
  import { startDMWith } from '$lib/dm/startDM';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import BanRoomMemberModal from '$lib/components/moderation/BanRoomMemberModal.svelte';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import type { RoomMember } from '$lib/state/room';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import type { MessageUserInteractionState } from './messageUserInteractions.svelte';

  let {
    interactions,
    serverId,
    roomId,
    currentUserId,
    canStartDMs,
    canBanRoomMembers
  }: {
    interactions: MessageUserInteractionState;
    serverId: string;
    roomId: string;
    currentUserId?: string;
    canStartDMs: boolean;
    canBanRoomMembers: boolean;
  } = $props();

  const connection = useConnection();
  let banningMemberId = $state<string | null>(null);
  let banDialogUser = $state<RoomMember | null>(null);
  let banError = $state<string | null>(null);

  const canBanPopoverUser = $derived.by(() => {
    const user = interactions.user;
    return (
      !!user &&
      !user.deleted &&
      canBanRoomMembers &&
      user.id !== currentUserId &&
      interactions.hasCurrentMember(user.id)
    );
  });

  function openBanDialog(member: RoomMember): void {
    if (member.deleted) return;

    banDialogUser = member;
    banError = null;
    interactions.close();
  }

  async function banFromRoom(
    member: RoomMember,
    reason: string,
    expiresAt: string | null
  ): Promise<void> {
    if (banningMemberId) return;

    banningMemberId = member.id;
    banError = null;
    const displayName = member.displayName || member.login;
    try {
      const api = connection().getAPI(createRoomCommandAPI);
      await api.banMember({ roomId, userId: member.id, reason, expiresAt });
    } catch (error) {
      banningMemberId = null;
      banError = m['room.sidebar.ban_failed']();
      toast.error(banError);
      console.error('Failed to ban member from room:', error);
      return;
    }
    banningMemberId = null;

    toast.success(m['room.sidebar.ban_success']({ name: displayName }));
    banDialogUser = null;
  }
</script>

{#if interactions.user && interactions.anchorRect}
  <UserContextMenu
    user={interactions.user}
    anchorRect={interactions.anchorRect}
    canSendMessage={canStartDMs && !interactions.user.deleted}
    canBanFromRoom={canBanPopoverUser}
    banningFromRoom={banningMemberId === interactions.user.id}
    onSendMessage={() => startDMWith(serverId, interactions.user!.id)}
    onBanFromRoom={() => openBanDialog(interactions.user!)}
    onClose={() => interactions.close()}
  />
{/if}

{#if banDialogUser}
  <BanRoomMemberModal
    user={banDialogUser}
    submitting={banningMemberId === banDialogUser.id}
    error={banError}
    onconfirm={(reason, expiresAt) => banFromRoom(banDialogUser!, reason, expiresAt)}
    onclose={() => (banDialogUser = null)}
  />
{/if}
