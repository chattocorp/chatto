<!--
@component

The **Room Sidebar** — right-hand pane scoped to the current room. Currently
hosts room-scoped extras. The members panel is the first full surface; files,
calls, and similar room-specific panels can plug into the same shell. See the
"UI" section of `docs/GLOSSARY.md`.
-->
<script module lang="ts">
  export type RoomSidebarPanel = 'members' | 'files';
</script>

<script lang="ts">
  import { graphql } from '$lib/gql';
  import { startDMWith } from '$lib/dm/startDM';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import type { PresenceStatus } from '$lib/gql/graphql';
  import { getRoomMembersState, type RoomFilesStore, type RoomMember } from '$lib/state/room';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
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
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import BanRoomMemberModal from '$lib/components/moderation/BanRoomMemberModal.svelte';
  import RoomFilesPanel from './RoomFilesPanel.svelte';

  const BanRoomMemberMutation = graphql(`
    mutation BanRoomMemberFromSidebar($input: BanRoomMemberInput!) {
      banRoomMember(input: $input)
    }
  `);

  let {
    loading = false,
    roomId,
    activePanel = 'members',
    presentation = 'desktop',
    canBanRoomMembers = false,
    currentUserId = null,
    filesStore,
    fileGroupingNow,
    onLoadMoreMembers,
    onOpenFile,
    onClose
  }: {
    loading?: boolean;
    roomId: string;
    activePanel?: RoomSidebarPanel;
    presentation?: 'desktop' | 'overlay';
    canBanRoomMembers?: boolean;
    currentUserId?: string | null;
    filesStore?: RoomFilesStore;
    fileGroupingNow?: Date;
    onLoadMoreMembers?: () => void | Promise<void>;
    onOpenFile?: (messageEventId: string, threadRootEventId: string | null) => void;
    onClose?: () => void;
  } = $props();

  const connection = useConnection();
  const presenceCache = getPresenceCache();

  // Get members from shared store (populated by Room.svelte)
  const membersState = $derived(getRoomMembersState());
  const members = $derived(membersState.members);
  const memberCount = $derived(membersState.totalCount);
  const title = $derived(activePanel === 'members' ? `Members (${memberCount})` : 'Files');

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
    return presenceCache.get(member.id, member.presenceStatus);
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
      getLiveDisplayName(a.id, a.displayName).localeCompare(getLiveDisplayName(b.id, b.displayName))
    );
  }

  const onlineMembers = $derived(
    (presenceCache.version,
    membersState.presenceVersion,
    sortByName(members.filter((m) => isOnlineStatus(getPresence(m)))))
  );
  const offlineMembers = $derived(
    (presenceCache.version,
    membersState.presenceVersion,
    sortByName(members.filter((m) => !isOnlineStatus(getPresence(m)))))
  );

  // Look up the selected member for the popover (rendered outside the {#each} loop
  // to avoid Svelte reactivity cycles between the popover's $effect and onlineMembers' $derived)
  const popoverMember = $derived(
    popoverMemberId ? (members.find((m) => m.id === popoverMemberId) ?? null) : null
  );

  const canRemovePopoverMember = $derived(
    !!popoverMember && !popoverMember.deleted && canBanRoomMembers && popoverMember.id !== currentUserId
  );

  function openBanDialog(member: RoomMember) {
    if (member.deleted) return;

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

  function loadMoreMembersWhenVisible(node: HTMLElement) {
    if (typeof IntersectionObserver === 'undefined') return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        if (!membersState.hasMore || membersState.loadingMore) return;
        void onLoadMoreMembers?.();
      },
      { rootMargin: '160px 0px' }
    );
    observer.observe(node);

    return () => observer.disconnect();
  }
</script>

<aside
  class={[
    'relative flex min-h-0 flex-col bg-background',
    presentation === 'desktop' ? 'border-l border-border' : 'w-full min-w-0 flex-1 overflow-hidden'
  ]}
  style:width={presentation === 'desktop' ? `${roomSidebarWidth.value}px` : undefined}
  aria-label="Room extras"
>
  {#if presentation === 'desktop'}
    <ResizeHandle
      width={roomSidebarWidth.value}
      min={ROOM_SIDEBAR_MIN_WIDTH}
      max={ROOM_SIDEBAR_MAX_WIDTH}
      onResize={(w) => roomSidebarWidth.set(w)}
      onReset={() => roomSidebarWidth.reset()}
      edge="left"
      label="Resize room extras pane"
    />
  {/if}
  <PaneHeader {title} {loading} skeletonButtons={0}>
    {#snippet actions()}
      <HeaderIconButton icon="uil--times" label="Hide room extras" onclick={() => onClose?.()} />
    {/snippet}
  </PaneHeader>

  {#if activePanel === 'members'}
    <nav class="flex flex-1 flex-col overflow-y-auto p-2" aria-label="Members">
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
          <div
            class="flex justify-center px-3 py-4 text-sm text-muted"
            data-testid="room-members-load-more-sentinel"
            {@attach loadMoreMembersWhenVisible}
          >
            {membersState.loadingMore ? 'Loading members...' : ''}
          </div>
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
  {:else if activePanel === 'files'}
    {#if filesStore}
      <RoomFilesPanel store={filesStore} serverId={getActiveServer()} {fileGroupingNow} {onOpenFile} />
    {:else}
      <div class="flex min-h-0 flex-1 items-center justify-center p-4 text-sm text-muted">
        No files in this room yet.
      </div>
    {/if}
  {/if}

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
    class={[
      'sidebar-item w-full text-left',
      member.deleted ? 'cursor-default' : 'cursor-pointer',
      !isOnline && 'opacity-50'
    ]}
    disabled={member.deleted}
    onclick={(e: MouseEvent) => {
      if (!member.deleted) togglePopover(member.id, e);
    }}
    oncontextmenu={(e: MouseEvent) => {
      e.preventDefault();
      if (!member.deleted) togglePopover(member.id, e);
    }}
    title={member.deleted
      ? 'Deleted User'
      : `View profile of ${getLiveDisplayName(member.id, member.displayName)}`}
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
