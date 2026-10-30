<!-- @component A web-link preview with an optional authored label; automatic URLs use entity or URL pills. Never discovers targets. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { serverRegistry } from '$lib/client';
  import { parseEntityLink } from '$lib/chatLinks';
  import { serverIdToSegment } from '$lib/navigation';
  import { conversationPath, userPath } from '$lib/navigation/conversationPath';
  import { serverDisplayName } from '@chatto/client/server/state';
  import EntityLinkPill from './EntityLinkPill.svelte';
  import UrlLink from './UrlLink.svelte';
  import LinkHover from './LinkHover.svelte';

  let { href, children }: { href: string; children?: Snippet } = $props();
  const target = $derived(parseEntityLink(href));
  const store = $derived(target ? serverRegistry.tryGetStore(target.serverId) : undefined);
  const server = $derived(target ? serverRegistry.getServer(target.serverId) : undefined);
  const user = $derived(
    target?.kind === 'user' ? store?.projection.users.view(target.id) : undefined
  );
  const room = $derived(
    target?.kind === 'room'
      ? store?.roomList.rooms.find((room) => room.id === target.id)
      : undefined
  );
  const known = $derived(
    !!target && !!server && (target.kind === 'server' || !!room || (!!user && !user.deleted))
  );
  const name = $derived(
    user
      ? user.displayName || user.login
      : room
        ? room.name
        : store
          ? serverDisplayName(store.serverInfo, server?.name)
          : (server?.name ?? '')
  );
  const path = $derived(
    target
      ? target.kind === 'user'
        ? userPath(target.serverId, target.id)
        : target.kind === 'room'
          ? conversationPath(target.serverId, target.id)
          : resolve('/chat/[serverId]', { serverId: serverIdToSegment(target.serverId) })
      : null
  );
</script>

<LinkHover
  {href}
  local={target && server
    ? {
        kind: target.kind,
        name,
        description:
          room?.description ??
          (target.kind === 'user' && user && !user.deleted
            ? store?.projection.users.get(target.id)?.user?.bio
            : target.kind === 'server'
              ? store?.serverInfo.description
              : null),
        icon:
          target.kind === 'room'
            ? 'icon-[uil--comments]'
            : target.kind === 'user'
              ? 'icon-[uil--user]'
              : 'icon-[uil--server]',
        image:
          user?.avatarUrl ??
          (target.kind === 'server' ? (store?.serverInfo.iconUrl ?? server.iconUrl) : null),
        server:
          target.kind !== 'server'
            ? store
              ? serverDisplayName(store.serverInfo, server.name)
              : server.name
            : undefined
      }
    : undefined}
>
  {#if children}
    {@render children()}
  {:else if known && target && path}
    <!-- Loss of the entity or replacement of its session disposes open menus. -->
    {#key store}
      <EntityLinkPill
        {target}
        {path}
        {href}
        {name}
        {user}
        {room}
        {store}
        logoUrl={store?.serverInfo.iconUrl ?? server?.iconUrl}
      />
    {/key}
  {:else}
    <UrlLink {href} />
  {/if}
</LinkHover>
