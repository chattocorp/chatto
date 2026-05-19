<!--
@component

The **Room Sidebar** — right-hand pane scoped to the current room. Currently
shows the member list; will grow to host other room-scoped surfaces (pinned
messages, files, etc.). See the "UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
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
  import { ROOM_SIDEBAR_MAX_WIDTH, ROOM_SIDEBAR_MIN_WIDTH } from '$lib/storage/roomSidebarWidth';
  import { serverStorageKey } from '$lib/storage/serverStorage';


  let { loading = false }: { loading?: boolean } = $props();

  // Get members from shared store (populated by Room.svelte)
  const membersState = $derived(getRoomMembersState());
  const members = $derived(membersState.members);

  // Check if user can write DMs (from centralized instance permissions)
  const serverPerms = getServerPermissions();
  let canWriteDMs = $derived(serverPerms.current.canWriteDMs);

  // Track which member's popover is open
  let popoverMemberId = $state<string | null>(null);
  let popoverAnchorRect = $state<DOMRect | null>(null);

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
  <PaneHeader title="Members ({members.length})" {loading} skeletonButtons={0} />

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
    {/if}

    {#if popoverMember && popoverAnchorRect}
      <UserContextMenu
        user={popoverMember}
        anchorRect={popoverAnchorRect}
        canSendMessage={canWriteDMs}
        onSendMessage={() => startDMWith(getActiveServer(), popoverMember!.id)}
        onClose={closePopover}
      />
    {/if}
  </nav>
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
