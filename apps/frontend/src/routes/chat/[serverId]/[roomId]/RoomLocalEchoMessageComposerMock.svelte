<script lang="ts">
  import { RoomEventKind } from '$lib/render/eventKinds';
  import type { RoomEventView } from '$lib/render/types';

  let {
    onMessageSent
  }: {
    onMessageSent?: (event: RoomEventView | null) => void;
  } = $props();

  const returnedPost = {
    id: 'msg-local',
    createdAt: '2026-06-17T10:47:00Z',
    actorId: 'test-user',
    actor: null,
    event: {
      kind: RoomEventKind.MessagePosted,
      roomId: 'room-1',
      body: 'local hello',
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

  const returnedEcho = {
    id: 'echo-local',
    createdAt: '2026-06-17T10:48:00Z',
    actorId: 'test-user',
    actor: null,
    event: {
      kind: RoomEventKind.MessagePosted,
      roomId: 'room-1',
      body: 'echoed reply',
      attachments: [],
      linkPreview: null,
      reactions: [],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: null,
      echoOfEventId: 'original-reply',
      echoFromThreadRootEventId: 'thread-root',
      channelEchoEventId: null,
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: true
    }
  } as RoomEventView;
</script>

<button data-testid="emit-returned-post" onclick={() => onMessageSent?.(returnedPost)}>
  emit returned post
</button>

<button data-testid="emit-returned-echo" onclick={() => onMessageSent?.(returnedEcho)}>
  emit returned echo
</button>
