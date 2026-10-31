<!-- @component Delayed link hover card. Metadata stays subscribed after hover so URL pills can reuse its title. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { useLinkPreview } from './linkPreviewContext';
  import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { FloatingPopover } from '$lib/ui';
  import { hasServerScope, useServerScope } from '$lib/state/server/scope.svelte';
  import LinkHoverContent, { type HoverIdentity } from './LinkHoverContent.svelte';
  import { handleRenderedLinkClick, renderedLinkTarget } from '$lib/navigation/renderedLink';

  let {
    href,
    local,
    children
  }: {
    href: string;
    local?: HoverIdentity;
    children: Snippet<[string | undefined, boolean]>;
  } = $props();
  const scope = hasServerScope() ? useServerScope() : undefined;
  const { lookup: existingPreview, isLoading: existingLoading } = useLinkPreview();
  const cardId = $props.id();
  const destination = $derived(renderedLinkTarget(href));
  let trigger: HTMLSpanElement | undefined;
  let card = $state<HTMLAnchorElement>();
  let anchor = $state<DOMRect | null>(null);
  let closeTimer: ReturnType<typeof setTimeout> | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let fetchedPreview = $state.raw<LinkPreviewView | null>();
  const preview = $derived(existingPreview(href) ?? fetchedPreview);
  let loading = $state(false);
  const pageTitle = $derived(
    preview?.socialPost?.contentWarning || preview?.socialPost?.quotedPost?.contentWarning
      ? undefined
      : preview?.title || undefined
  );
  function close() {
    clearTimeout(timer);
    clearTimeout(closeTimer);
    anchor = null;
  }
  function keepOpen() {
    clearTimeout(closeTimer);
  }
  function leave() {
    // Pointer movement must not dismiss the card while its link has keyboard focus.
    if (trigger?.matches(':focus-within') || card?.matches(':focus-within')) return;
    clearTimeout(closeTimer);
    closeTimer = setTimeout(close, 180);
  }
  function activate(event: MouseEvent) {
    if (event.defaultPrevented) return;
    handleRenderedLinkClick(event, href);
    close();
  }

  // The trigger owns timers and listeners, including cleanup when its URL changes.
  function attachTrigger(node: HTMLSpanElement) {
    const url = href;
    const identity = local;
    let disposed = false;
    let generation = 0;
    let started = false;
    let stopPreview: (() => void) | undefined;
    fetchedPreview = undefined;
    loading = false;
    const stopRemoval = registerQueryCacheRemovalListener((serverId) => {
      if (serverId !== scope?.serverId) return;
      generation++;
      stopPreview?.();
      stopPreview = undefined;
      started = false;
      fetchedPreview = undefined;
      loading = false;
      close();
    });
    async function loadPreview() {
      if (identity || !scope || started || existingPreview(url)) return;
      started = true;
      loading = true;
      const current = generation;
      try {
        const { observeLinkHover } = await import('$lib/query/linkHover');
        if (disposed || current !== generation) return;
        stopPreview = observeLinkHover(scope, url, (data, pending) => {
          fetchedPreview = data;
          loading = pending;
        });
      } catch {
        if (disposed || current !== generation) return;
        loading = false;
        started = false;
      }
    }
    trigger = node;
    const link = node.querySelector('a');
    const title = link?.getAttribute('title');
    const enter = () => {
      // The hover card replaces the browser tooltip; do not display both.
      link?.removeAttribute('title');
      clearTimeout(timer);
      keepOpen();
      timer = setTimeout(() => {
        anchor = node.getBoundingClientRect();
        void loadPreview();
      }, 350);
    };
    const exit = () => {
      if (title != null) link?.setAttribute('title', title);
      clearTimeout(timer);
      leave();
    };
    node.addEventListener('mouseenter', enter);
    node.addEventListener('mouseleave', exit);
    node.addEventListener('focusin', enter);
    node.addEventListener('focusout', exit);
    node.addEventListener('click', close);
    node.addEventListener('contextmenu', close);
    return () => {
      disposed = true;
      stopPreview?.();
      stopRemoval();
      if (title != null) link?.setAttribute('title', title);
      close();
      node.removeEventListener('mouseenter', enter);
      node.removeEventListener('mouseleave', exit);
      node.removeEventListener('focusin', enter);
      node.removeEventListener('focusout', exit);
      node.removeEventListener('click', close);
      node.removeEventListener('contextmenu', close);
    };
  }

  // Add the card to the anchor's description only for the card's lifetime.
  function describeTrigger() {
    const link = trigger?.querySelector('a');
    if (!link) return;
    const previous = link.getAttribute('aria-describedby');
    link.setAttribute('aria-describedby', [previous, cardId].filter(Boolean).join(' '));
    return () => {
      if (previous === null) link.removeAttribute('aria-describedby');
      else link.setAttribute('aria-describedby', previous);
    };
  }
</script>

<svelte:window
  onkeydown={(event) => {
    if (event.key === 'Escape') {
      if (document.activeElement === card) trigger?.querySelector('a')?.focus();
      // Restoring focus can schedule another open; cancel that timer too.
      close();
    }
  }}
/>
<span {@attach attachTrigger}>{@render children(pageTitle, loading || existingLoading(href))}</span
>{#if anchor}
  <FloatingPopover
    animate
    {anchor}
    onclose={close}
    onmouseenter={keepOpen}
    onmouseleave={leave}
    class="max-h-[calc(100dvh-1rem)] preview-surface max-w-[calc(100vw-1rem)] overflow-x-hidden overflow-y-auto"
  >
    <a
      bind:this={card}
      {...destination}
      data-content-href={href}
      id={cardId}
      {@attach describeTrigger}
      class="link-hover-card block cursor-pointer rounded-md text-text no-underline outline-offset-2 focus-visible:outline-2 focus-visible:outline-action"
      data-testid="link-hover-card"
      onfocus={keepOpen}
      onblur={leave}
      onclick={activate}
      onauxclick={(event) => {
        if (event.button === 1) activate(event);
      }}
    >
      {#if local}
        <LinkHoverContent {href} {local} />
      {:else}
        <LinkHoverContent {href} {preview} {loading} />
      {/if}
    </a>
  </FloatingPopover>
{/if}
