<script lang="ts">
  import {
    TimelineEventKind,
    type TimelineEventView
  } from '@chatto/client/timeline/timelineEvents';
  import { getComposerContext } from '$lib/state/room';
  import type { Snippet } from 'svelte';
  import type { LatestThreadRootCandidate } from '$lib/components/composer/messageComposerState.svelte';

  let {
    showCreateThread = false,
    createThreadRequired = false,
    onMessageSent,
    getLatestThreadRootCandidate = () => null,
    inputStatus
  }: {
    showCreateThread?: boolean;
    createThreadRequired?: boolean;
    onMessageSent?: (event: TimelineEventView | null) => void;
    inputStatus?: Snippet;
    getLatestThreadRootCandidate?: () => LatestThreadRootCandidate | null;
  } = $props();

  const composerContext = getComposerContext();
  let candidateId = $state('');

  const returnedPost = {
    id: 'msg-local',
    createdAt: '2026-06-17T10:47:00Z',
    actorId: 'test-user',
    actor: null,
    event: {
      kind: TimelineEventKind.MessagePosted,
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
  } as TimelineEventView;

  const returnedThreadPost = {
    ...returnedPost,
    event: { ...returnedPost.event, threadExists: true }
  } as TimelineEventView;
</script>

{@render inputStatus?.()}

<button data-testid="emit-returned-post" onclick={() => onMessageSent?.(returnedPost)}>
  emit returned post
</button>

<button data-testid="emit-created-thread" onclick={() => onMessageSent?.(returnedThreadPost)}>
  emit created thread
</button>

<button
  data-testid="start-composer-reply"
  onclick={() => composerContext.replyState.startReply('reply-target', 'Reply Target', 'excerpt')}
>
  start composer reply
</button>

<output data-testid="composer-in-reply-to">{composerContext.replyState.messageEventId ?? ''}</output
>
<output data-testid="composer-can-create-thread">{String(showCreateThread)}</output>
<output data-testid="composer-requires-thread">{String(createThreadRequired)}</output>

<button
  data-testid="check-thread-candidate"
  onclick={() => (candidateId = getLatestThreadRootCandidate()?.threadRootEventId ?? '')}
>
  check thread candidate
</button>
<output data-testid="thread-candidate-id">{candidateId}</output>
