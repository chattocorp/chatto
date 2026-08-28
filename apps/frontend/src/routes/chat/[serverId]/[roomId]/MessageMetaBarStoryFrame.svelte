<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import MessageMetaBar from './MessageMetaBar.svelte';
  import { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import type { ReactionSummaryView } from '$lib/render/reactions';
  import type { UserAvatarUserView } from '$lib/render/users';
  import type { MessageActionModel } from './messageActionModel';
  type Variant =
    | 'reactions'
    | 'replies-and-reactions'
    | 'unread-followed-thread'
    | 'thread-echo'
    | 'read-only-reactions'
    | 'short-reaction-popover'
    | 'high-count-reaction-popover';

  let { variant }: { variant: Variant } = $props();

  const storyConnection = new ServerConnection({
    serverUrl: 'http://localhost:5173',
    token: null,
    serverId: 'storybook'
  });
  storyConnection.setRealtimeConnectionStatus('connected');
  provideServerScope({
    serverId: 'storybook',
    connection: storyConnection,
    store: {} as ServerStateStore,
    isCurrent: () => true
  });
  createPresenceCache();
  createUserProfileCache();

  const roomId = 'room-design';
  const serverSegment = '-';
  const threadRootEventId = 'evt-root';

  const alice: UserAvatarUserView = {
    id: 'user-alice',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.ONLINE
  };
  const jordan: UserAvatarUserView = {
    id: 'user-jordan',
    login: 'jordan',
    displayName: 'Jordan',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.AWAY
  };
  const mika: UserAvatarUserView = {
    id: 'user-mika',
    login: 'mika',
    displayName: 'Mika',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE
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
  const shortReaction: ReactionSummaryView = {
    emoji: 'thumbsup',
    count: 2,
    hasReacted: false,
    users: [
      { id: 'alice', displayName: 'Alice' },
      { id: 'bob', displayName: 'Bob' }
    ]
  };
  const highCountReaction: ReactionSummaryView = {
    emoji: 'heart',
    count: 72,
    hasReacted: false,
    users: [
      { id: 'azerbaijan', displayName: 'Azerbaijan' },
      { id: 'german-noob', displayName: 'German_Noob_With_An_Absurdly_Long_Name' },
      { id: '2tap2b', displayName: '2tap2b' },
      { id: 'muchtin', displayName: 'muchtin' },
      { id: 'patry', displayName: 'patry' }
    ]
  };

  function noop() {}
  async function noopAsync() {}
  const action: MessageActionModel = {
    serverId: 'storybook',
    messageBody: '',
    canReact: true,
    canEdit: false,
    canDelete: false,
    canPin: false,
    isPinned: false,
    replyInRoomLabel: 'Reply',
    replyThreadLabel: 'Reply in thread',
    hasReacted: () => false,
    toggleReaction: noopAsync,
    edit: noop,
    copyText: noopAsync,
    copyLink: noopAsync,
    delete: noop,
    togglePin: noopAsync
  };
  const readOnlyAction: MessageActionModel = { ...action, canReact: false };
</script>

<div class="group/badges inline-flex rounded-md bg-background p-4 text-text">
  {#if variant === 'reactions'}
    <MessageMetaBar
      {roomId}
      {serverSegment}
      {threadRootEventId}
      {reactions}
      {action}
      onOpenEmojiPicker={noop}
    />
  {:else if variant === 'replies-and-reactions'}
    <MessageMetaBar
      {roomId}
      {serverSegment}
      {threadRootEventId}
      {reactions}
      {action}
      replyCount={2}
      threadParticipants={[alice, jordan, mika]}
      isFollowingThread
      onToggleThreadFollow={noop}
      onOpenThread={noop}
      onOpenEmojiPicker={noop}
    />
  {:else if variant === 'unread-followed-thread'}
    <MessageMetaBar
      {roomId}
      {serverSegment}
      {threadRootEventId}
      reactions={[]}
      {action}
      replyCount={5}
      threadParticipants={[alice, jordan, mika]}
      hasThreadNotification
      isFollowingThread
      onToggleThreadFollow={noop}
      onOpenThread={noop}
      onOpenEmojiPicker={noop}
    />
  {:else if variant === 'thread-echo'}
    <MessageMetaBar
      {roomId}
      {serverSegment}
      {threadRootEventId}
      reactions={reactions.slice(0, 1)}
      {action}
      isEchoEvent
      onOpenThread={noop}
      onOpenEmojiPicker={noop}
    />
  {:else if variant === 'read-only-reactions'}
    <MessageMetaBar
      {roomId}
      {serverSegment}
      {threadRootEventId}
      {reactions}
      action={readOnlyAction}
    />
  {:else if variant === 'short-reaction-popover'}
    <MessageMetaBar
      {roomId}
      {serverSegment}
      {threadRootEventId}
      reactions={[shortReaction]}
      action={readOnlyAction}
    />
  {:else}
    <MessageMetaBar
      {roomId}
      {serverSegment}
      {threadRootEventId}
      reactions={[highCountReaction]}
      action={readOnlyAction}
    />
  {/if}
</div>
