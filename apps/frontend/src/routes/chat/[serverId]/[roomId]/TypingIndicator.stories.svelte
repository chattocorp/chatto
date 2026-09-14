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
            'Floating typing indicator for room and thread panes. Shows up to three avatars and two names, then counts the remaining people. Missing profiles use an unknown-user label. The indicator stays inside its pane and does not move messages.'
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
  const bob = {
    id: 'bob',
    login: 'bob',
    displayName: 'Bob',
    presenceStatus: PresenceStatus.ONLINE
  };
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

<Story name="Unknown members" asChild>
  <div class="relative h-24 w-96 overflow-hidden rounded-lg border border-border bg-background p-2">
    <TypingIndicator typingUserIds={['ghost-1', 'ghost-2', 'ghost-3']} {members} />
  </div>
</Story>

<Story name="Partially known group" asChild>
  <div
    class="relative h-24 w-96 max-w-full overflow-hidden rounded-lg border border-border bg-background p-2"
  >
    <TypingIndicator typingUserIds={['alice', 'ghost', 'carol']} members={[alice]} />
  </div>
</Story>

<Story name="Narrow pane with long names" asChild>
  <div class="relative h-24 w-60 max-w-full rounded-lg border border-border bg-background p-2">
    <TypingIndicator
      typingUserIds={['alice', 'bob', 'carol', 'dave']}
      members={[
        { ...alice, displayName: 'A very long display name for a narrow conversation' },
        bob,
        carol,
        dave
      ]}
    />
  </div>
</Story>

<Story name="Mixed direction names" asChild>
  <div
    dir="rtl"
    class="relative h-24 w-60 max-w-full rounded-lg border border-border bg-background p-2"
  >
    <TypingIndicator
      typingUserIds={['alice', 'bob']}
      members={[{ ...alice, displayName: 'علي' }, bob]}
    />
  </div>
</Story>

<Story name="Hidden when nobody is typing" asChild>
  <div class="relative h-24 w-96 overflow-hidden rounded-lg border border-border bg-background p-2">
    <TypingIndicator typingUserIds={[]} {members} />
  </div>
</Story>
