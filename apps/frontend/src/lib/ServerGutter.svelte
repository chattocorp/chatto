<!--
@component

The **Server Gutter** — narrow inline-start column listing every server the user
is connected to, plus the add-server button pinned to the bottom. See the
"UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverRegistry } from '$lib/client';
  import { m } from '$lib/i18n/messages';
  import { ScrollFader } from '$lib/ui';
  import ServerSidebarEntry from './ServerSidebarEntry.svelte';
  import { serverGutterOrder } from '$lib/state/serverGutterOrder.svelte';
  import type { Attachment } from 'svelte/attachments';
  import { MediaQuery } from 'svelte/reactivity';
  import { TOUCH_ONLY_QUERY } from '$lib/utils/inputMediaQueries';
  import type { GutterItem } from './serverGutterDrag.svelte';

  const touchOnly = new MediaQuery(TOUCH_ONLY_QUERY, false);
  const servers = $derived(serverRegistry.servers);
  const originId = $derived(servers.find((server) => serverRegistry.isOriginServer(server.id))?.id);
  const serverIds = $derived(
    servers.filter((server) => serverRegistry.tryGetStore(server.id)).map((server) => server.id)
  );
  const membership = $derived([...serverIds].sort().join('\0'));
  const orderedItems: GutterItem[] = $derived(
    serverGutterOrder.ordered(serverIds, originId).map((id) => ({ id, serverId: id }))
  );
  let preview = $state.raw<GutterItem[] | null>(null);
  const items: GutterItem[] = $derived(preview ?? orderedItems);
  const syncOrder: Attachment = () => serverGutterOrder.listen();
  /** Restart the drag action when membership changes, cancelling stale drops. */
  const dragServers: Attachment<HTMLDivElement> = (node) => {
    const initialMembership = membership;
    if (touchOnly.current || serverIds.length < 2) return;
    let disposed = false;
    let detach: (() => void) | undefined;
    // Public pages import the gutter. Keep drag code outside their initial graph.
    void import('./serverGutterDrag.svelte').then(({ attachServerGutterDrag }) => {
      if (disposed) return;
      detach = attachServerGutterDrag(node, {
        items: () => items,
        consider: (detail) => {
          if (membership === initialMembership) preview = detail.items;
        },
        finalize: ({ items: dropped, info }) => {
          const ids = dropped.filter((item) => !item.isDndShadowItem).map((item) => item.serverId);
          if (
            membership === initialMembership &&
            info.trigger !== 'droppedOutsideOfAny' &&
            ids.length === serverIds.length &&
            ids.every((id, index) => ids.indexOf(id) === index) &&
            ids.every((id) => serverIds.includes(id))
          ) {
            serverGutterOrder.save(ids);
          }
          preview = null;
        }
      });
    });
    return () => {
      disposed = true;
      detach?.();
      preview = null;
    };
  };

  const directoryHref = resolve('/chat/servers');
  const directoryActive = $derived(
    page.route.id === '/chat/servers' || page.state.modal?.type === 'addServer'
  );

  /**
   * Open the Server Directory as a history-backed dialog over the current view.
   * Modified clicks keep the link behavior, and the full page stays in place
   * when it is already open.
   */
  function openAddServerDialog(event: MouseEvent) {
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey ||
      directoryActive
    ) {
      return;
    }
    event.preventDefault();
    pushState('', { modal: { type: 'addServer' } });
  }
</script>

<div class="server-gutter flex min-h-0 flex-1 flex-col border-e border-border" {@attach syncOrder}>
  <ScrollFader top bottom scrollClass="scrollbar-hide">
    <div class="flex flex-col gap-2 p-2 max-md:ps-3">
      <div
        class="flex flex-col gap-2"
        data-sidebar-swipe-ignore={!touchOnly.current || undefined}
        data-testid="server-list"
        {@attach dragServers}
      >
        {#each items as item (item.id)}
          {@const store = serverRegistry.tryGetStore(item.serverId)}
          <div
            aria-hidden={item.isDndShadowItem || undefined}
            data-is-dnd-shadow-item-hint={item.isDndShadowItem || undefined}
          >
            {#if store}
              <!-- Authentication changes replace the store; ordering keeps the entry mounted. -->
              {#key store}
                <ServerSidebarEntry serverId={item.serverId} />
              {/key}
            {/if}
          </div>
        {/each}
      </div>
    </div>
  </ScrollFader>

  <!-- Add Server - pinned to the bottom -->
  <div class="flex shrink-0 flex-col items-center gap-2 p-2 max-md:ps-3">
    <a
      href={directoryHref}
      title={m('chat.server_gutter.add_server')}
      aria-label={m('chat.server_gutter.add_server')}
      aria-current={directoryActive ? 'page' : undefined}
      onclick={openAddServerDialog}
      class={['server-gutter-item cursor-pointer', directoryActive && 'server-gutter-item-active']}
    >
      <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
    </a>
  </div>
</div>
