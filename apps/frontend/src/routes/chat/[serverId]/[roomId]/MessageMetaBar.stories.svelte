<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MessageMetaBar from './MessageMetaBar.svelte';

  const { Story } = defineMeta({
    title: 'Routes/Chat/MessageMetaBar',
    component: MessageMetaBar,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Message metadata row with thread controls, reaction pills, and capped reaction-user popovers.'
        }
      }
    }
  });

  type ReactionSummary = {
    emoji: string;
    count: number;
    hasReacted: boolean;
    users: { id: string; displayName: string }[];
  };

  const shortReaction: ReactionSummary = {
    emoji: 'thumbsup',
    count: 2,
    hasReacted: false,
    users: [
      { id: 'alice', displayName: 'Alice' },
      { id: 'bob', displayName: 'Bob' }
    ]
  };

  const highCountReaction: ReactionSummary = {
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
</script>

<script lang="ts">
  import { provideConnection } from '$lib/state/server/connection.svelte';
  import { ServerConnection } from '$lib/state/server/serverConnection.svelte';

  const storyConnection = new ServerConnection({
    serverId: 'storybook-server',
    serverUrl: 'http://localhost:4000',
    token: null
  });

  provideConnection(() => storyConnection);

  const commonProps = {
    roomId: 'storybook-room',
    messageEventId: 'storybook-message',
    serverSegment: '-',
    threadRootEventId: null,
    replyCount: 0,
    canReact: false
  };
</script>

{#snippet storyFrame(reactions: ReactionSummary[])}
  <div class="min-h-40 w-96 rounded-md bg-background p-6 text-text">
    <div class="rounded-md bg-surface px-4 py-3 shadow-sm">
      <div class="mb-2 flex items-baseline gap-2">
        <strong class="text-sm font-semibold">Hendrik Mans</strong>
        <span class="text-xs text-muted">19:23</span>
      </div>
      <p class="text-sm text-text">Reaction summary preview</p>
      <MessageMetaBar {...commonProps} {reactions} />
    </div>
  </div>
{/snippet}

<Story name="Short Reaction Popover" asChild>
  {@render storyFrame([shortReaction])}
</Story>

<Story name="High Count Reaction Popover" asChild>
  {@render storyFrame([highCountReaction])}
</Story>
