<!-- @component A native entity link. Menu state exists only while its entity and session are available. -->
<script lang="ts">
  import type { ResolvedPathname } from '$app/types';
  import type { EntityLink } from '$lib/chatLinks';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import type { RoomListItem } from '@chatto/client/server/rooms';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import { handleRenderedLinkClick } from '$lib/navigation/renderedLink';
  import { contextMenuTrigger, type ContextMenuTriggerDetails } from '$lib/ui';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import ServerScopeProvider from '$lib/state/server/ServerScopeProvider.svelte';

  let {
    target,
    path,
    href,
    name,
    user,
    room,
    store,
    logoUrl
  }: {
    target: EntityLink;
    path: ResolvedPathname;
    href: string;
    name: string;
    user?: UserAvatarUserView;
    room?: RoomListItem;
    store?: ServerStateStore;
    logoUrl?: string | null;
  } = $props();
  let menu = $state<ContextMenuTriggerDetails | null>(null);
  const trigger = contextMenuTrigger((details) => (menu = details));

  function openKeyboardMenu(event: KeyboardEvent) {
    if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return;
    event.preventDefault();
    event.stopPropagation();
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { position: { x: rect.left, y: rect.bottom }, presentation: 'auto' };
  }
</script>

<a
  href={path}
  class="entity-link"
  data-entity-kind={target.kind}
  data-entity-server={target.serverId}
  title={href}
  onclick={(event) => handleRenderedLinkClick(event, path)}
  onauxclick={(event) => {
    if (event.button === 1) handleRenderedLinkClick(event, path);
  }}
  onkeydown={openKeyboardMenu}
  oncontextmenu={(event) => event.stopPropagation()}
  ontouchstart={(event) => event.stopPropagation()}
  {@attach trigger}
>
  {#if user}
    <span class="inline-flex shrink-0" aria-hidden="true"
      ><UserAvatar {user} size="xs" useLiveProfile={false} /></span
    >
  {:else if room}
    {#if room.type === RoomKind.DM || room.isUniversal}
      <span
        aria-hidden="true"
        class={[
          'iconify shrink-0',
          room.type === RoomKind.DM ? 'icon-[uil--comments]' : 'icon-[uil--globe]'
        ]}
      ></span>
    {:else}
      <span aria-hidden="true" class="shrink-0">#</span>
    {/if}
  {:else}
    <span class="inline-block size-5 shrink-0 overflow-hidden rounded-sm" aria-hidden="true">
      <ServerLogo server={{ name, logoUrl }} compact />
    </span>
  {/if}
  <bdi class="truncate">{name}</bdi>
</a>
{#if menu && store}
  <ServerScopeProvider serverId={target.serverId} {store} connection={store.connection}>
    {#await import('./EntityLinkMenu.svelte') then module}
      <module.default {target} {user} {room} {name} {menu} onclose={() => (menu = null)} />
    {/await}
  </ServerScopeProvider>
{/if}
