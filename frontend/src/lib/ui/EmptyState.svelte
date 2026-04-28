<!--
@component

A centered empty-state placeholder for "nothing here yet" or "no results"
contexts: a large muted icon, an optional title, and optional body text or
a CTA passed as children.

Stretches to fill its parent (`flex-1`), so wrap the parent in a flex column
where you want it to take up the remaining space:

```svelte
<div class="flex flex-1 flex-col">
  <EmptyState icon="uil--bell-slash" title="No notifications">
    You're all caught up!
  </EmptyState>
</div>
```

For non-stretching contexts (cards, panels), the parent's flex layout
controls the height. Title and icon are both optional — pass just children
for a simple "select something to continue" placeholder.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    icon,
    title,
    children
  }: {
    /** Iconify class, e.g. `'uil--bell-slash'`. Omit for no icon. */
    icon?: string;
    /** Bold headline rendered above the body. */
    title?: string;
    /** Body content rendered below the title. */
    children?: Snippet;
  } = $props();
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
  {#if icon}
    <span class={['iconify text-5xl text-muted', icon]}></span>
  {/if}
  {#if title || children}
    <div class="flex flex-col gap-1">
      {#if title}
        <p class="font-medium">{title}</p>
      {/if}
      {#if children}
        <div class="text-sm text-muted">
          {@render children()}
        </div>
      {/if}
    </div>
  {/if}
</div>
