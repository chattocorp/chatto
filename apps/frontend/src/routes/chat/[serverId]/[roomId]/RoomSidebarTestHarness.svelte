<!--
@component

Test-only wrapper for `RoomSidebar`. Creates the room-member context and uses
the same attachment-driven store lifecycle as the room shell so browser specs
can exercise pagination wiring without mounting the full chat room.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import type { RoomData } from '$lib/hooks/useRoomData.svelte';
  import { createPresenceCache, type PresenceCache } from '$lib/state/presenceCache.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { RoomFilesStore, RoomMembersStore, setRoomMembersStore } from '$lib/state/room';
  import { MessageSearchState, MessageSearchStore } from '$lib/state/server/messageSearch.svelte';
  import RoomSidebar, { type RoomSidebarPanel } from './RoomSidebar.svelte';

  let {
    roomId = 'room-1',
    roomData: _roomData,
    activePanel = 'members',
    presentation = 'desktop',
    maximized = false,
    hasActiveCall = false,
    currentUserId = 'viewer',
    canBanRoomMembers = false,
    searchStore = new MessageSearchStore({
      getStatus: async () => ({ state: MessageSearchState.READY, retryAfterMs: null }),
      searchMessages: async () => ({ results: [], nextCursor: null })
    }),
    livekitUrl,
    fileGroupingNow,
    onPresenceCacheReady,
    onOpenFile,
    onOpenSearchResult,
    onToggleMaximized,
    onClose
  }: {
    roomId?: string;
    roomData: RoomData;
    activePanel?: RoomSidebarPanel;
    presentation?: 'desktop' | 'overlay';
    maximized?: boolean;
    hasActiveCall?: boolean;
    currentUserId?: string | null;
    canBanRoomMembers?: boolean;
    searchStore?: MessageSearchStore;
    livekitUrl?: string;
    fileGroupingNow?: Date;
    onPresenceCacheReady?: (cache: PresenceCache) => void;
    onOpenFile?: (messageEventId: string, threadRootEventId: string | null) => void;
    onOpenSearchResult?: (messageEventId: string, threadRootEventId: string | null) => void;
    onToggleMaximized?: () => void;
    onClose?: () => void;
  } = $props();

  const serverScope = useServerScope();
  const presenceCache = createPresenceCache();
  queueMicrotask(() => {
    onPresenceCacheReady?.(presenceCache);
  });
  const roomFilesStore = $derived(new RoomFilesStore(serverScope.connection, roomId));
  const roomMembersStore = setRoomMembersStore(new RoomMembersStore(serverScope.connection));

  const syncMembersStore: Attachment = () => {
    const selectedRoomId = roomId;
    const active = activePanel === 'members';
    untrack(() => {
      roomMembersStore.setRoom(selectedRoomId);
      if (active) roomMembersStore.ensureLoaded();
    });
  };

  const syncFilesStore: Attachment = () => {
    const active = activePanel === 'files';
    if (active) return untrack(() => roomFilesStore.retain());
  };
</script>

<div class="contents" {@attach syncMembersStore} {@attach syncFilesStore}>
  <RoomSidebar
    {roomId}
    {activePanel}
    {presentation}
    {maximized}
    {hasActiveCall}
    loading={false}
    {canBanRoomMembers}
    {currentUserId}
    membersStore={roomMembersStore}
    {searchStore}
    filesStore={roomFilesStore}
    {livekitUrl}
    {fileGroupingNow}
    {onOpenFile}
    {onOpenSearchResult}
    {onToggleMaximized}
    {onClose}
  />
</div>
