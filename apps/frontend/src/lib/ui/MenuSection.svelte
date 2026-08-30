<!--
@component

Groups related `MenuItem` components on one standard menu surface. Use sibling
sections to separate action groups; the owning `ContextMenu` supplies the gap.

Set `ariaLabel` only when the group needs its own accessible name.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';

  let {
    ariaLabel,
    class: className,
    children
  }: {
    ariaLabel?: string;
    /** Layout-only classes for exceptional section composition. */
    class?: ClassValue;
    children: Snippet;
  } = $props();
</script>

<div
  class={['flex flex-col gap-1 menu-section', className]}
  role={ariaLabel ? 'group' : undefined}
  aria-label={ariaLabel}
>
  {@render children()}
</div>
