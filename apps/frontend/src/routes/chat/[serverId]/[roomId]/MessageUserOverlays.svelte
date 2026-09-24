<script lang="ts">
  import { accountNameToken } from '$lib/render/accountName';
  import { startDMWith } from '$lib/dm/startDM';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { RoomMember } from '$lib/state/room';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { Dialog, LoadingFog } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import type { MessageUserInteractionState } from './messageUserInteractions.svelte';

  type UserContextMenuModule = typeof import('$lib/components/menus/UserContextMenu.svelte');
  type BanRoomMemberModalModule =
    typeof import('$lib/components/moderation/BanRoomMemberModal.svelte');

  let userContextMenuModule: Promise<UserContextMenuModule> | null = null;
  let userContextMenuLoadAttempt = $state(0);
  let banRoomMemberModalModule: Promise<BanRoomMemberModalModule> | null = null;
  let banRoomMemberModalLoadAttempt = $state(0);

  function importUserContextMenu(): Promise<UserContextMenuModule> {
    return import('$lib/components/menus/UserContextMenu.svelte');
  }

  function importBanRoomMemberModal(): Promise<BanRoomMemberModalModule> {
    return import('$lib/components/moderation/BanRoomMemberModal.svelte');
  }

  function loadUserContextMenu(_attempt: number) {
    userContextMenuModule ??= userContextMenuLoader().catch((error: unknown) => {
      userContextMenuModule = null;
      throw error;
    });
    return userContextMenuModule;
  }

  function loadBanRoomMemberModal(_attempt: number) {
    banRoomMemberModalModule ??= banRoomMemberModalLoader().catch((error: unknown) => {
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
    canBanRoomMembers,
    onOpenProfile,
    userContextMenuLoader = importUserContextMenu,
    banRoomMemberModalLoader = importBanRoomMemberModal
  }: {
    interactions: MessageUserInteractionState;
    serverId: string;
    roomId: string;
    currentUserId?: string;
    canStartDMs: boolean;
    canBanRoomMembers: boolean;
    onOpenProfile?: (userId: string) => void;
    userContextMenuLoader?: () => Promise<UserContextMenuModule>;
    banRoomMemberModalLoader?: () => Promise<BanRoomMemberModalModule>;
  } = $props();

  const serverScope = useServerScope();
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
    try {
      const api = serverScope.connection.getAPI(createRoomCommandAPI);
      await api.banMember({ roomId, userId: member.id, reason, expiresAt });
    } catch (error) {
      if (!serverScope.isCurrent()) return;
      banningMemberId = null;
      banError = m('room.sidebar.ban_failed');
      toast.error(banError);
      console.error('Failed to ban member from room:', error);
      return;
    }
    if (!serverScope.isCurrent()) return;
    banningMemberId = null;

    toast.success({
      text: m('room.sidebar.ban_success', { name: accountNameToken(0) }),
      accounts: [{
        name: member.displayName || member.login,
        identity: { isBot: member.isBot, deleted: member.deleted }
      }]
    });
    banDialogUser = null;
  }
</script>

{#snippet loadError(onretry: () => void)}
  <div class="flex flex-col items-center gap-3 p-4 text-center" role="alert">
    <p class="text-sm text-muted">{m('common.error.network')}</p>
    <button type="button" class="btn-secondary" onclick={onretry}>
      {m('common.retry')}
    </button>
  </div>
{/snippet}

{#if interactions.user && interactions.anchorRect}
  {#await loadUserContextMenu(userContextMenuLoadAttempt)}
    <ContextMenu
      anchor={interactions.anchorRect}
      ariaLabel={m('common.loading')}
      onclose={() => interactions.close()}
    >
      <LoadingFog class="m-2 h-28 w-64 max-w-full" />
    </ContextMenu>
  {:then { default: UserContextMenu }}
    <UserContextMenu
      user={interactions.user}
      anchorRect={interactions.anchorRect}
      canSendMessage={canStartDMs && !interactions.user.deleted}
      canBanFromRoom={canBanPopoverUser}
      banningFromRoom={banningMemberId === interactions.user.id}
      viewerSettings={serverScope.store.currentUser.user?.settings}
      onSendMessage={() => startDMWith(serverId, interactions.user!.id)}
      onBanFromRoom={() => openBanDialog(interactions.user!)}
      {onOpenProfile}
      onClose={() => interactions.close()}
    />
  {:catch}
    <ContextMenu
      anchor={interactions.anchorRect}
      role="alertdialog"
      ariaLabel={m('common.error.generic')}
      onclose={() => interactions.close()}
    >
      {@render loadError(() => (userContextMenuLoadAttempt += 1))}
    </ContextMenu>
  {/await}
{/if}

{#if banDialogUser}
  {#await loadBanRoomMemberModal(banRoomMemberModalLoadAttempt)}
    <Dialog visible title={m('admin.moderation.ban_action')} onclose={() => (banDialogUser = null)}>
      <LoadingFog class="h-24 w-full" />
    </Dialog>
  {:then { default: BanRoomMemberModal }}
    <BanRoomMemberModal
      user={banDialogUser}
      submitting={banningMemberId === banDialogUser.id}
      error={banError}
      onconfirm={(reason, expiresAt) => banFromRoom(banDialogUser!, reason, expiresAt)}
      onclose={() => (banDialogUser = null)}
    />
  {:catch}
    <Dialog visible title={m('common.error.generic')} onclose={() => (banDialogUser = null)}>
      {@render loadError(() => (banRoomMemberModalLoadAttempt += 1))}
    </Dialog>
  {/await}
{/if}
