<!-- @component
An inset frame with optional actions in its right rim. Content and controls use
separate layout columns, so controls cannot cover links, artwork, or players.
Hover or keyboard focus reveals desktop controls. Touch-primary devices use
a context sheet opened by a long-press on the content. The owner supplies
content, permissions, and callbacks.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import type { HTMLAttributes } from 'svelte/elements';
  import PreviewToolbar from './PreviewToolbar.svelte';
  import PreviewActionSheet from './PreviewActionSheet.svelte';
  import { contextMenuTrigger } from './contextMenuTrigger.svelte';

  let {
    children,
    actions,
    touchActions,
    class: className,
    contentClass,
    ...attributes
  }: Omit<HTMLAttributes<HTMLDivElement>, 'children'> & {
    children: Snippet;
    /** Omit this snippet when no actions are available. */
    actions?: Snippet;
    /** Optional sheet actions, for example View when the image itself is the desktop trigger. */
    touchActions?: Snippet;
    /** Layout for the content column; its inset finish belongs to the content. */
    contentClass?: string;
  } = $props();

  const touchPrimary = new MediaQuery('(pointer: coarse)', false);
  const sheetActions = $derived(touchActions ?? actions);
  const ownsTouch = $derived(touchPrimary.current && !!sheetActions);
  let sheetOpen = $state(false);

  /** The frame owns the held touch, including native menus from its descendants. */
  function touchMenu(node: HTMLDivElement) {
    if (!ownsTouch) return;
    const dispose = contextMenuTrigger(() => (sheetOpen = true), {
      capture: true
    })(node);
    return () => {
      dispose?.();
      // A removed gesture surface cannot retain an open sheet.
      sheetOpen = false;
    };
  }
</script>

<div
  {...attributes}
  class={['preview-action-frame preview-frame', className]}
  data-preview-actions={ownsTouch || undefined}
>
  <div class={['preview-frame-body', contentClass]} {@attach touchMenu}>
    {@render children()}
  </div>
  {#if actions && !touchPrimary.current}
    <div class="preview-frame-actions">
      <PreviewToolbar>{@render actions()}</PreviewToolbar>
    </div>
  {/if}
  {#if ownsTouch && sheetOpen && sheetActions}
    <PreviewActionSheet onclose={() => (sheetOpen = false)}>
      {@render sheetActions()}
    </PreviewActionSheet>
  {/if}
</div>
