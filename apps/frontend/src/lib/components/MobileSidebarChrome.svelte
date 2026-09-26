<script lang="ts">
  import type { Snippet } from 'svelte';
  import ServerGutter from '$lib/ServerGutter.svelte';
  import { m } from '$lib/i18n/messages';
  import { sidebarNav } from '$lib/state/globals.svelte';

  let {
    covered = false,
    children
  }: {
    /**
     * A frame view covers the frame. The route content becomes inert and
     * invisible but stays mounted. Wide windows also hide the Server Gutter;
     * narrow windows keep the drawer so the header's sidebar button still works.
     */
    covered?: boolean;
    children?: Snippet;
  } = $props();

  const progress = $derived(sidebarNav.isMobile ? sidebarNav.progress : 1);
  const dragging = $derived(sidebarNav.dragOffset !== null);
  const mobileClosed = $derived(sidebarNav.drawerClosed);
  const gutterCovered = $derived(covered && !sidebarNav.isMobile);

  /** Use the work plane's actual width for drawer transforms and swipe progress. */
  function observePanelWidth(node: HTMLDivElement) {
    const update = () => sidebarNav.setPanelWidth(node.clientWidth);
    const observer = new ResizeObserver(update);
    update();
    observer.observe(node);
    return () => {
      observer.disconnect();
      sidebarNav.setPanelWidth(null);
    };
  }
</script>

{#if sidebarNav.isMobile}
  <button
    type="button"
    data-app-sidebar="true"
    data-testid="mobile-sidebar-backdrop"
    class={[
      'fixed inset-x-0 mobile-sidebar-insets z-40 touch-none bg-black/50 md:hidden',
      !dragging &&
        'transition-opacity duration-[var(--motion-duration-pane)] ease-[var(--ease-out-expo)] motion-reduce:duration-0',
      mobileClosed && 'pointer-events-none'
    ]}
    style:opacity={progress}
    disabled={mobileClosed}
    tabindex={mobileClosed ? -1 : 0}
    aria-hidden={mobileClosed}
    onclick={() => sidebarNav.close()}
    aria-label={m('common.close_sidebar')}
  ></button>
{/if}

<div
  {@attach observePanelWidth}
  class="flex min-h-0 flex-1 flex-row"
  data-sidebar-closed={mobileClosed}
  data-sidebar-dragging={dragging}
  style:--sidebar-progress={progress}
  style:--sidebar-width={`${sidebarNav.panelWidth}px`}
>
  <div
    data-app-sidebar="true"
    data-testid="mobile-sidebar-panel"
    class={[
      'sidebar-drawer z-50 min-h-0 flex-col self-stretch bg-background',
      'max-md:fixed max-md:start-0 max-md:mobile-sidebar-insets max-md:w-17 max-md:touch-pan-y',
      // Mobile: always rendered so we can animate transform.
      // Desktop: hide entirely when closed (no overlay; layout reflows).
      sidebarNav.isMobile || sidebarNav.isOpen ? 'flex' : 'hidden',
      gutterCovered && 'invisible'
    ]}
    inert={mobileClosed || gutterCovered}
  >
    <ServerGutter />
  </div>

  <div class={['contents', covered && 'invisible']} inert={covered}>
    {@render children?.()}
  </div>
</div>
