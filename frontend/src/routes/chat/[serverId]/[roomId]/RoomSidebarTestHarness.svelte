<!--
@component

Test-only wrapper for `RoomSidebar`. Creates the room-member context through
the real sync hook so browser specs can exercise pagination wiring without
mounting the full chat room shell.
-->
<script lang="ts">
  import { useRoomMembersSync } from '$lib/hooks/useRoomMembersSync.svelte';
  import type { RoomData } from '$lib/hooks/useRoomData.svelte';
  import { createPresenceCache, type PresenceCache } from '$lib/state/presenceCache.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { RoomFilesStore } from '$lib/state/room';
  import { setUserSettings, UserSettingsState } from '$lib/state/userSettings.svelte';
  import RoomSidebar, { type RoomSidebarPanel } from './RoomSidebar.svelte';

  let {
    roomId = 'room-1',
    roomData,
    activePanel = 'members',
    presentation = 'desktop',
    currentUserId = 'viewer',
    canBanRoomMembers = false,
    fileGroupingNow,
    onPresenceCacheReady,
    onOpenFile,
    onClose
  }: {
    roomId?: string;
    roomData: RoomData;
    activePanel?: RoomSidebarPanel;
    presentation?: 'desktop' | 'overlay';
    currentUserId?: string | null;
    canBanRoomMembers?: boolean;
    fileGroupingNow?: Date;
    onPresenceCacheReady?: (cache: PresenceCache) => void;
    onOpenFile?: (messageEventId: string, threadRootEventId: string | null) => void;
    onClose?: () => void;
  } = $props();

  const connection = useConnection();
  setUserSettings(new UserSettingsState());
  const presenceCache = createPresenceCache();
  queueMicrotask(() => {
    onPresenceCacheReady?.(presenceCache);
  });
  const roomFilesStore = new RoomFilesStore(connection());

  $effect(() => {
    if (activePanel !== 'files') return;
    roomFilesStore.setRoom(roomId);
  });

  const roomMembers = useRoomMembersSync(() => ({
    roomId,
    isDM: false,
    roomData,
    dmData: null
  }));
</script>

<RoomSidebar
  {roomId}
  {activePanel}
  {presentation}
  loading={false}
  {canBanRoomMembers}
  {currentUserId}
  filesStore={roomFilesStore}
  {fileGroupingNow}
  onLoadMoreMembers={roomMembers.loadMoreMembers}
  {onOpenFile}
  {onClose}
/>
