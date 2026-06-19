<!--
@component

Test-only wrapper for `RoomSidebar`. Creates the room-member context through
the real sync hook so browser specs can exercise pagination wiring without
mounting the full chat room shell.
-->
<script lang="ts">
  import type { RoomData } from '$lib/hooks/useRoomData.svelte';
  import { createPresenceCache, type PresenceCache } from '$lib/state/presenceCache.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { RoomFilesStore, RoomMembersStore, setRoomMembersStore } from '$lib/state/room';
  import { setUserSettings, UserSettingsState } from '$lib/state/userSettings.svelte';
  import RoomSidebar, { type RoomSidebarPanel } from './RoomSidebar.svelte';

  let {
    roomId = 'room-1',
    roomData: _roomData,
    activePanel = 'members',
    presentation = 'desktop',
    currentUserId = 'viewer',
    canBanRoomMembers = false,
    livekitUrl,
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
    livekitUrl?: string;
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
  const roomMembersStore = setRoomMembersStore(new RoomMembersStore(connection()));

  $effect(() => {
    if (activePanel !== 'files') return;
    roomFilesStore.setRoom(roomId);
  });

  $effect(() => {
    roomMembersStore.setRoom(roomId);
    if (activePanel === 'members') {
      roomMembersStore.ensureLoaded();
    }
  });
</script>

<RoomSidebar
  {roomId}
  {activePanel}
  {presentation}
  loading={false}
  {canBanRoomMembers}
  {currentUserId}
  membersStore={roomMembersStore}
  filesStore={roomFilesStore}
  {livekitUrl}
  {fileGroupingNow}
  {onOpenFile}
  {onClose}
/>
