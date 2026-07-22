<script lang="ts">
  import MessageMetaBar from './MessageMetaBar.svelte';
  import MessageView from '$lib/components/messages/MessageView.svelte';
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
    | 'plain'
    | 'with-meta-bar'
    | 'footer-comparison'
    | 'compact-grouped'
    | 'search-result'
    | 'deleted';

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

  const threadParticipants: UserAvatarUserView[] = [
    {
      id: 'user-alice',
      login: 'alice',
      displayName: 'Alice',
      deleted: false,
      avatarUrl: null,
      presenceStatus: PresenceStatus.Online
    },
    {
      id: 'user-jordan',
      login: 'jordan',
      displayName: 'Jordan',
      deleted: false,
      avatarUrl: null,
      presenceStatus: PresenceStatus.Away
    }
  ];
  const alice = threadParticipants[0];
  const bea: UserAvatarUserView = {
    id: 'user-bea',
    login: 'bea',
    displayName: 'Bea',
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
      emoji: 'wave',
      count: 2,
      hasReacted: false,
      users: [
        { id: 'user-alice', displayName: 'Alice' },
        { id: 'user-jordan', displayName: 'Jordan' }
      ]
    }
  ];

  function noop() {}
</script>

{#snippet metaBar(replyCount = 2)}
  <MessageMetaBar
    {roomId}
    {messageEventId}
    {serverSegment}
    {threadRootEventId}
    {reactions}
    {replyCount}
    {threadParticipants}
    canReact
    isFollowingThread
    onToggleThreadFollow={noop}
    onOpenThread={noop}
    onOpenEmojiPicker={noop}
  />
{/snippet}

<div class="min-h-screen bg-background p-10 text-text">
  <div
    class="max-w-2xl {variant === 'footer-comparison' ? 'space-y-3' : ''} {variant ===
    'compact-grouped'
      ? 'space-y-0.5'
      : ''}"
  >
    {#if variant === 'plain'}
      <MessageView
        eventId="evt-plain"
        actor={alice}
        displayName="Alice"
        body="Hello!"
        rowClass="bg-surface"
      >
        {#snippet headerMeta()}<span class="text-xs text-muted">10:23</span>{/snippet}
      </MessageView>
    {:else if variant === 'with-meta-bar'}
      <MessageView
        eventId="evt-with-meta"
        actor={alice}
        displayName="Alice"
        body="Hello!"
        rowClass="bg-surface"
        hasFooter
      >
        {#snippet headerMeta()}<span class="text-xs text-muted">10:23</span>{/snippet}
        {#snippet afterBody()}{@render metaBar(2)}{/snippet}
      </MessageView>
    {:else if variant === 'footer-comparison'}
      <MessageView
        eventId="evt-no-footer"
        actor={alice}
        displayName="Alice"
        body="No footer: the hover shell keeps the default row padding."
        rowClass="bg-surface"
      >
        {#snippet headerMeta()}<span class="text-xs text-muted">10:23</span>{/snippet}
      </MessageView>

      <MessageView
        eventId="evt-footer"
        actor={bea}
        displayName="Bea"
        body="With footer: the hover shell uses the footer row padding primitive."
        rowClass="bg-surface"
        hasFooter
      >
        {#snippet headerMeta()}<span class="text-xs text-muted">10:24</span>{/snippet}
        {#snippet afterBody()}{@render metaBar(1)}{/snippet}
      </MessageView>
    {:else if variant === 'compact-grouped'}
      <MessageView
        eventId="evt-group-root"
        actor={alice}
        displayName="Alice"
        body="First message in a short burst."
      >
        {#snippet headerMeta()}<span class="text-xs text-muted">10:23</span>{/snippet}
      </MessageView>

      <MessageView
        eventId="evt-grouped"
        actor={alice}
        displayName="Alice"
        body="Grouped follow-up with reactions."
        rowClass="bg-surface"
        compact
        hasFooter
      >
        {#snippet compactLeading()}<span class="text-xs text-muted">10:24</span>{/snippet}
        {#snippet afterBody()}{@render metaBar(0)}{/snippet}
      </MessageView>
    {:else if variant === 'search-result'}
      <MessageView
        eventId="evt-search"
        actor={alice}
        displayName="Alice"
        body="I found the deployment notes in **Operations** — the checklist is still current."
      >
        {#snippet headerMeta()}
          <span class="inline-flex items-center gap-1 text-xs text-muted">
            <span aria-hidden="true">#</span>
            <span>operations</span>
          </span>
          <span class="text-xs text-muted" aria-hidden="true">·</span>
          <span class="text-xs text-muted">22 July 2026 at 11:42</span>
        {/snippet}
        {#snippet afterBody()}
          <span class="inline-flex items-center gap-1 text-sm text-muted">
            <span class="iconify uil--paperclip" aria-hidden="true"></span>
            2 attachments
          </span>
        {/snippet}
      </MessageView>
    {:else}
      <MessageView
        eventId="evt-deleted"
        actor={null}
        displayName="Deleted User"
        deleted
        rowClass="bg-surface"
      >
        {#snippet headerMeta()}<span class="text-xs text-muted">10:25</span>{/snippet}
      </MessageView>
    {/if}
  </div>
</div>
