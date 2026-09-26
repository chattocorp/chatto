<!--
@component

A history-backed view that fills the app frame. While it is open, it replaces
the Server Gutter, the sidebars, and the main area; the app header stays
available. Use it for large, browsable content that does not fit a dialog.

Render it as a direct child of the app frame. The caller must make the covered
frame content inert and invisible. The view takes focus when it opens and
returns focus to the previous element when it closes. Escape and the close
button call `onclose`.
-->
<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import HeaderIconButton from './HeaderIconButton.svelte';
  import PaneContent from './PaneContent.svelte';
  import PaneHeader from './PaneHeader.svelte';

  let {
    title,
    subtitle,
    onclose,
    children
  }: {
    title: string;
    subtitle?: string;
    /** Close the view, usually by going back in history. */
    onclose: () => void;
    children: Snippet;
  } = $props();

  let root = $state<HTMLElement>();

  onMount(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    root?.focus({ preventScroll: true });
    return () => {
      // The caller removes `inert` from the covered content in the same
      // update, so wait until that update is complete.
      queueMicrotask(() => {
        if (previous?.isConnected) previous.focus({ preventScroll: true });
      });
    };
  });

  /** Close on an Escape from inside the view that no nested control handled. */
  function handleKeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape' || event.defaultPrevented) return;
    if (!(event.target instanceof Node) || !root?.contains(event.target)) return;
    event.preventDefault();
    onclose();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<section
  bind:this={root}
  tabindex="-1"
  aria-label={title}
  class="absolute inset-0 z-30 flex flex-col bg-background outline-none"
  data-testid="frame-view"
>
  <div class="pane-page">
    <PaneHeader {title} {subtitle}>
      {#snippet actions()}
        <HeaderIconButton icon="icon-[uil--times]" label={m('ui.close')} onclick={onclose} />
      {/snippet}
    </PaneHeader>

    <PaneContent>
      {@render children()}
    </PaneContent>
  </div>
</section>
