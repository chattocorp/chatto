<script lang="ts">
  import MessageMetaBar from './MessageMetaBar.svelte';
  import { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { provideConnection } from '$lib/state/server/connection.svelte';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import {
    PresenceStatus,
    type ReactionSummaryView,
    type UserAvatarUserView
  } from '$lib/render/types';

  type Variant =
    | 'reactions'
    | 'replies-and-reactions'
    | 'unread-followed-thread'
    | 'thread-echo'
    | 'read-only-reactions';

  let { variant }: { variant: Variant } = $props();

  const storyConnection = new ServerConnection({
    serverUrl: 'http://localhost:5173',
    token: null,
    serverId: 'storybook'
  });
  storyConnection.setRealtimeConnectionStatus('connected');
  provideConnection(() => storyConnection);
  createPresenceCache();
  createUserProfileCache();

  const roomId = 'room-design';
  const messageEventId = 'evt-root';
  const serverSegment = '-';
  const threadRootEventId = 'evt-root';

  const alice: UserAvatarUserView = {
    id: 'user-alice',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online
  };
  const jordan: UserAvatarUserView = {
    id: 'user-jordan',
    login: 'jordan',
    displayName: 'Jordan',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Away
  };
  const mika: UserAvatarUserView = {
    id: 'user-mika',
    login: 'mika',
    displayName: 'Mika',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };

  const reactions: ReactionSummaryView[] = [
    {
      emoji: 'joy',
      count: 1,
      hasReacted: true,
      users: [{ id: 'user-current', displayName: 'You' }]
    },
    {
      emoji: 'thumbsup',
      count: 4,
      hasReacted: false,
      users: [
        { id: 'user-alice', displayName: 'Alice' },
        { id: 'user-jordan', displayName: 'Jordan' },
        { id: 'user-mika', displayName: 'Mika' },
        { id: 'user-lee', displayName: 'Lee' }
      ]
    }
  ];

  function noop() {}
</script>

<div class="group/badges inline-flex rounded-md bg-background p-4 text-text">
  {#if variant === 'reactions'}
    <MessageMetaBar
      {roomId}
      {messageEventId}
      {serverSegment}
      {threadRootEventId}
      {reactions}
      canReact
      onOpenEmojiPicker={noop}
    />
  {:else if variant === 'replies-and-reactions'}
    <MessageMetaBar
      {roomId}
      {messageEventId}
      {serverSegment}
      {threadRootEventId}
      {reactions}
      replyCount={2}
      threadParticipants={[alice, jordan, mika]}
      canReact
      isFollowingThread
      onToggleThreadFollow={noop}
      onOpenThread={noop}
      onOpenEmojiPicker={noop}
    />
  {:else if variant === 'unread-followed-thread'}
    <MessageMetaBar
      {roomId}
      {messageEventId}
      {serverSegment}
      {threadRootEventId}
      reactions={[]}
      replyCount={5}
      threadParticipants={[alice, jordan, mika]}
      hasThreadNotification
      canReact
      isFollowingThread
      onToggleThreadFollow={noop}
      onOpenThread={noop}
      onOpenEmojiPicker={noop}
    />
  {:else if variant === 'thread-echo'}
    <MessageMetaBar
      {roomId}
      {messageEventId}
      {serverSegment}
      {threadRootEventId}
      reactions={reactions.slice(0, 1)}
      canReact
      isEchoEvent
      onOpenThread={noop}
      onOpenEmojiPicker={noop}
    />
  {:else}
    <MessageMetaBar
      {roomId}
      {messageEventId}
      {serverSegment}
      {threadRootEventId}
      {reactions}
      canReact={false}
    />
  {/if}
</div>
