<script lang="ts">
  import { RoomEventKind } from '$lib/render/eventKinds';
  import { PresenceStatus, type RoomEventView } from '$lib/render/types';
  import {
    createComposerContext,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import { setUserSettings, UserSettingsState } from '$lib/state/userSettings.svelte';
  import EventList from './EventList.svelte';

  let {
    eventIds,
    roomId = 'room-1',
    eventKind = 'message',
    scrollToEventId,
    onComplete,
    isLoading = false,
    isJumpedMode = false,
    onJumpToPresent,
    updateCounter = 0,
    pendingHighlightId = null,
    hasReachedStart = false
  }: {
    eventIds: string[];
    roomId?: string;
    eventKind?: 'message' | 'join';
    scrollToEventId: string | null;
    onComplete?: () => void;
    isLoading?: boolean;
    isJumpedMode?: boolean;
    onJumpToPresent?: () => Promise<boolean>;
    updateCounter?: number;
    pendingHighlightId?: string | null;
    hasReachedStart?: boolean;
  } = $props();

  createComposerContext({ scroll: true });
  createRoomPermissions(() => DEFAULT_ROOM_PERMISSIONS);
  setUserSettings(new UserSettingsState());

  const events = $derived(
    eventIds.map((id, index): RoomEventView => {
      const base = {
        id,
        createdAt: `2026-06-17T10:47:${String(index).padStart(2, '0')}Z`,
        actorId: `user-${id}`,
        actor: {
          id: `user-${id}`,
          login: id,
          displayName: `User ${id}`,
          deleted: false,
          avatarUrl: null,
          presenceStatus: PresenceStatus.Offline
        }
      };
      if (eventKind === 'join') {
        return {
          ...base,
          event: {
            kind: RoomEventKind.UserJoinedRoom,
            roomId
          }
        } as unknown as RoomEventView;
      }
      return {
        ...base,
        event: {
          kind: RoomEventKind.MessagePosted,
          roomId,
          body: id,
          attachments: [],
          linkPreview: null,
          reactions: [],
          updatedAt: null,
          inReplyTo: null,
          threadRootEventId: null,
          echoOfEventId: null,
          echoFromThreadRootEventId: null,
          channelEchoEventId: null,
          replyCount: 0,
          lastReplyAt: null,
          threadParticipants: [],
          viewerIsFollowingThread: true
        }
      } as RoomEventView;
    })
  );

  const messageStore = {
    refreshCurrentWindow: async () => ({
      hasOlder: false,
      hasNewer: false,
      refreshed: false,
      changed: false
    })
  };
</script>

<EventList
  {roomId}
  messageStore={messageStore as never}
  {events}
  {isLoading}
  {isJumpedMode}
  {onJumpToPresent}
  {updateCounter}
  {pendingHighlightId}
  {hasReachedStart}
  {scrollToEventId}
  onScrollToEventComplete={onComplete}
/>
