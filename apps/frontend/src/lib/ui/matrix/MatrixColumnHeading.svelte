<!--
@component

Domain-neutral vertical heading used by dense matrix columns. It owns the
shared writing direction, rotation, and no-wrap geometry while callers supply
the label or interactive control.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';

  let {
    children,
    class: className,
    title
  }: {
    children: Snippet;
    class?: ClassValue;
    title?: string;
  } = $props();
</script>

<span
  class={['rotate-180 text-sm whitespace-nowrap [writing-mode:vertical-rl]', className]}
  {title}
>
  {@render children()}
</span>
