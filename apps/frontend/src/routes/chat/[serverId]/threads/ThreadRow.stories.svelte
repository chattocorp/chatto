<script module lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
  import imageAvatar from '$lib/assets/chatto-icon-maskable.png';
  import ThreadRow from './ThreadRow.svelte';

  const { Story } = defineMeta({
    title: 'Threads/Thread row',
    component: ThreadRow,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Compact followed threads reuse the activity row, avatar, and text components. Unread threads have bold text and a neutral dot. Read text and metadata are muted. Desktop actions replace trailing metadata on hover or focus; touch actions stay visible.'
        }
      }
    }
  });

  function user(id: string, displayName: string, avatarUrl: string | null = null) {
    return {
      id,
      login: id,
      displayName,
      deleted: false,
      avatarUrl,
      presenceStatus: PresenceStatus.OFFLINE,
      customStatus: null
    } satisfies UserAvatarUserView;
  }

  const alice = user('alice', 'Alice', imageAvatar);
  const bob = user('bob', 'Bob');
  const carol = user('carol', 'Carol');

  const rows = [
    {
      participants: [bob, alice],
      message: 'Checking the unread thread appearance',
      roomName: 'general',
      time: '2m ago',
      unread: true,
      attention: NotificationAttentionLevel.UNSPECIFIED,
      canMarkRead: true
    },
    {
      participants: [alice, bob],
      message: 'Shipping the new notifications layout today!',
      roomName: 'chatto-development',
      time: '2m ago',
      unread: true,
      attention: NotificationAttentionLevel.IMPORTANT,
      canMarkRead: true
    },
    {
      participants: [bob, carol],
      message: 'Scrolling jumps when images load in long threads',
      roomName: 'bug-reports',
      time: '1h ago',
      unread: true,
      attention: NotificationAttentionLevel.AMBIENT,
      canMarkRead: true
    },
    {
      participants: [alice],
      message: 'Release notes are up for beta 15.',
      roomName: 'announcements',
      time: '9h ago',
      unread: false,
      attention: NotificationAttentionLevel.UNSPECIFIED,
      canMarkRead: false
    },
    {
      participants: [carol, bob],
      message: 'Should we change the quick switcher shortcut?',
      roomName: 'random-chat',
      time: '11h ago',
      unread: false,
      attention: NotificationAttentionLevel.UNSPECIFIED,
      canMarkRead: false
    }
  ];
</script>

<script lang="ts">
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';

  provideUserProfiles();
</script>

{#snippet list()}
  <div class="selectable-list max-w-5xl">
    {#each rows as row, index (index)}
      <ThreadRow
        {...row}
        onopen={() => undefined}
        onmarkread={() => undefined}
        onunfollow={() => undefined}
      >
        {#snippet room()}#{row.roomName}{/snippet}
      </ThreadRow>
    {/each}
  </div>
{/snippet}

<Story name="List" globals={{ theme: 'light' }} asChild>
  {@render list()}
</Story>

<Story name="Dark" globals={{ theme: 'dark' }} asChild>
  {@render list()}
</Story>

<Story name="Narrow" asChild>
  <div class="w-80 max-w-full">
    {@render list()}
  </div>
</Story>
