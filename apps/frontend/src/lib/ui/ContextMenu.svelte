<!--
@component

A reusable floating menu/popover. On hover-capable devices, positions itself at a viewport point or
anchored to an element. On pure touch-primary devices, renders as a BottomSheet instead. Handles
click-outside dismissal, Escape key, and scroll dismissal (floating), or swipe-to-close (sheet).

Built on top of `FloatingPopover` — the desktop branch is just menu-specific styling and Escape
handling around the shared positioning primitive.

**Props:**
- `position` - Viewport coordinates {x, y} for point-based positioning (context menus)
- `anchor` - Element rect {top, bottom, left} for anchor-based positioning (popovers)
- `role` - ARIA role (default: "menu")
- `ariaLabel` - ARIA label for the container
- `presentation` - "auto" uses input capability, "floating" or "sheet" forces a mode
- `class` - Additional CSS classes for the outer container (floating mode only)
- `onclose` - Callback when the menu should be dismissed

In floating mode, exactly one of `position` or `anchor` must be provided. In sheet mode, both are
ignored (the BottomSheet handles its own positioning).
-->
<script lang="ts">
  import { fade } from 'svelte/transition';
  import type { Snippet } from 'svelte';
  import BottomSheet from './BottomSheet.svelte';
  import FloatingPopover from './FloatingPopover.svelte';
  import { prefersTouchActions, supportsHoverActions } from '$lib/utils/inputCapabilities';

  type ContextMenuPresentation = 'auto' | 'floating' | 'sheet';

  let {
    position,
    anchor,
    role = 'menu',
    ariaLabel,
    presentation = 'auto',
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
    presentation?: ContextMenuPresentation;
    class?: string;
    onclose: () => void;
    onmouseenter?: () => void;
    onmouseleave?: () => void;
    children: Snippet;
  } = $props();

  const useSheet = $derived(
    presentation === 'sheet' ||
      (presentation === 'auto' && prefersTouchActions() && !supportsHoverActions())
  );
  let sheetVisible = $state(true);

  function handleKeydown(e: KeyboardEvent) {
    if (useSheet) return;
    if (e.key === 'Escape') {
      e.stopPropagation();
      onclose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if useSheet}
  <BottomSheet bind:visible={sheetVisible} {onclose}>
    {@render children()}
  </BottomSheet>
{:else}
  <FloatingPopover
    {position}
    {anchor}
    {role}
    {ariaLabel}
    class={['min-w-48 menu', className]}
    {onclose}
    {onmouseenter}
    {onmouseleave}
  >
    <div class="flex flex-col gap-1" transition:fade|global={{ duration: 100 }}>
      {@render children()}
    </div>
  </FloatingPopover>
{/if}
