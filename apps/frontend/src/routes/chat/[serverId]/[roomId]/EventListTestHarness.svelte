<script lang="ts">
  import { RoomEventKind } from '$lib/render/eventKinds';
  import type { RoomEventView } from '$lib/render/types';
  import {
    createComposerContext,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import { setUserSettings, UserSettingsState } from '$lib/state/userSettings.svelte';
  import EventList from './EventList.svelte';

  let {
    eventIds,
    scrollToEventId,
    onComplete
  }: {
    eventIds: string[];
    scrollToEventId: string | null;
    onComplete?: () => void;
  } = $props();

  createComposerContext({ scroll: true });
  createRoomPermissions(() => DEFAULT_ROOM_PERMISSIONS);
  setUserSettings(new UserSettingsState());

  const events = $derived(
    eventIds.map(
      (id): RoomEventView => ({
        id,
        createdAt: '2026-06-17T10:47:00Z',
        actorId: 'test-user',
        actor: null,
        event: {
          kind: RoomEventKind.MessagePosted,
          roomId: 'room-1',
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
      })
    )
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
  roomId="room-1"
  messageStore={messageStore as never}
  {events}
  isLoading={false}
  {scrollToEventId}
  onScrollToEventComplete={onComplete}
/>
