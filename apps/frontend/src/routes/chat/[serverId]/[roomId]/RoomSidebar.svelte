<!--
@component

The **Room Sidebar** — right-hand pane scoped to the current room. Currently
hosts room-scoped extras. The members panel is the first full surface; files,
calls, and similar room-specific panels can plug into the same shell. See the
"UI" section of `docs/GLOSSARY.md`.
-->
<script module lang="ts">
  export type RoomSidebarPanel = 'members' | 'files' | 'call';
</script>

<script lang="ts">
  import { onDestroy } from 'svelte';
  import * as m from '$lib/i18n/messages';
  import { startDMWith } from '$lib/dm/startDM';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import type { PresenceStatus } from '$lib/render/types';
  import type { RoomFilesStore, RoomMember, RoomMembersStore } from '$lib/state/room';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import {
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin
  } from '$lib/state/userProfiles.svelte';
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
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import VoiceCallPanel from '$lib/components/voice/VoiceCallPanel.svelte';
  import RoomFilesPanel from './RoomFilesPanel.svelte';

  let {
    loading = false,
    roomId,
    activePanel = 'members',
    presentation = 'desktop',
    maximized = false,
    hasActiveCall = false,
    canBanRoomMembers = false,
    currentUserId = null,
    membersStore,
    filesStore,
    livekitUrl,
    fileGroupingNow,
    onOpenFile,
    onToggleMaximized,
    onClose
  }: {
    loading?: boolean;
    roomId: string;
    activePanel?: RoomSidebarPanel;
    presentation?: 'desktop' | 'overlay';
    maximized?: boolean;
    hasActiveCall?: boolean;
    canBanRoomMembers?: boolean;
    currentUserId?: string | null;
    membersStore: RoomMembersStore;
    filesStore?: RoomFilesStore;
    livekitUrl?: string;
    fileGroupingNow?: Date;
    onOpenFile?: (messageEventId: string, threadRootEventId: string | null) => void;
    onToggleMaximized?: () => void;
    onClose?: () => void;
  } = $props();

  const connection = useConnection();
  const presenceCache = getPresenceCache();

  const members = $derived(membersStore.filteredMembers);
  const allMembers = $derived(membersStore.members);
  const memberCount = $derived(membersStore.totalCount);
  const title = $derived.by(() => {
    if (activePanel === 'members') return m['room.sidebar.members_title']({ count: memberCount });
    if (activePanel === 'files') return m['room.sidebar.files']();
    return m['room.sidebar.call']();
  });
  const showMaximizeButton = $derived(
    presentation === 'desktop' && activePanel === 'call' && hasActiveCall && !!onToggleMaximized
  );
  const showCallFullscreenButton = $derived(activePanel === 'call' && hasActiveCall);

  // Check if user can start DMs (from centralized server permissions)
  const serverPerms = getServerPermissions();
  let canStartDMs = $derived(serverPerms.current.canStartDMs);
  let sidebarElement = $state<HTMLElement | null>(null);
  let fullscreenElement = $state<Element | null>(null);

  // Track which member's popover is open
  let popoverMemberId = $state<string | null>(null);
  let popoverAnchorRect = $state<DOMRect | null>(null);
  let banningMemberId = $state<string | null>(null);
  let banDialogMember = $state<RoomMember | null>(null);
  let banError = $state<string | null>(null);
  let memberSearchTimer: ReturnType<typeof setTimeout> | null = null;
  let memberSearchInput = $state<HTMLInputElement | null>(null);

  onDestroy(() => {
    if (memberSearchTimer) clearTimeout(memberSearchTimer);
  });

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
    membersStore.presenceVersion,
    sortByName(members.filter((m) => isOnlineStatus(getPresence(m)))))
  );
  const offlineMembers = $derived(
    (presenceCache.version,
    membersStore.presenceVersion,
    sortByName(members.filter((m) => !isOnlineStatus(getPresence(m)))))
  );

  // Look up the selected member for the popover (rendered outside the {#each} loop
  // to avoid Svelte reactivity cycles between the popover's $effect and onlineMembers' $derived)
  const popoverMember = $derived(
    popoverMemberId ? (allMembers.find((m) => m.id === popoverMemberId) ?? null) : null
  );

  const canRemovePopoverMember = $derived(
    !!popoverMember &&
      !popoverMember.deleted &&
      canBanRoomMembers &&
      popoverMember.id !== currentUserId
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
    try {
      const conn = connection();
      const api = createRoomCommandAPI({
        serverId: conn.serverId ?? getActiveServer(),
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      await api.banRoomMember({ roomId, userId: member.id, reason, expiresAt });
    } catch (error) {
      banningMemberId = null;
      banError = m['room.sidebar.ban_failed']();
      toast.error(banError);
      console.error('Failed to ban member from room:', error);
      return;
    }
    banningMemberId = null;

    toast.success(m['room.sidebar.ban_success']({ name: displayName }));
    banDialogMember = null;
  }

  function scheduleMemberSearch(event: Event) {
    const value = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    membersStore.searchInput = value;
    if (memberSearchTimer) clearTimeout(memberSearchTimer);
    memberSearchTimer = setTimeout(() => {
      memberSearchTimer = null;
      void membersStore.setSearch(value);
    }, 250);
  }

  function clearMemberSearch() {
    if (memberSearchTimer) {
      clearTimeout(memberSearchTimer);
      memberSearchTimer = null;
    }
    void membersStore.setSearch('');
    memberSearchInput?.focus();
  }

  async function toggleCallFullscreen(): Promise<void> {
    if (!sidebarElement || typeof document === 'undefined') return;

    try {
      if (document.fullscreenElement === sidebarElement) {
        await document.exitFullscreen();
      } else {
        await sidebarElement.requestFullscreen();
      }
    } catch {
      // Fullscreen can be denied by browser or OS policy; the regular pane still works.
    }
  }
</script>

<svelte:document onfullscreenchange={() => (fullscreenElement = document.fullscreenElement)} />

<aside
  bind:this={sidebarElement}
  class={[
    'relative flex min-h-0 flex-col bg-background',
    presentation === 'desktop'
      ? ['border-l border-border', maximized ? 'min-w-0 flex-1' : '']
      : 'w-full min-w-0 flex-1 overflow-hidden'
  ]}
  style:width={presentation === 'desktop' && !maximized ? `${roomSidebarWidth.value}px` : undefined}
  aria-label={m['room.sidebar.extras']()}
>
  {#if presentation === 'desktop' && !maximized}
    <ResizeHandle
      width={roomSidebarWidth.value}
      min={ROOM_SIDEBAR_MIN_WIDTH}
      max={ROOM_SIDEBAR_MAX_WIDTH}
      onResize={(w) => roomSidebarWidth.set(w)}
      onReset={() => roomSidebarWidth.reset()}
      edge="left"
      label={m['room.sidebar.resize']()}
    />
  {/if}
  <PaneHeader {title} {loading} skeletonButtons={0}>
    {#snippet actions()}
      {#if showMaximizeButton}
        <HeaderIconButton
          icon={maximized ? 'mdi--arrow-collapse-right' : 'mdi--arrow-expand-left'}
          label={maximized ? m['room.sidebar.minimize_call']() : m['room.sidebar.maximize_call']()}
          onclick={() => onToggleMaximized?.()}
        />
      {/if}
      {#if showCallFullscreenButton}
        <HeaderIconButton
          icon={fullscreenElement === sidebarElement
            ? 'mdi--fullscreen-exit'
            : 'mdi--monitor-share'}
          label={fullscreenElement === sidebarElement
            ? m['voice.exit_fullscreen_call']()
            : m['voice.fullscreen_call']()}
          onclick={() => void toggleCallFullscreen()}
        />
      {/if}
      <HeaderIconButton
        icon="uil--times"
        label={m['room.sidebar.hide']()}
        iconSize="lg"
        onclick={() => onClose?.()}
      />
    {/snippet}
  </PaneHeader>

  {#if activePanel === 'members'}
    <nav class="flex flex-1 flex-col overflow-y-auto p-2" aria-label={m['room.sidebar.members']()}>
      <div class="sticky top-0 z-10 bg-background pb-2">
        <label class="sr-only" for="room-member-search">{m['room.sidebar.search_members']()}</label>
        <div class="relative">
          <span
            class="pointer-events-none absolute top-1/2 left-2 iconify h-4 w-4 -translate-y-1/2 text-muted uil--search"
            aria-hidden="true"
          ></span>
          <input
            bind:this={memberSearchInput}
            id="room-member-search"
            type="search"
            value={membersStore.searchInput}
            oninput={scheduleMemberSearch}
            placeholder={m['room.sidebar.search_members_placeholder']()}
            class={[
              'room-member-search-input h-8 w-full rounded-md bg-surface py-1 pl-8 text-sm transition-colors outline-none placeholder:text-muted',
              membersStore.searchInput ? 'pr-8' : 'pr-2'
            ]}
          />
          {#if membersStore.searchInput}
            <button
              type="button"
              class="pane-header-icon-button absolute top-1/2 right-1 h-6 w-6 -translate-y-1/2"
              aria-label={m['room.sidebar.clear_member_search']()}
              title={m['room.sidebar.clear_member_search']()}
              onclick={clearMemberSearch}
            >
              <span class="pane-header-icon-glyph iconify uil--times" aria-hidden="true"></span>
            </button>
          {/if}
        </div>
      </div>

      {#if loading || membersStore.isInitialLoading}
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
        {#if members.length === 0}
          <div class="px-2 py-8 text-center text-sm text-muted">
            {m['room.sidebar.no_members']()}
          </div>
        {:else if onlineMembers.length > 0}
          <CollapsibleGroup
            label={m['room.sidebar.online']({ count: onlineMembers.length })}
            items={onlineMembers}
            item={memberRow}
            persistKey={serverStorageKey(getActiveServer(), 'collapsible:room-members:online')}
          />
        {/if}

        {#if offlineMembers.length > 0}
          <CollapsibleGroup
            label={m['room.sidebar.offline']({ count: offlineMembers.length })}
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
      <RoomFilesPanel
        store={filesStore}
        serverId={getActiveServer()}
        {fileGroupingNow}
        {onOpenFile}
      />
    {:else}
      <div class="flex min-h-0 flex-1 items-center justify-center p-4 text-sm text-muted">
        {m['room.sidebar.no_files']()}
      </div>
    {/if}
  {:else if activePanel === 'call'}
    {#if livekitUrl}
      <VoiceCallPanel {roomId} {livekitUrl} layout={maximized ? 'stage' : 'sidebar'} />
    {:else}
      <div class="flex min-h-0 flex-1 items-center justify-center p-4 text-sm text-muted">
        {m['room.sidebar.calls_unavailable']()}
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

<style>
  .room-member-search-input::-webkit-search-cancel-button {
    -webkit-appearance: none;
    appearance: none;
    display: none;
  }
</style>

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
      ? m['room.sidebar.deleted_user']()
      : m['room.sidebar.view_profile']({ name: getLiveDisplayName(member.id, member.displayName) })}
  >
    <UserAvatar user={member} size="sm" showPresence />
    <div class="min-w-0 flex-1">
      <div class="flex min-w-0 items-center gap-1.5">
        <span class="min-w-0 truncate">{getLiveDisplayName(member.id, member.displayName)}</span>
        <UserCustomStatusBadge
          status={getLiveCustomStatus(member.id, member.customStatus)}
          class="shrink-0 text-xs"
        />
      </div>
      <div class="truncate text-xs text-muted">@{getLiveLogin(member.id, member.login)}</div>
    </div>
  </button>
{/snippet}
