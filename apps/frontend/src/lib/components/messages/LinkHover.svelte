<!-- @component Delayed, dismissible link hover card. Owns only pointer/focus timers and positioning. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
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
    children: Snippet;
  } = $props();
  const scope = hasServerScope() ? useServerScope() : undefined;
  const cardId = $props.id();
  const destination = $derived(renderedLinkTarget(href));
  let trigger: HTMLSpanElement | undefined;
  let card = $state<HTMLAnchorElement>();
  let anchor = $state<DOMRect | null>(null);
  let closeTimer: ReturnType<typeof setTimeout> | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
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
    void href;
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
<span {@attach attachTrigger}>{@render children()}</span>{#if anchor}
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
      {:else if scope}
        {#await import('./RemoteLinkHover.svelte')}
          <LinkHoverContent {href} loading />
        {:then module}
          <module.default {href} {scope} />
        {/await}
      {:else}
        <LinkHoverContent {href} />
      {/if}
    </a>
  </FloatingPopover>
{/if}
