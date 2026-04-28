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
- While pinned, a click outside dismisses it.

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

  let {
    children,
    label = 'More information'
  }: {
    children: Snippet;
    /** aria-label for the trigger button. */
    label?: string;
  } = $props();

  let open = $state(false);
  let pinned = $state(false);
  let wrapper = $state<HTMLSpanElement>();
  const tooltipId = `help-tooltip-${crypto.randomUUID().slice(0, 8)}`;

  function showHover() {
    if (!pinned) open = true;
  }
  function hideHover() {
    if (!pinned) open = false;
  }
  function toggle(e: MouseEvent) {
    // Stop propagation so the document click listener doesn't immediately
    // unpin a freshly-pinned popover.
    e.stopPropagation();
    pinned = !pinned;
    open = pinned;
  }

  // Click outside closes a pinned popover.
  $effect(() => {
    if (!pinned) return;
    function onDocClick(e: MouseEvent) {
      if (wrapper && !wrapper.contains(e.target as Node)) {
        pinned = false;
        open = false;
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        pinned = false;
        open = false;
      }
    }
    document.addEventListener('click', onDocClick);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('click', onDocClick);
      document.removeEventListener('keydown', onKey);
    };
  });
</script>

<span bind:this={wrapper} class="relative inline-flex align-middle">
  <button
    type="button"
    aria-label={label}
    aria-describedby={open ? tooltipId : undefined}
    class="-m-1 inline-flex cursor-help items-center p-1 text-muted/60 hover:text-muted focus-visible:text-muted focus-visible:outline-none"
    onmouseenter={showHover}
    onmouseleave={hideHover}
    onfocus={showHover}
    onblur={hideHover}
    onclick={toggle}
  >
    <span class="iconify text-base uil--info-circle" aria-hidden="true"></span>
  </button>

  {#if open}
    <span
      id={tooltipId}
      role="tooltip"
      class="absolute top-full left-0 z-10 mt-1 w-max max-w-xs rounded-md border border-border bg-surface-200 px-3 py-2 text-xs text-text shadow-lg"
    >
      {@render children()}
    </span>
  {/if}
</span>
