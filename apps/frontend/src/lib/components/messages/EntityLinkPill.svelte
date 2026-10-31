<!-- @component A native entity link. Menu state exists only while its entity and session are available. -->
<script lang="ts">
  import type { ResolvedPathname } from '$app/types';
  import type { EntityLink } from '$lib/chatLinks';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import type { RoomListItem } from '@chatto/client/server/rooms';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { roomIcon } from '$lib/render/roomIcon';
  import { handleRenderedLinkClick } from '$lib/navigation/renderedLink';
  import { contextMenuTrigger, type ContextMenuTriggerDetails } from '$lib/ui';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import ServerLogo from '$lib/components/ServerLogo.svelte';

  let {
    target,
    path,
    href,
    name,
    context,
    user,
    room,
    store,
    logoUrl
  }: {
    target: EntityLink;
    path: ResolvedPathname;
    href: string;
    name: string;
    context?: string;
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
  <span
    class="link-pill-main"
    class:link-pill-main-image={!!user || !room}
    class:link-pill-main-avatar={!!user}
  >
    <span class="link-pill-icon" class:link-pill-icon-image={!!user || !room}>
      {#if user}
        <span
          class="entity-link-image m-px inline-flex size-[calc(100%-2px)] shrink-0 rounded-full [&>div]:size-full"
          aria-hidden="true"
          ><UserAvatar {user} size="xs" mutedPlaceholder useLiveProfile={false} /></span
        >
      {:else if room}
        {@const icon = roomIcon(room)}
        <span aria-hidden="true" class={['shrink-0 self-center', icon.className]}>{icon.text}</span>
      {:else}
        <span
          class="entity-link-image inline-flex size-full shrink-0 rounded-[var(--inline-link-radius)]"
          aria-hidden="true"
        >
          <ServerLogo server={{ name, logoUrl }} compact />
        </span>
      {/if}
    </span>
    <bdi class="min-w-0 truncate">{name}</bdi></span
  >
  {#if context}<span class="link-pill-meta"><bdi class="truncate">{context}</bdi></span>{/if}
</a>
{#if menu && store}
  {#await Promise.all( [import('./EntityLinkMenu.svelte'), import('$lib/state/server/ServerScopeProvider.svelte')] ) then [actions, scope]}
    <scope.default serverId={target.serverId} {store} connection={store.connection}>
      <actions.default {target} {user} {room} {name} {menu} onclose={() => (menu = null)} />
    </scope.default>
  {/await}
{/if}
