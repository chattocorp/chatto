<script lang="ts">
  import { fly } from 'svelte/transition';
  import * as m from '$lib/i18n/messages';
  import type { RoomFilesStore, RoomMembersStore } from '$lib/state/room';
  import type { RoomSidebarPanel } from '$lib/storage/roomSidebarPanel';
  import RoomSidebar from './RoomSidebar.svelte';

  let {
    presentation,
    roomId,
    activePanel,
    maximized = false,
    hasActiveCall = false,
    loading = false,
    filesStore,
    livekitUrl,
    canBanRoomMembers = false,
    currentUserId = null,
    membersStore,
    onOpenFile,
    onToggleMaximized,
    onClose
  }: {
    presentation: 'mobile' | 'desktop';
    roomId: string;
    activePanel: RoomSidebarPanel;
    maximized?: boolean;
    hasActiveCall?: boolean;
    loading?: boolean;
    filesStore: RoomFilesStore;
    livekitUrl?: string;
    canBanRoomMembers?: boolean;
    currentUserId?: string | null;
    membersStore: RoomMembersStore;
    onOpenFile: (messageEventId: string, threadRootEventId: string | null) => void;
    onToggleMaximized?: () => void;
    onClose: () => void;
  } = $props();
</script>

{#snippet sidebar()}
  <RoomSidebar
    {roomId}
    {activePanel}
    presentation={presentation === 'mobile' ? 'overlay' : 'desktop'}
    {maximized}
    {hasActiveCall}
    {loading}
    {filesStore}
    {livekitUrl}
    {canBanRoomMembers}
    {currentUserId}
    {membersStore}
    {onOpenFile}
    {onToggleMaximized}
    {onClose}
  />
{/snippet}

{#if presentation === 'mobile'}
  <button
    type="button"
    class="absolute inset-0 z-10 bg-transparent lg:hidden"
    aria-label={m['room.close_extras']()}
    onclick={onClose}
  ></button>
  <div
    class="absolute inset-y-0 right-0 z-20 flex min-h-0 w-full min-w-0 flex-col overflow-hidden border-l border-border bg-background shadow-[-4px_0_12px_rgba(0,0,0,0.15)] sm:w-[90%] lg:hidden"
    data-testid="room-sidebar-mobile-pane"
    transition:fly={{ x: 300, duration: 200 }}
  >
    {@render sidebar()}
  </div>
{:else}
  <div
    class={['hidden min-h-0 min-w-0 lg:flex', maximized ? 'flex-1' : 'shrink-0']}
    data-testid="room-sidebar-desktop-pane"
  >
    {@render sidebar()}
  </div>
{/if}
