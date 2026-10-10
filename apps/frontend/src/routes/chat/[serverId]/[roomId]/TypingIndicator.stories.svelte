<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TypingIndicator from './TypingIndicator.svelte';
  import type { RoomMember } from '$lib/state/room';
  import { Button } from '$lib/ui/form';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import avatarUrl from '$lib/assets/bot.svg';

  const { Story } = defineMeta({
    title: 'Chat/Typing indicator',
    component: TypingIndicator,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Typing indicator on the message composer of room and thread panes. Shows up to three avatars and two names, then counts the remaining people. A bright dot moves clockwise around a 3×3 grid with a fading trail and stays still with reduced motion. Missing profiles use an unknown-user label. It straddles the top edge of the input surface, so it neither covers messages nor moves layout.'
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
  const helper = { ...bob, id: 'helper', displayName: 'Helper', isBot: true };
</script>

<script lang="ts">
  let typing = $state(false);
</script>

<!-- Mock composer: an input surface like the real one, with room above it for the edge tab. -->
{#snippet composer(typingUserIds: string[], storyMembers: RoomMember[], width = 'w-96')}
  <div
    class={[
      '@container/composer max-w-full rounded-lg border border-border bg-background p-2 pt-8',
      width
    ]}
  >
    <div class="relative flex chat-input-surface min-w-0 items-center gap-2 px-2.5 py-1.5">
      <TypingIndicator {typingUserIds} members={storyMembers} />
      <span class="min-w-0 flex-1 truncate text-muted">Type a message…</span>
      <span aria-hidden="true" class="iconify icon-[uil--telegram-alt] shrink-0 text-muted"></span>
    </div>
  </div>
{/snippet}

<Story name="Fade and zoom in and out" asChild>
  <div class="flex flex-col items-start gap-4">
    <Button onclick={() => (typing = !typing)}>{typing ? 'Stop typing' : 'Start typing'}</Button>
    {@render composer(typing ? ['alice', 'bob'] : [], members)}
  </div>
</Story>

<Story name="Single typer" asChild>
  {@render composer(['alice'], members, 'w-96')}
</Story>

<Story name="Two typers" asChild>
  {@render composer(['alice', 'bob'], members, 'w-96')}
</Story>

<Story name="Bot typing" asChild>
  {@render composer(['helper', 'alice'], [helper, alice], 'w-96')}
</Story>

<Story name="Image and initial avatars" asChild>
  {@render composer(['alice', 'bob'], [{ ...alice, avatarUrl }, bob], 'w-96')}
</Story>

<Story name="Large group (aggregate fallback)" asChild>
  {@render composer(['alice', 'bob', 'carol', 'dave'], members, 'w-96')}
</Story>

<Story name="Unknown members" asChild>
  {@render composer(['ghost-1', 'ghost-2', 'ghost-3'], members, 'w-96')}
</Story>

<Story name="Partially known group" asChild>
  {@render composer(['alice', 'ghost', 'carol'], [alice], 'w-96')}
</Story>

<Story name="Narrow pane with long names" asChild>
  {@render composer(
    ['alice', 'bob', 'carol', 'dave'],
    [
      { ...alice, displayName: 'A very long display name for a narrow conversation' },
      bob,
      carol,
      dave
    ],
    'w-60'
  )}
</Story>

<Story name="Mixed direction names" asChild>
  <div dir="rtl">
    {@render composer(['alice', 'bob'], [{ ...alice, displayName: 'علي' }, bob], 'w-60')}
  </div>
</Story>

<Story name="Hidden when nobody is typing" asChild>
  {@render composer([], members, 'w-96')}
</Story>
