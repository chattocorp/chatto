<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TypingIndicator from './TypingIndicator.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  const { Story } = defineMeta({
    title: 'Chat/Typing indicator',
    component: TypingIndicator,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Floating typing indicator for room and thread panes. Shows avatars of typing users with a compact label; groups larger than two are aggregated ("A, B and 4 others are typing…"). Positioned absolutely so it never shifts the message list.'
        }
      }
    }
  });

  const alice = {
    id: 'alice',
    login: 'alice',
    displayName: 'Alice',
    presenceStatus: PresenceStatus.ONLINE
  };
  const bob = { id: 'bob', login: 'bob', displayName: 'Bob', presenceStatus: PresenceStatus.ONLINE };
  const carol = {
    id: 'carol',
    login: 'carol',
    displayName: 'Carol',
    presenceStatus: PresenceStatus.ONLINE
  };
  const dave = {
    id: 'dave',
    login: 'dave',
    displayName: 'Dave',
    presenceStatus: PresenceStatus.ONLINE
  };

  const members = [alice, bob, carol, dave];
</script>

<Story name="Single typer" asChild>
  <div class="relative h-24 w-96 overflow-hidden rounded-lg border border-border bg-background p-2">
    <TypingIndicator typingUserIds={['alice']} {members} />
  </div>
</Story>

<Story name="Two typers" asChild>
  <div class="relative h-24 w-96 overflow-hidden rounded-lg border border-border bg-background p-2">
    <TypingIndicator typingUserIds={['alice', 'bob']} {members} />
  </div>
</Story>

<Story name="Large group (aggregate fallback)" asChild>
  <div class="relative h-24 w-96 overflow-hidden rounded-lg border border-border bg-background p-2">
    <TypingIndicator typingUserIds={['alice', 'bob', 'carol', 'dave']} {members} />
  </div>
</Story>

<Story name="Unknown members (aggregate only)" asChild>
  <div class="relative h-24 w-96 overflow-hidden rounded-lg border border-border bg-background p-2">
    <TypingIndicator typingUserIds={['ghost-1', 'ghost-2', 'ghost-3']} {members} />
  </div>
</Story>

<Story name="Hidden when nobody is typing" asChild>
  <div class="relative h-24 w-96 overflow-hidden rounded-lg border border-border bg-background p-2">
    <TypingIndicator typingUserIds={[]} {members} />
  </div>
</Story>
