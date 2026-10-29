<!-- @component An automatic URL display: known entity pill or compact web link. Never discovers targets. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverRegistry } from '$lib/client';
  import { parseEntityLink } from '$lib/chatLinks';
  import { serverIdToSegment } from '$lib/navigation';
  import { conversationPath, userPath } from '$lib/navigation/conversationPath';
  import { serverDisplayName } from '@chatto/client/server/state';
  import EntityLinkPill from './EntityLinkPill.svelte';
  import UrlLink from './UrlLink.svelte';

  let { href }: { href: string } = $props();
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

{#if known && target && path}
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
