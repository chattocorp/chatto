<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect } from 'storybook/test';
  import { resolve } from '$app/paths';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import EntityLinkPill from './EntityLinkPill.svelte';
  import UrlLink from './UrlLink.svelte';
  import MessageLinkPill from './MessageLinkPill.svelte';
  import LinkPreviewCard from '../LinkPreviewCard.svelte';

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

<Story name="Message destinations" asChild>
  <div class="flex max-w-lg flex-col gap-5 bg-background p-4">
    <p>
      As discussed in <MessageLinkPill
        href="https://chat.example.test/chat/-/R123456789abcde/m/E123456789abcde"
        name="Alice"
        context="development"
        {user}
      />.
    </p>
    <p>
      Room known: <MessageLinkPill
        href="https://chat.example.test/chat/-/R123456789abcde/m/E123456789abcde"
        name="Open linked message"
        context="development"
      />.
    </p>
    <p>
      Server known: <MessageLinkPill
        href="https://chat.example.test/chat/-/R123456789abcde/m/E123456789abcde"
        name="Open linked message"
        context="Community server"
      />.
    </p>
    <div class="w-56">
      <MessageLinkPill
        href="https://chat.example.test/chat/-/R123456789abcde/m/E123456789abcde"
        name="A very long author display name"
        context="a-very-long-room-name"
        {user}
      />
    </div>
  </div>
</Story>

{#snippet examples(logoUrl?: string)}
  Meet
  <EntityLinkPill
    target={{ kind: 'user', serverId: 'demo', id: user.id }}
    path={resolve('/chat/[serverId]/user/[userId]', { serverId: '-', userId: user.id })}
    href="https://chat.example.test/chat/-/user/alice"
    name={user.displayName}
    context="Community"
    {user}
  />
  in
  <EntityLinkPill
    target={{ kind: 'room', serverId: 'demo', id: room.id }}
    path={resolve('/chat/[serverId]/[roomId]', { serverId: '-', roomId: room.id })}
    href="https://chat.example.test/chat/-/R123456789abcde"
    name={room.name}
    context="Community"
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

<Story
  name="Web and entity links together"
  asChild
  play={async ({ canvasElement }) => {
    for (const pill of canvasElement.querySelectorAll<HTMLElement>('.entity-link, .url-link')) {
      const style = getComputedStyle(pill);
      await expect(style.borderTopRightRadius).toBe('6px');
      // Huge leading radii cause CSS to scale down the trailing corners as well.
      await expect(parseFloat(style.borderTopLeftRadius)).toBeLessThanOrEqual(
        pill.getBoundingClientRect().height / 2
      );
    }
    const image = canvasElement.querySelector('[data-entity-kind="server"] .entity-link-image')!;
    await expect(getComputedStyle(image).borderRadius).toBe('6px');
    await expect(getComputedStyle(image.firstElementChild!).borderRadius).toBe('6px');
  }}
>
  <div class="prose max-w-xl bg-background p-4">
    <p>{@render examples(serverLogo)}</p>
    <p>Read <UrlLink href="https://docs.example.test/getting-started" /> for details.</p>
    <p><UrlLink href="https://www.youtube.com/watch?v=example" /></p>
    <LinkPreviewCard
      preview={{
        url: 'https://www.youtube.com/watch?v=example',
        siteName: 'YouTube',
        title: 'Building something together',
        description: 'A preview shares the same recessed surface as its inline link.'
      }}
      showDismiss={false}
    />
  </div>
</Story>

<Story name="Narrow web links" asChild>
  <div class="prose w-64 bg-background p-4">
    <p>
      See <UrlLink
        href="https://docs.example.test/a/very/long/path/to/getting-started?language=en#intro"
      /> and then ask {@render examples(serverLogo)}
    </p>
  </div>
</Story>
