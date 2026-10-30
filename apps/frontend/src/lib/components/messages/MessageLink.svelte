<!-- @component A message destination described only by existing client data. Reading this link never loads a timeline. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { serverRegistry } from '$lib/client';
  import type { MessageLink } from '$lib/chatLinks';
  import { serverDisplayName } from '@chatto/client/server/state';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import { m } from '$lib/i18n/messages';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import LinkHover from './LinkHover.svelte';
  import MessageLinkPill from './MessageLinkPill.svelte';

  let { href, link, children }: { href: string; link: MessageLink; children?: Snippet } = $props();
  const store = $derived(link.serverId ? serverRegistry.tryGetStore(link.serverId) : undefined);
  const server = $derived(link.serverId ? serverRegistry.getServer(link.serverId) : undefined);
  const room = $derived(store?.roomList.rooms.find((room) => room.id === link.roomId));
  const event = $derived(store?.knownMessage(link.roomId, link.messageId));
  const author = $derived(event?.actor && !event.actor.deleted ? event.actor : undefined);
  const profile = $derived(author ? store?.projection.users.view(author.id) : undefined);
  const user = $derived(profile?.deleted ? undefined : (profile ?? author));
  const name = $derived(
    user ? user.displayName || user.login : m('message_preview.open_linked_message')
  );
  const serverName = $derived(
    (store ? serverDisplayName(store.serverInfo, server?.name) : server?.name) ?? ''
  );
  const context = $derived(room?.name ?? serverName);
  const body = $derived(isMessagePostedEvent(event?.event) ? event.event.body : undefined);
</script>

<LinkHover
  {href}
  local={{
    name,
    server: room ? `${room.name} · ${serverName}` : serverName,
    description: body,
    timestamp: event?.createdAt
      ? formatDateTime(event.createdAt, timeFormatSettingsFor(store?.currentUser.user?.settings))
      : undefined,
    image: user?.avatarUrl,
    kind: user ? 'user' : 'room',
    icon: 'icon-[uil--comment-alt-message]'
  }}
>
  {#if children}
    {@render children()}
  {:else}
    <MessageLinkPill {href} {name} {context} {user} />
  {/if}
</LinkHover>
