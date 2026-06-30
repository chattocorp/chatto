<!--
@component

Inline help affordance: an info icon that reveals a small popover with
extra context. Use this for "what is this?" hints next to labels,
permission identifiers, etc. — content the user might want, but doesn't
need to see by default.

Behavior:
- Desktop hover/focus: shows the popover transiently.
- Click/tap: pins the popover open (so touch users can read it without
  needing hover, and so keyboard users can dwell on it).
- While pinned, a click/tap outside or Escape dismisses it.

The popover is rendered via `FloatingPopover`, which puts it in the
browser's top layer (escaping every ancestor stacking context) and
handles viewport-clamped positioning.

```svelte
<HelpTooltip>
  Edit and delete any room in this space, regardless of who created it.
</HelpTooltip>

<HelpTooltip label="Permission scope">
  This permission applies at every room within the space.
</HelpTooltip>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import * as m from '$lib/i18n/messages';
  import FloatingPopover from './FloatingPopover.svelte';

  let {
    children,
    label = m['ui.tooltip.more_information']()
  }: {
    children: Snippet;
    /** aria-label for the trigger button. */
    label?: string;
  } = $props();

  let open = $state(false);
  let pinned = $state(false);
  let trigger = $state<HTMLButtonElement>();
  let anchorRect = $state<{ top: number; bottom: number; left: number } | null>(null);
  const tooltipId = `help-tooltip-${crypto.randomUUID().slice(0, 8)}`;

  function setOpen(value: boolean) {
    open = value;
    if (value) {
      updateAnchor();
    } else {
      anchorRect = null;
    }
  }

  function showHover() {
    if (!pinned) setOpen(true);
  }
  function hideHover() {
    if (!pinned) setOpen(false);
  }
  function toggle(e: MouseEvent) {
    // Stop propagation so the document click listener doesn't immediately
    // unpin a freshly-pinned popover.
    e.stopPropagation();
    pinned = !pinned;
    setOpen(pinned);
  }

  function updateAnchor() {
    if (!trigger) return;
    const r = trigger.getBoundingClientRect();
    anchorRect = { top: r.top, bottom: r.bottom, left: r.left };
  }

  function attachTrigger(node: HTMLButtonElement) {
    trigger = node;
    return () => {
      if (trigger === node) trigger = undefined;
    };
  }

  function handleViewportChange() {
    if (open) updateAnchor();
  }

  function handleDocumentKeydown(e: KeyboardEvent) {
    if (!pinned) return;
    if (e.key === 'Escape') {
      pinned = false;
      setOpen(false);
    }
  }

  function handleOutsideDismiss() {
    pinned = false;
    setOpen(false);
  }
</script>

<!-- Keep the anchor following the trigger while open. FloatingPopover re-positions reactively when `anchorRect` updates. -->
<svelte:window onscrollcapture={handleViewportChange} onresize={handleViewportChange} />

<!-- Escape closes a pinned popover. Pointer-outside dismissal is handled by FloatingPopover via the `onclose` callback below. -->
<svelte:document onkeydown={handleDocumentKeydown} />

<button
  {@attach attachTrigger}
  type="button"
  aria-label={label}
  aria-describedby={open ? tooltipId : undefined}
  class="-m-1 inline-flex cursor-help items-center p-1 align-middle text-muted/60 hover:text-muted focus-visible:text-muted focus-visible:outline-none"
  onmouseenter={showHover}
  onmouseleave={hideHover}
  onfocus={showHover}
  onblur={hideHover}
  onclick={toggle}
>
  <span class="iconify text-base uil--info-circle" aria-hidden="true"></span>
</button>

{#if open && anchorRect}
  <FloatingPopover
    anchor={anchorRect}
    role="tooltip"
    id={tooltipId}
    class="max-w-xs menu"
    onclose={pinned ? handleOutsideDismiss : undefined}
  >
    <div class="menu-section px-3 py-2 text-xs whitespace-normal">
      {@render children()}
    </div>
  </FloatingPopover>
{/if}
