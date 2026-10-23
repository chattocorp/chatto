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
            'One followed thread: latest reply, room and root message, reply count, and a participant preview. Unread threads have bold text and a dot. Important notification attention keeps its colour. Action space stays equal on read and unread rows.'
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

  const line = (author: UserAvatarUserView, excerpt: string) => ({
    name: author.displayName,
    identity: author,
    excerpt
  });

  const rows = [
    {
      participants: [alice],
      latest: line(alice, 'An unread reply without a notification.'),
      root: line(bob, 'Checking the unread thread appearance'),
      roomName: 'general',
      time: '2m ago',
      replyCount: '1 reply',
      unread: true,
      attention: NotificationAttentionLevel.UNSPECIFIED,
      canMarkRead: true
    },
    {
      participants: [bob, alice],
      latest: line(bob, 'Looks great, nice work! Shipping it after lunch.'),
      root: line(alice, 'Shipping the new notifications layout today!'),
      roomName: 'chatto-development',
      time: '2m ago',
      replyCount: '4 replies',
      unread: true,
      attention: NotificationAttentionLevel.IMPORTANT,
      canMarkRead: true
    },
    {
      participants: [carol, bob],
      latest: line(carol, 'I can reproduce this on Firefox too.'),
      root: line(bob, 'Scrolling jumps when images load in long threads'),
      roomName: 'bug-reports',
      time: '1h ago',
      replyCount: '12 replies',
      unread: true,
      attention: NotificationAttentionLevel.AMBIENT,
      canMarkRead: true
    },
    {
      participants: [alice],
      latest: line(alice, 'Release notes are up for beta 15.'),
      root: null,
      roomName: 'announcements',
      time: '9h ago',
      replyCount: '1 reply',
      unread: false,
      attention: NotificationAttentionLevel.UNSPECIFIED,
      canMarkRead: false
    },
    {
      participants: [bob, carol],
      latest: line(bob, 'Agreed, let’s keep the old shortcut for a while.'),
      root: line(carol, 'Should we change the quick switcher shortcut?'),
      roomName: 'random-chat',
      time: '11h ago',
      replyCount: '7 replies',
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

<Story name="List" asChild>
  <div class="selectable-list max-w-2xl">
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
</Story>
