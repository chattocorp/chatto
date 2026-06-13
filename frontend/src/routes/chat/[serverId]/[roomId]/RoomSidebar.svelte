<!--
@component

The **Room Sidebar** — right-hand pane scoped to the current room. Currently
shows the member list; will grow to host other room-scoped surfaces (pinned
messages, files, etc.). See the "UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import { graphql } from '$lib/gql';
  import { startDMWith } from '$lib/dm/startDM';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import type { PresenceStatus } from '$lib/gql/graphql';
  import {
    getRoomMembersState,
    getMemberPresence,
    type RoomMember
  } from '$lib/state/room';
  import { getLiveDisplayName, getLiveLogin } from '$lib/state/userProfiles.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import CollapsibleGroup from '$lib/ui/CollapsibleGroup.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import ResizeHandle from '$lib/components/ResizeHandle.svelte';
  import { roomSidebarWidth } from '$lib/state/roomSidebarWidth.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { ROOM_SIDEBAR_MAX_WIDTH, ROOM_SIDEBAR_MIN_WIDTH } from '$lib/storage/roomSidebarWidth';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { toast } from '$lib/ui/toast';
  import BanRoomMemberModal from '$lib/components/moderation/BanRoomMemberModal.svelte';

  const BanRoomMemberMutation = graphql(`
    mutation BanRoomMemberFromSidebar($input: BanRoomMemberInput!) {
      banRoomMember(input: $input)
    }
  `);

  let {
    loading = false,
    roomId,
    canBanRoomMembers = false,
    currentUserId = null,
    onLoadMoreMembers
  }: {
    loading?: boolean;
    roomId: string;
    canBanRoomMembers?: boolean;
    currentUserId?: string | null;
    onLoadMoreMembers?: () => void | Promise<void>;
  } = $props();

  const connection = useConnection();

  // Get members from shared store (populated by Room.svelte)
  const membersState = $derived(getRoomMembersState());
  const members = $derived(membersState.members);
  const memberCount = $derived(membersState.totalCount);

  // Check if user can start DMs (from centralized server permissions)
  const serverPerms = getServerPermissions();
  let canStartDMs = $derived(serverPerms.current.canStartDMs);

  // Track which member's popover is open
  let popoverMemberId = $state<string | null>(null);
  let popoverAnchorRect = $state<DOMRect | null>(null);
  let banningMemberId = $state<string | null>(null);
  let banDialogMember = $state<RoomMember | null>(null);
  let banError = $state<string | null>(null);

  function togglePopover(memberId: string, e: MouseEvent) {
    if (popoverMemberId === memberId) {
      popoverMemberId = null;
      popoverAnchorRect = null;
    } else {
      popoverMemberId = memberId;
      const button = (e.target as HTMLElement).closest('button');
      popoverAnchorRect = button?.getBoundingClientRect() ?? null;
    }
  }

  function closePopover() {
    popoverMemberId = null;
    popoverAnchorRect = null;
  }

  // Get effective presence for a member (live update or fall back to initial value)
  function getPresence(member: RoomMember): PresenceStatus {
    return getMemberPresence(member);
  }

  // Check if a presence status counts as "online" (connected to the system)
  function isOnlineStatus(status: PresenceStatus): boolean {
    return status !== 'OFFLINE';
  }

  // Sort members alphabetically by display name within each presence group.
  // Reading presenceVersion ensures $derived re-runs on any presence change —
  // SvelteMap.size only changes when keys are added/removed, not when existing
  // values change, so it would miss updates like OFFLINE→ONLINE.
  function sortByName(list: RoomMember[]): RoomMember[] {
    return [...list].sort((a, b) =>
      getLiveDisplayName(a.id, a.displayName).localeCompare(
        getLiveDisplayName(b.id, b.displayName)
      )
    );
  }

  const onlineMembers = $derived(
    (membersState.presenceVersion,
    sortByName(members.filter((m) => isOnlineStatus(getPresence(m)))))
  );
  const offlineMembers = $derived(
    (membersState.presenceVersion,
    sortByName(members.filter((m) => !isOnlineStatus(getPresence(m)))))
  );

  // Look up the selected member for the popover (rendered outside the {#each} loop
  // to avoid Svelte reactivity cycles between the popover's $effect and onlineMembers' $derived)
  const popoverMember = $derived(
    popoverMemberId ? (members.find((m) => m.id === popoverMemberId) ?? null) : null
  );

  const canRemovePopoverMember = $derived(
    !!popoverMember && canBanRoomMembers && popoverMember.id !== currentUserId
  );

  function openBanDialog(member: RoomMember) {
    banDialogMember = member;
    banError = null;
    closePopover();
  }

  async function banFromRoom(member: RoomMember, reason: string, expiresAt: string | null) {
    if (banningMemberId) return;

    banningMemberId = member.id;
    banError = null;
    const displayName = member.displayName || member.login;
    const result = await connection().client.mutation(BanRoomMemberMutation, {
      input: { roomId, userId: member.id, reason, expiresAt }
    });
    banningMemberId = null;

    if (result.error) {
      banError = 'Failed to ban member from room';
      toast.error(banError);
      console.error('Failed to ban member from room:', result.error);
      return;
    }

    toast.success(`Banned ${displayName} from room`);
    banDialogMember = null;
  }
</script>

<aside
  class="relative flex flex-col border-l border-border"
  style:width="{roomSidebarWidth.value}px"
  aria-label="Room members"
>
  <ResizeHandle
    width={roomSidebarWidth.value}
    min={ROOM_SIDEBAR_MIN_WIDTH}
    max={ROOM_SIDEBAR_MAX_WIDTH}
    onResize={(w) => roomSidebarWidth.set(w)}
    onReset={() => roomSidebarWidth.reset()}
    edge="left"
    label="Resize members pane"
  />
  <PaneHeader title="Members ({memberCount})" {loading} skeletonButtons={0} />

  <nav class="flex flex-1 flex-col overflow-y-auto p-2" aria-label="Member list">
    {#if loading}
      <ul role="list">
        {#each Array(8) as _, i (i)}
          <li class="flex items-center gap-2 rounded-md px-2 py-1.5">
            <div class="skeleton h-8 w-8 shrink-0 rounded-full"></div>
            <div class="min-w-0 flex-1 space-y-1">
              <div class="skeleton h-3.5 w-24 rounded"></div>
              <div class="skeleton h-3 w-16 rounded"></div>
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      {#if onlineMembers.length > 0}
        <CollapsibleGroup
          label="Online ({onlineMembers.length})"
          items={onlineMembers}
          item={memberRow}
          persistKey={serverStorageKey(getActiveServer(), 'collapsible:room-members:online')}
        />
      {/if}

      {#if offlineMembers.length > 0}
        <CollapsibleGroup
          label="Offline ({offlineMembers.length})"
          items={offlineMembers}
          item={memberRow}
          persistKey={serverStorageKey(getActiveServer(), 'collapsible:room-members:offline')}
          defaultCollapsed
          class="mt-4"
        />
      {/if}

      {#if membersState.hasMore}
        <button
          type="button"
          class="mt-3 flex w-full cursor-pointer items-center justify-center gap-2 rounded-md border border-border px-3 py-2 text-sm font-semibold text-muted transition-colors hover:border-text/30 hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
          disabled={membersState.loadingMore}
          onclick={() => onLoadMoreMembers?.()}
        >
          <span class="iconify text-base uil--angle-down"></span>
          {membersState.loadingMore ? 'Loading members...' : 'Load more members'}
        </button>
      {/if}
    {/if}

    {#if popoverMember && popoverAnchorRect}
      <UserContextMenu
        user={popoverMember}
        anchorRect={popoverAnchorRect}
        canSendMessage={canStartDMs}
        canBanFromRoom={canRemovePopoverMember}
        banningFromRoom={banningMemberId === popoverMember.id}
        onSendMessage={() => startDMWith(getActiveServer(), popoverMember!.id)}
        onBanFromRoom={() => openBanDialog(popoverMember!)}
        onClose={closePopover}
      />
    {/if}
  </nav>

  {#if banDialogMember}
    <BanRoomMemberModal
      user={banDialogMember}
      submitting={banningMemberId === banDialogMember.id}
      error={banError}
      onconfirm={(reason, expiresAt) => banFromRoom(banDialogMember!, reason, expiresAt)}
      onclose={() => (banDialogMember = null)}
    />
  {/if}
</aside>

{#snippet memberRow(member: RoomMember)}
  {@const isOnline = isOnlineStatus(getPresence(member))}
  <button
    type="button"
    class={['sidebar-item w-full cursor-pointer text-left', !isOnline && 'opacity-50']}
    onclick={(e: MouseEvent) => togglePopover(member.id, e)}
    oncontextmenu={(e: MouseEvent) => {
      e.preventDefault();
      togglePopover(member.id, e);
    }}
    title={`View profile of ${getLiveDisplayName(member.id, member.displayName)}`}
  >
    <UserAvatar user={member} size="sm" />
    <div class="min-w-0 flex-1">
      <div class="truncate">{getLiveDisplayName(member.id, member.displayName)}</div>
      <div class="truncate text-xs text-muted">
        @{getLiveLogin(member.id, member.login)}
      </div>
    </div>
  </button>
{/snippet}
