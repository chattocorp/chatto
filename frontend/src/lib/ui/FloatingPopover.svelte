<!--
@component

Low-level primitive for floating UI: tooltips, context menus, anchored
popovers, autocompletes. Renders in the browser's top layer via the
native `popover="manual"` attribute, so it escapes every ancestor
stacking context (sticky cells, `overflow: hidden`, `contain: layout`,
etc.) and never gets clipped by the page chrome.

Use this for any new floating UI — do NOT hand-roll `position: fixed` +
z-index. Higher-level components (`ContextMenu`, `HelpTooltip`) wrap
this with their own semantics and styling; reach for them first.

Positioning modes (exactly one is required):

- **`anchor`** — anchor rect with `{ top, bottom, left }`. The popover
  is placed below the anchor by default, flipped above if there's no
  room, and horizontally clamped to the viewport.
- **`position`** — viewport point `{ x, y }`, with optional
  `alignRight` / `centerX` flags. Used for cursor-driven menus.

When `anchor` or `position` change, the popover repositions reactively
— callers wanting "follow the trigger on scroll" can simply update the
prop on scroll.

If `onclose` is provided, the popover dismisses itself when the user
clicks/taps outside or scrolls a container that isn't part of it. The
caller still owns Escape handling (the dismissal contract is different
between tooltips and menus, and `onclose` here is intentionally
pointer-only).
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';

  const PADDING = 8; // Min distance from viewport edge.
  const GAP = 4; // Space between anchor rect and popover (anchor mode).

  let {
    position,
    anchor,
    role,
    ariaLabel,
    id,
    class: className,
    onclose,
    onmouseenter,
    onmouseleave,
    children
  }: {
    position?: { x: number; y: number; alignRight?: boolean; centerX?: boolean };
    anchor?: { top: number; bottom: number; left: number } | null;
    role?: string;
    ariaLabel?: string;
    id?: string;
    class?: ClassValue;
    /**
     * Pointer-based dismissal hook. If provided, the popover closes on
     * outside pointerdown and on outside scroll. Escape and other
     * dismissal triggers are the caller's responsibility.
     */
    onclose?: () => void;
    onmouseenter?: () => void;
    onmouseleave?: () => void;
    children: Snippet;
  } = $props();

  let node = $state<HTMLDivElement>();

  function applyPosition() {
    if (!node) return;
    const { height, width } = node.getBoundingClientRect();
    let top: number;
    let left: number;

    if (anchor) {
      // Anchor mode: prefer below, fall back above, then pin to bottom.
      if (anchor.bottom + GAP + height <= window.innerHeight - PADDING) {
        top = anchor.bottom + GAP;
      } else if (anchor.top - GAP - height >= PADDING) {
        top = anchor.top - GAP - height;
      } else {
        top = Math.max(PADDING, window.innerHeight - PADDING - height);
      }
      left = anchor.left;
      left = Math.max(PADDING, Math.min(left, window.innerWidth - PADDING - width));
    } else if (position) {
      // Point mode: prefer below/right of cursor, flip near edges.
      if (position.y + height <= window.innerHeight - PADDING) {
        top = position.y;
      } else if (position.y - height >= PADDING) {
        top = position.y - height;
      } else {
        top = Math.max(PADDING, window.innerHeight - PADDING - height);
      }

      if (position.centerX) {
        left = position.x - width / 2;
        left = Math.max(PADDING, Math.min(left, window.innerWidth - PADDING - width));
      } else if (position.alignRight) {
        left = position.x - width;
        left = Math.max(PADDING, Math.min(left, window.innerWidth - PADDING - width));
      } else if (position.x + width <= window.innerWidth - PADDING) {
        left = position.x;
      } else {
        left = Math.max(PADDING, position.x - width);
      }
    } else {
      return;
    }

    node.style.top = `${top}px`;
    node.style.left = `${left}px`;
  }

  // Show on mount + reposition reactively whenever anchor/position changes.
  $effect(() => {
    if (!node) return;
    // Re-read reactive inputs so the effect retriggers when they change.
    void anchor;
    void position;
    node.showPopover();
    applyPosition();
  });

  // Pointer-based dismissal (deferred one frame so the opening click doesn't
  // immediately close the popover).
  $effect(() => {
    if (!node || !onclose) return;
    const handlePointerDown = (e: PointerEvent) => {
      if (!node || node.contains(e.target as Node)) return;
      onclose();
    };
    const handleScroll = (e: Event) => {
      if (!node || node.contains(e.target as Node)) return;
      onclose();
    };
    const frame = requestAnimationFrame(() => {
      document.addEventListener('pointerdown', handlePointerDown);
      window.addEventListener('scroll', handleScroll, { capture: true });
    });
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener('pointerdown', handlePointerDown);
      window.removeEventListener('scroll', handleScroll, { capture: true });
    };
  });
</script>

<div
  bind:this={node}
  {id}
  popover="manual"
  class={['fixed z-50 m-0', className]}
  {role}
  aria-label={ariaLabel}
  {onmouseenter}
  {onmouseleave}
>
  {@render children()}
</div>
