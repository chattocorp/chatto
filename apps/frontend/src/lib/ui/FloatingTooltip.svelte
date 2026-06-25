<!--
@component

Small non-interactive tooltip surface rendered through `FloatingPopover`.

Use this when a trigger already owns its hover/focus/open state and only needs
to display short contextual text in the top layer. For pinned or interactive
help content, use `HelpTooltip` instead.

**Props:**
- `anchor` - Element rect `{ top, bottom, left }` for anchor-based positioning
- `open` - Whether the tooltip is visible. Keep mounted and toggle this when the trigger is in a measured list.
- `placement` - Preferred side for anchored tooltips, defaults to `bottom`
- `position` - Viewport point for cursor/coordinate positioning
- `id` - Optional tooltip id for `aria-describedby`
- `ariaLabel` - Optional accessible label for the tooltip surface
- `class` - Optional extra classes for the floating surface
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import FloatingPopover from './FloatingPopover.svelte';

  let {
    anchor,
    open = true,
    placement = 'bottom',
    position,
    id,
    ariaLabel,
    class: className,
    children
  }: {
    anchor?: { top: number; bottom: number; left: number } | null;
    open?: boolean;
    placement?: 'top' | 'bottom';
    position?: { x: number; y: number; alignRight?: boolean; centerX?: boolean };
    id?: string;
    ariaLabel?: string;
    class?: ClassValue;
    children: Snippet;
  } = $props();
</script>

<FloatingPopover
  {anchor}
  {open}
  anchorPlacement={placement}
  {position}
  role="tooltip"
  {id}
  {ariaLabel}
  class={['floating-tooltip pointer-events-none', className]}
>
  {@render children()}
</FloatingPopover>
