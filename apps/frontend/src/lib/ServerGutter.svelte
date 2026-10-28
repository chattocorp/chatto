<!--
@component

The **Server Gutter** — narrow inline-start column listing every server the user
is connected to, plus the add-server button pinned to the bottom. See the
"UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverRegistry } from '$lib/client';
  import { m } from '$lib/i18n/messages';
  import { ScrollFader } from '$lib/ui';
  import ServerSidebarEntry from './ServerSidebarEntry.svelte';
  import { serverGutterOrder } from '$lib/state/serverGutterOrder.svelte';
  import type { Attachment } from 'svelte/attachments';
  import { MediaQuery, SvelteSet } from 'svelte/reactivity';
  import { prefersReducedMotion } from 'svelte/motion';
  import { fade } from 'svelte/transition';
  import { TOUCH_ONLY_QUERY } from '$lib/utils/inputMediaQueries';
  import type { GutterItem } from './serverGutterDrag.svelte';

  const touchOnly = new MediaQuery(TOUCH_ONLY_QUERY, false);
  const servers = $derived(serverRegistry.servers);
  const originId = $derived(servers.find((server) => serverRegistry.isOriginServer(server.id))?.id);
  const serverIds = $derived(
    servers.filter((server) => serverRegistry.tryGetStore(server.id)).map((server) => server.id)
  );
  const membership = $derived([...serverIds].sort().join('\0'));
  // Track nodes so an interrupted exit can resume dragging on introstart.
  const exitingEntries = new SvelteSet<HTMLDivElement>();
  // Outgoing entries remain in the DOM; the drag library requires one child per item.
  const dragEnabled = $derived(
    !touchOnly.current && serverIds.length >= 2 && exitingEntries.size === 0
  );
  const orderedItems: GutterItem[] = $derived(
    serverGutterOrder.ordered(serverIds, originId).map((id) => ({ id, serverId: id }))
  );
  let preview = $state.raw<GutterItem[] | null>(null);
  const items: GutterItem[] = $derived(preview ?? orderedItems);
  const syncOrder: Attachment = () => serverGutterOrder.listen();

  /** A preview must contain each current server once, with at most one drag shadow. */
  function validPreview(candidate: GutterItem[]): boolean {
    return (
      candidate.length === serverIds.length &&
      new Set(candidate.map((item) => item.id)).size === candidate.length &&
      new Set(candidate.map((item) => item.serverId)).size === candidate.length &&
      candidate.every((item) => serverIds.includes(item.serverId)) &&
      candidate.filter((item) => item.isDndShadowItem).length <= 1
    );
  }

  /** Restart the drag action when membership changes, cancelling stale drops. */
  const dragServers: Attachment<HTMLDivElement> = (node) => {
    const initialMembership = membership;
    // Depend on the enabled state, not the server array. Replacing a same-ID
    // store must not restart the action or clear an active preview.
    if (!dragEnabled) return;
    let disposed = false;
    let detach: (() => void) | undefined;
    // Public pages import the gutter. Keep drag code outside their initial graph.
    void import('./serverGutterDrag.svelte').then(({ attachServerGutterDrag }) => {
      if (disposed) return;
      detach = attachServerGutterDrag(node, {
        items: () => items,
        consider: (detail) => {
          if (membership === initialMembership && validPreview(detail.items)) {
            // The library changes shadow IDs in place. Keep rendered keys in
            // our own snapshot, and retain the last valid preview on bad input.
            preview = detail.items.map((item) => ({ ...item }));
          }
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
  const directoryActive = $derived(page.route.id === '/chat/servers');
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
            transition:fade={{
              duration: prefersReducedMotion.current || preview || item.isDndShadowItem ? 0 : 150
            }}
            onoutrostart={(event) => exitingEntries.add(event.currentTarget)}
            onoutroend={(event) => exitingEntries.delete(event.currentTarget)}
            onintrostart={(event) => exitingEntries.delete(event.currentTarget)}
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
      class="server-gutter-item cursor-pointer"
    >
      <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
    </a>
  </div>
</div>
