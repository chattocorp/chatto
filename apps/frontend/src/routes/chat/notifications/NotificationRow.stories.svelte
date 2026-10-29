<script module lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { accountNameToken } from '@chatto/client/timeline/accountName';
  import { NotificationSignalKind, type NotificationActor } from '@chatto/client/api/notifications';
  import imageAvatar from '$lib/assets/chatto-icon-maskable.png';
  import { m } from '$lib/i18n/messages';
  import NotificationRow from './NotificationRow.svelte';

  const { Story } = defineMeta({
    title: 'Notifications/Notification row',
    component: NotificationRow,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'One notification group: cause badge on the actor avatar, short summary, time, and room. When more than one server is signed in, a server logo appears before the avatar and the server name after the room.'
        }
      }
    }
  });

  function actor(id: string, displayName: string, avatarUrl: string | null = null) {
    return {
      id,
      login: id,
      displayName,
      deleted: false,
      avatarUrl,
      presenceStatus: PresenceStatus.OFFLINE,
      customStatus: null
    } satisfies NotificationActor;
  }

  const alice = actor('alice', 'Alice', imageAvatar);
  const bob = actor('bob', 'Bob');
  const carol = actor('carol', 'Carol');

  const token = accountNameToken(0);
  const accounts = (user: NotificationActor) => [{ name: user.displayName, identity: user }];

  const rows = [
    {
      summary: m('chat.notifications.summary.room_invitation', { actor: token }),
      actor: alice,
      signalKind: NotificationSignalKind.ROOM_INVITATION,
      roomName: 'design-team',
      time: '14:32',
      unread: true,
      important: true
    },
    {
      summary: m('chat.notifications.summary.direct_message', { actor: token }),
      actor: bob,
      signalKind: NotificationSignalKind.DIRECT_MESSAGE,
      roomName: null,
      time: '14:30',
      unread: true,
      important: true
    },
    {
      summary: m('chat.notifications.summary.mention', { actor: token }),
      actor: alice,
      signalKind: NotificationSignalKind.DIRECT_MENTION,
      roomName: 'chatto-development',
      time: '14:15',
      unread: true,
      important: true
    },
    {
      summary: m('chat.notifications.summary.reaction_group', { actor: token, emoji: '❤️ 😎' }),
      actor: carol,
      signalKind: NotificationSignalKind.REACTION,
      reactionEmoji: '❤️',
      roomName: 'chatto-development',
      time: '10:32',
      unread: false,
      important: false
    },
    {
      summary: m('chat.notifications.summary.followed_thread', { actor: token }),
      actor: bob,
      signalKind: NotificationSignalKind.FOLLOWED_THREAD,
      roomName: 'bug-reports',
      time: '05:32',
      unread: false,
      important: false
    },
    {
      summary: m('chat.notifications.summary.reaction', { actor: token, emoji: '👍' }),
      actor: alice,
      signalKind: NotificationSignalKind.REACTION,
      reactionEmoji: '👍',
      roomName: 'random-chat',
      time: '03:32 AM',
      unread: false,
      important: false
    }
  ];

  const servers = [
    { name: 'Chatto HQ', hostname: 'chat.chatto.run', logoUrl: imageAvatar },
    { name: 'Beans', hostname: 'beans.example', logoUrl: null },
    { name: 'Zettai', hostname: 'zettai.example', logoUrl: null }
  ];
</script>

<script lang="ts">
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';

  provideUserProfiles();
</script>

<Story name="Single server" asChild>
  <div class="selectable-list max-w-5xl">
    {#each rows as row, index (index)}
      <NotificationRow
        {...row}
        summaryAccounts={accounts(row.actor)}
        onopen={() => undefined}
        ondismiss={() => undefined}
      />
    {/each}
  </div>
</Story>

<Story name="Wrapped rows" asChild>
  <div class="selectable-list w-80 max-w-full">
    {#each rows as row, index (index)}
      <NotificationRow
        {...row}
        summaryAccounts={accounts(row.actor)}
        server={servers[index % servers.length]}
        onopen={() => undefined}
        ondismiss={() => undefined}
      />
    {/each}
  </div>
</Story>

<Story name="Multiple servers" asChild>
  <div class="selectable-list max-w-5xl">
    {#each rows as row, index (index)}
      <NotificationRow
        {...row}
        summaryAccounts={accounts(row.actor)}
        server={servers[index % servers.length]}
        onopen={() => undefined}
        ondismiss={() => undefined}
      />
    {/each}
  </div>
</Story>
