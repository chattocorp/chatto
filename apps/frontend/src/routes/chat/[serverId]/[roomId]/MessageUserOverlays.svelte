<script lang="ts">
  import { startDMWith } from '$lib/dm/startDM';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import type { RoomMember } from '$lib/state/room';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import type { MessageUserInteractionState } from './messageUserInteractions.svelte';

  let userContextMenuModule: Promise<
    typeof import('$lib/components/menus/UserContextMenu.svelte')
  > | null = null;
  let userContextMenuLoadAttempt = $state(0);
  let banRoomMemberModalModule: Promise<
    typeof import('$lib/components/moderation/BanRoomMemberModal.svelte')
  > | null = null;
  let banRoomMemberModalLoadAttempt = $state(0);

  function loadUserContextMenu(_attempt: number) {
    userContextMenuModule ??= import('$lib/components/menus/UserContextMenu.svelte').catch(
      (error: unknown) => {
        userContextMenuModule = null;
        throw error;
      }
    );
    return userContextMenuModule;
  }

  function loadBanRoomMemberModal(_attempt: number) {
    banRoomMemberModalModule ??=
      import('$lib/components/moderation/BanRoomMemberModal.svelte').catch((error: unknown) => {
        banRoomMemberModalModule = null;
        throw error;
      });
    return banRoomMemberModalModule;
  }

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

{#snippet loadError(onretry: () => void)}
  <div class="flex flex-col items-center gap-3 p-4 text-center" role="alert">
    <p class="text-sm text-muted">{m['common.error.network']()}</p>
    <button type="button" class="btn-secondary" onclick={onretry}>
      {m['common.retry']()}
    </button>
  </div>
{/snippet}

{#if interactions.user && interactions.anchorRect}
  {#await loadUserContextMenu(userContextMenuLoadAttempt)}
    <span class="sr-only" aria-busy="true">{m['common.loading']()}</span>
  {:then { default: UserContextMenu }}
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
  {:catch}
    <ContextMenu
      anchor={interactions.anchorRect}
      role="alertdialog"
      ariaLabel={m['common.error.generic']()}
      onclose={() => interactions.close()}
    >
      {@render loadError(() => (userContextMenuLoadAttempt += 1))}
    </ContextMenu>
  {/await}
{/if}

{#if banDialogUser}
  {#await loadBanRoomMemberModal(banRoomMemberModalLoadAttempt)}
    <span class="sr-only" aria-busy="true">{m['common.loading']()}</span>
  {:then { default: BanRoomMemberModal }}
    <BanRoomMemberModal
      user={banDialogUser}
      submitting={banningMemberId === banDialogUser.id}
      error={banError}
      onconfirm={(reason, expiresAt) => banFromRoom(banDialogUser!, reason, expiresAt)}
      onclose={() => (banDialogUser = null)}
    />
  {:catch}
    <Dialog visible title={m['common.error.generic']()} onclose={() => (banDialogUser = null)}>
      {@render loadError(() => (banRoomMemberModalLoadAttempt += 1))}
    </Dialog>
  {/await}
{/if}
