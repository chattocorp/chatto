<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { resolve } from '$app/paths';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import EntityLinkPill from './EntityLinkPill.svelte';

  const { Story } = defineMeta({ title: 'Messages/Entity links', component: EntityLinkPill });
  const user = {
    id: 'alice',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    presenceStatus: PresenceStatus.OFFLINE,
    avatarUrl: null
  };
  const room = {
    id: 'R123456789abcde',
    name: 'general',
    type: RoomKind.CHANNEL,
    isUniversal: false,
    viewerIsMember: true,
    viewerCanJoinRoom: false,
    viewerCanManageRoom: false,
    members: []
  };
</script>

{#snippet examples()}
  Meet
  <EntityLinkPill
    target={{ kind: 'user', serverId: 'demo', id: user.id }}
    path={resolve('/chat/[serverId]/user/[userId]', { serverId: '-', userId: user.id })}
    href="https://chat.example.test/chat/-/user/alice"
    name={user.displayName}
    {user}
  />
  in
  <EntityLinkPill
    target={{ kind: 'room', serverId: 'demo', id: room.id }}
    path={resolve('/chat/[serverId]/[roomId]', { serverId: '-', roomId: room.id })}
    href="https://chat.example.test/chat/-/R123456789abcde"
    name={room.name}
    {room}
  />
  on
  <EntityLinkPill
    target={{ kind: 'server', serverId: 'demo' }}
    path={resolve('/chat/[serverId]', { serverId: '-' })}
    href="https://chat.example.test"
    name="Community server"
  />.
{/snippet}

<Story name="Inline identities" asChild>
  <div class="prose max-w-xl bg-background p-4">{@render examples()}</div>
</Story>
<Story name="Narrow message" asChild>
  <div class="prose w-64 bg-background p-4">{@render examples()}</div>
</Story>
