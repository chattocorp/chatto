<script lang="ts">
  import type { RoomEventViewFragment } from '$lib/gql/graphql';

  let {
    onMessageSent
  }: {
    onMessageSent?: (event: RoomEventViewFragment | null) => void;
  } = $props();

  const returnedPost = {
    __typename: 'Event',
    id: 'msg-local',
    createdAt: '2026-06-17T10:47:00Z',
    actorId: 'test-user',
    actor: null,
    event: {
      __typename: 'MessagePostedEvent',
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
  } as RoomEventViewFragment;
</script>

<button data-testid="emit-returned-post" onclick={() => onMessageSent?.(returnedPost)}>
  emit returned post
</button>
