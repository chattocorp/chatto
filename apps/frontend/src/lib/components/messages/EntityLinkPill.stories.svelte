<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { resolve } from '$app/paths';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import EntityLinkPill from './EntityLinkPill.svelte';

  const serverLogo = `data:image/svg+xml,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="#6750a4"/><path d="M8 8h16v12H14l-6 5z" fill="white"/></svg>'
  )}`;

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

{#snippet examples(logoUrl?: string)}
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
    {logoUrl}
  />.
{/snippet}

<Story name="Inline identities" asChild>
  <div class="prose max-w-xl bg-background p-4"><p>{@render examples()}</p></div>
</Story>
<Story name="Narrow message" asChild>
  <div class="prose w-64 bg-background p-4"><p>{@render examples()}</p></div>
</Story>
<Story name="Large text and server image" asChild>
  <div class="text-2xl">
    <div class="prose max-w-xl bg-background p-4"><p>{@render examples(serverLogo)}</p></div>
  </div>
</Story>
