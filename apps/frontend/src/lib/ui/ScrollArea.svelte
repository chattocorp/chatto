<!--
@component

Reusable scroll viewport. It owns the relative outer wrapper, native vertical
and optional horizontal scrolling, and exposes its inner element for consumers
such as virtualizers and infinite-scroll observers. `ScrollFader` composes this
primitive when a scroll viewport also needs edge fades.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';

  type Props = {
    children: Snippet;
    /** Optional non-interactive content rendered over the scroll viewport. */
    overlay?: Snippet;
    /** Let the inner viewport scroll horizontally as well as vertically. */
    scrollX?: boolean;
    /** Fill the remaining height of a flex parent. Disable for intrinsic-height viewports. */
    fill?: boolean;
    /** Extra classes for the outer positioning wrapper. */
    class?: string;
    /** Extra classes for the inner scroll container. */
    scrollClass?: string;
    /** Bound to the inner scroll container for imperative integrations. */
    scrollEl?: HTMLDivElement;
    /** Optional lifecycle attachment for the inner scroll container. */
    scrollAttachment?: Attachment<HTMLDivElement>;
    [key: string]: unknown;
  };

  let {
    children,
    overlay,
    scrollX = false,
    fill = true,
    class: className = '',
    scrollClass = '',
    scrollEl = $bindable(),
    scrollAttachment,
    ...rest
  }: Props = $props();

  let scrollProps = $derived({ tabindex: 0, ...rest });
</script>

<div class={['relative flex min-h-0 min-w-0 flex-col', fill && 'flex-1', className]}>
  <div
    bind:this={scrollEl}
    {@attach scrollAttachment}
    class={[
      'flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto',
      scrollX ? 'overflow-x-auto' : 'overflow-x-hidden',
      scrollClass
    ]}
    {...scrollProps}
  >
    {@render children()}
  </div>
  {@render overlay?.()}
</div>
