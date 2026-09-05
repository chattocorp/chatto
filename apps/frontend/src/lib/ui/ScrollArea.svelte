<!--
@component

Reusable scroll viewport. It owns the relative outer wrapper, native vertical
and optional horizontal scrolling, and exposes its inner element for consumers
such as virtualizers and infinite-scroll observers. `ScrollFader` composes this
primitive when a scroll viewport also needs edge fades. A stable inner flex
column sizes to its content and fills short viewports, including bottom-aligned
timelines. Attach content size observers to that wrapper.
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
    /** Optional lifecycle attachment for the stable content wrapper. */
    contentAttachment?: Attachment<HTMLDivElement>;
    /** Keep the scroll viewport in the tab order for keyboard scrolling. */
    keyboardFocusable?: boolean;
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
    contentAttachment,
    keyboardFocusable = true,
    ...rest
  }: Props = $props();

</script>

<div class={['relative flex min-h-0 min-w-0 flex-col', fill && 'flex-1', className]}>
  <!-- A scroll viewport must be keyboard-focusable for WCAG 2.1. Svelte's
       generic non-interactive tabindex warning does not model that exception. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    bind:this={scrollEl}
    role={keyboardFocusable ? 'region' : undefined}
    tabindex={keyboardFocusable ? 0 : undefined}
    class={[
      'min-h-0 min-w-0 flex-1 overflow-y-auto',
      scrollX ? 'overflow-x-auto' : 'overflow-x-hidden',
      scrollClass
    ]}
    {...rest}
  >
    <div class="flex min-h-full flex-col" {@attach contentAttachment}>
      {@render children()}
    </div>
  </div>
  {@render overlay?.()}
</div>
