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

  type Variant = 'plain' | 'with-meta-bar' | 'footer-comparison' | 'compact-grouped' | 'deleted';

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

{#snippet avatar(initials: string)}
  <div
    class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-lg font-semibold text-muted shadow-md"
  >
    {initials}
  </div>
{/snippet}

{#snippet header(name: string, time: string)}
  <div class="flex min-w-0 items-center gap-2">
    <strong class="shrink-0 leading-none font-semibold">{name}</strong>
    <span class="shrink-0 text-xs leading-none text-muted">{time}</span>
  </div>
{/snippet}

{#snippet body(text: string)}
  <p class="leading-snug text-text">{text}</p>
{/snippet}

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
      <div class="group/msg group/badges message-row items-start bg-surface">
        {@render avatar('A')}
        <div class="message-content-stack">
          {@render header('Alice', '10:23')}
          {@render body('Hello!')}
        </div>
      </div>
    {:else if variant === 'with-meta-bar'}
      <div class="group/msg group/badges message-row items-start bg-surface message-row-footer">
        {@render avatar('A')}
        <div class="message-content-stack">
          {@render header('Alice', '10:23')}
          {@render body('Hello!')}
          {@render metaBar(2)}
        </div>
      </div>
    {:else if variant === 'footer-comparison'}
      <div class="group/msg group/badges message-row items-start bg-surface">
        {@render avatar('A')}
        <div class="message-content-stack">
          {@render header('Alice', '10:23')}
          {@render body('No footer: the hover shell keeps the default row padding.')}
        </div>
      </div>

      <div class="group/msg group/badges message-row items-start bg-surface message-row-footer">
        {@render avatar('B')}
        <div class="message-content-stack">
          {@render header('Bea', '10:24')}
          {@render body('With footer: the hover shell uses the footer row padding primitive.')}
          {@render metaBar(1)}
        </div>
      </div>
    {:else if variant === 'compact-grouped'}
      <div class="group/msg group/badges message-row items-start">
        {@render avatar('A')}
        <div class="message-content-stack">
          {@render header('Alice', '10:23')}
          {@render body('First message in a short burst.')}
        </div>
      </div>

      <div class="group/msg group/badges message-row items-baseline bg-surface message-row-footer">
        <div class="flex w-11 shrink-0 items-center justify-center text-xs text-muted">10:24</div>
        <div class="message-content-stack">
          {@render body('Grouped follow-up with reactions.')}
          {@render metaBar(0)}
        </div>
      </div>
    {:else}
      <div class="group/msg group/badges message-row items-start bg-surface">
        {@render avatar('D')}
        <div class="message-content-stack">
          {@render header('Deleted User', '10:25')}
          <span class="text-muted italic">Message deleted</span>
        </div>
      </div>
    {/if}
  </div>
</div>
