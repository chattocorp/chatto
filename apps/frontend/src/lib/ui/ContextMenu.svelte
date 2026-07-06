<!--
@component

A reusable floating menu/popover. On desktop, positions itself at a viewport point or anchored to
an element. On touch devices, renders as a BottomSheet instead. Handles click-outside dismissal,
Escape key, and scroll dismissal (desktop), or swipe-to-close (mobile).

Built on top of `FloatingPopover` — the desktop branch is just menu-specific styling and Escape
handling around the shared positioning primitive.

**Props:**
- `position` - Viewport coordinates {x, y} for point-based positioning (context menus)
- `anchor` - Element rect {top, bottom, left} for anchor-based positioning (popovers)
- `role` - ARIA role (default: "menu")
- `ariaLabel` - ARIA label for the container
- `class` - Additional CSS classes for the outer container (desktop only)
- `onclose` - Callback when the menu should be dismissed

On desktop, exactly one of `position` or `anchor` must be provided. On touch devices, both are
ignored (the BottomSheet handles its own positioning).
-->
<script lang="ts">
  import { fade } from 'svelte/transition';
  import type { Snippet } from 'svelte';
  import BottomSheet from './BottomSheet.svelte';
  import FloatingPopover from './FloatingPopover.svelte';
  import { isTouchDevice } from '$lib/utils/isTouchDevice';

  let {
    position,
    anchor,
    role = 'menu',
    ariaLabel,
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
    class?: string;
    onclose: () => void;
    onmouseenter?: () => void;
    onmouseleave?: () => void;
    children: Snippet;
  } = $props();

  const isTouch = isTouchDevice();
  let sheetVisible = $state(true);

  function handleKeydown(e: KeyboardEvent) {
    if (isTouch) return;
    if (e.key === 'Escape') {
      e.stopPropagation();
      onclose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isTouch}
  <BottomSheet bind:visible={sheetVisible} {onclose}>
    {@render children()}
  </BottomSheet>
{:else}
  <FloatingPopover
    {position}
    {anchor}
    {role}
    {ariaLabel}
    class={['menu min-w-48', className]}
    {onclose}
    {onmouseenter}
    {onmouseleave}
  >
    <div class="flex flex-col gap-1" transition:fade|global={{ duration: 100 }}>
      {@render children()}
    </div>
  </FloatingPopover>
{/if}
